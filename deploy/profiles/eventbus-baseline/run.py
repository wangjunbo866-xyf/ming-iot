#!/usr/bin/env python3
"""构建并验证专属 Kafka/Redis 环境，所有停止操作保留数据卷。"""
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid

PROFILE = Path(__file__).resolve().parent
REPO = PROFILE.parents[2]
RUNTIME = REPO / "deploy/.runtime/eventbus-baseline"
PROJECT = "ming-iot-eventbus-baseline"
DOCKER = ["docker", "--host", "unix:///var/run/docker.sock"]
COMPOSE = DOCKER + ["compose", "--project-name", PROJECT, "--project-directory", str(PROFILE),
                    "--env-file", "/dev/null", "-f", str(PROFILE / "compose.yaml")]
ENV = {k: v for k, v in os.environ.items() if not k.startswith(("DOCKER_", "COMPOSE_"))}


def run(args, timeout=180, capture=True):
    """限定每次操作的等待时间，并保留真实退出码。"""
    result = subprocess.run(args, env=ENV, text=True, capture_output=capture, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {result.stdout or ''} {result.stderr or ''}")
    return result.stdout or ""


def require(condition, message):
    """隔离和验收检查在 python -O 下也必须执行。"""
    if not condition:
        raise ValueError(message)


def check(config):
    """拒绝外部网络、宿主端口、外部卷、额外服务及配置注入。"""
    require(config["name"] == PROJECT, "unexpected compose project")
    require(set(config["services"]) == {"kafka", "redis", "cache-a", "cache-b", "probe"}, "unexpected service set")
    networks = config["networks"]
    require(set(networks) == {"isolated"} and networks["isolated"].get("internal") is True, "internal isolated network required")
    require(networks["isolated"].get("name") == PROJECT + "_isolated", "unexpected network name")
    require(not networks["isolated"].get("external"), "external network forbidden")
    require(set(config["volumes"]) == {"kafka_data", "redis_data"}, "unexpected volume set")
    for name, volume in config["volumes"].items():
        require(volume.get("name") == PROJECT + "_" + name and not volume.get("external"), f"{name}: dedicated volume required")
        require(volume.get("driver", "local") == "local" and not volume.get("driver_opts"), f"{name}: volume driver or options forbidden")
    images = {
        "kafka": "apache/kafka:4.3.1@sha256:77e3df9054047a88b520d0cc46e16696d3b22022e1d580aeccd2632df6532837",
        "redis": "redis:7.4-alpine@sha256:ff02b58f971e7d7d156a1267e283fcbbeee91773b6aa36c49dac28ecfe28eadf",
    }
    for name, service in config["services"].items():
        require(service["image"] == images.get(name, "ming-iot-eventbus-baseline:local"), f"{name}: unexpected image")
        require(service.get("pull_policy") == "never", f"{name}: image pull forbidden")
        require(set(service["networks"]) == {"isolated"}, f"{name}: unexpected network")
        for forbidden in ("ports", "network_mode", "privileged", "devices", "extra_hosts", "env_file", "secrets", "configs", "cap_add", "pid", "ipc", "volumes_from"):
            require(not service.get(forbidden), f"{name}: {forbidden} forbidden")
        require(service.get("environment", {}) == ({"CLUSTER_ID": "P1BusVerifyIndependent1"} if name == "kafka" else {}), f"{name}: unexpected environment")
        for volume in service.get("volumes", []):
            if volume["type"] == "bind":
                require(name == "kafka" and Path(volume["source"]).resolve() == PROFILE / "config", f"{name}: unexpected bind source")
                require(volume["target"] == "/mnt/shared/config" and volume.get("read_only") is True, f"{name}: read-only config mount required")
            else:
                require(volume["type"] == "volume" and volume["source"] == name + "_data", f"{name}: unexpected volume source")
                require(volume["target"] == {"kafka": "/var/lib/kafka/data", "redis": "/data"}[name], f"{name}: unexpected volume target")
    properties = (PROFILE / "config/server.properties").read_text()
    require("advertised.listeners=INTERNAL://kafka:29092\n" in properties, "internal advertised listener required")
    require("EXTERNAL" not in properties, "external listener forbidden")


def contract():
    check(json.loads(run(COMPOSE + ["--profile", "verify", "config", "--format", "json"])))


def call(instance, path, body=None):
    args = COMPOSE + ["exec", "-T", "cache-" + instance, "/app/probe", "call", path]
    if body is not None:
        args.append(json.dumps(body))
    output = run(args, timeout=15)
    payloads = [line.removeprefix("EVENTBUS_RESULT ") for line in output.splitlines() if line.startswith("EVENTBUS_RESULT ")]
    if len(payloads) != 1:
        raise ValueError("expected one worker result")
    return json.loads(payloads[0])


def wait(label, predicate, seconds=45):
    deadline = time.monotonic() + seconds
    last = None
    while time.monotonic() < deadline:
        try:
            last = predicate()
            if last:
                return last
        except (RuntimeError, ValueError) as error:
            last = str(error)
        time.sleep(0.25)
    raise RuntimeError(f"timeout {label}: {last}")


def assert_read(instance, key, value):
    r = call(instance, "/read?key=" + key)
    return r.get("ok") and r.get("value", {}).get("version") == value


def verify(evidence, cycle):
    """以真实断网和重新启动暴露内存缓存与消费恢复问题。"""
    for name in "ab":
        wait("ready " + name, lambda name=name: call(name, "/state")["ready"])
    ids = [run(COMPOSE + ["ps", "-q", "cache-" + n]).strip() for n in "ab"]
    require(ids[0] and ids[1] and ids[0] != ids[1], "two distinct cache processes required")
    key = "p1-" + uuid.uuid4().hex
    require(call("a", "/write", {"key": key, "value": "v1"}).get("ok"), "v1 write failed")
    for name in "ab":
        wait("broadcast v1 " + name, lambda name=name: call(name, "/state")["notifications"].get(key, 0) >= 1)
        require(assert_read(name, key, "v1"), f"{name}: initial v1 read failed")
        require(assert_read(name, key, "v1"), f"{name}: cached v1 read failed")
    require(call("a", "/write", {"key": key, "value": "v2"}).get("ok"), "v2 write failed")
    for name in "ab":
        wait("broadcast v2 " + name, lambda name=name: call(name, "/state")["notifications"].get(key, 0) >= 2)
        wait("read v2 " + name, lambda name=name: assert_read(name, key, "v2"))
    before = call("b", "/state")
    time.sleep(1)  # 允许上一条 callback 的 commit 完成，再注入空闲失联。
    network = PROJECT + "_isolated"
    run(DOCKER + ["network", "disconnect", network, ids[1]])
    try:
        wait("disconnected readiness", lambda: not call("b", "/state")["ready"], 20)
        rejected = call("b", "/read?key=" + key)
        require(not rejected.get("ok"), f"offline read must fail: {rejected}")
        require(call("a", "/write", {"key": key, "value": "v3"}).get("ok"), "v3 write failed")
        wait("healthy replica", lambda: assert_read("a", key, "v3"))
    finally:
        run(DOCKER + ["network", "connect", "--alias", "cache-b", network, ids[1]])
    wait("reconnected readiness", lambda: call("b", "/state")["ready"], 60)
    wait("recovered value", lambda: assert_read("b", key, "v3"))
    after = call("b", "/state")
    require(after["generation"] > before["generation"] and after["pid"] == before["pid"], "reconnect must advance generation within the same process")
    wait("replayed invalidation", lambda: call("b", "/state")["notifications"].get(key, 0) >= 3)
    result = {"case": "two-real-cache-processes", "result": "PASS", "cycle": cycle,
              "key": key, "containers": ids, "before": before, "after": after, "offline_read": rejected}
    (evidence / f"cache-{cycle}.json").write_text(json.dumps(result, indent=2))
    print(f"PASS cycle={cycle} two real caches, broadcast, offline rejection, same-process reconnect", flush=True)
    for case in ("completion", "competition"):
        output = run(COMPOSE + ["run", "--rm", "--no-deps", "probe", "/app/probe", case], timeout=150)
        (evidence / f"{case}-{cycle}.jsonl").write_text(output)
        print(output, end="", flush=True)
    return key


def repeat():
    run([sys.executable, "-m", "unittest", "discover", "-s", str(PROFILE), "-p", "safety_test.py"], capture=False)
    RUNTIME.mkdir(parents=True, exist_ok=True)
    evidence_root = Path(os.environ.get("EVENTBUS_BASELINE_EVIDENCE_DIR", str(RUNTIME / "evidence")))
    evidence = evidence_root / (time.strftime("run-%Y%m%dT%H%M%SZ", time.gmtime()) + "-" + uuid.uuid4().hex[:8])
    evidence.mkdir(parents=True)
    print("eventbus evidence:", evidence, flush=True)
    env = dict(ENV, CGO_ENABLED="0")
    subprocess.run(["go", "-C", str(REPO / "backend"), "build", "-o", str(RUNTIME / "probe"), "./tools/cmd/eventbusbaseline"], env=env, check=True, timeout=240)
    run(DOCKER + ["build", "--network", "none", "-t", "ming-iot-eventbus-baseline:local", "-f", str(PROFILE / "Dockerfile"), str(RUNTIME)], capture=False)
    (evidence / "profile.json").write_text(run(COMPOSE + ["--profile", "verify", "config", "--format", "json"]))
    result = "FAIL"
    try:
        for cycle in (1, 2):
            run(COMPOSE + ["up", "-d", "--wait", "--wait-timeout", "150", "kafka", "redis"], capture=False)
            run(COMPOSE + ["exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:29092", "--create", "--if-not-exists", "--topic", "iot.bus.coordination.v1.g01", "--partitions", "3", "--replication-factor", "1", "--config", "min.insync.replicas=1", "--config", "retention.ms=604800000"])
            (evidence / f"topic-{cycle}.txt").write_text(run(COMPOSE + ["exec", "-T", "kafka", "/opt/kafka/bin/kafka-topics.sh", "--bootstrap-server", "kafka:29092", "--describe", "--topic", "iot.bus.coordination.v1.g01"]))
            run(COMPOSE + ["up", "-d", "cache-a", "cache-b"], capture=False)
            if cycle == 2:
                wait("restart retains Redis state", lambda: assert_read("a", previous, "v3"))
            previous = verify(evidence, cycle)
            run(COMPOSE + ["stop"], capture=False)
        result = "PASS"
    finally:
        (evidence / "services.log").write_text(run(COMPOSE + ["logs", "--no-color"]))
        run(COMPOSE + ["stop"], capture=False)
        (evidence / "result.json").write_text(json.dumps({"result": result, "volumes_retained": True}))
    print("PASS repeated startup; Redis records and Kafka offsets retained", flush=True)


if __name__ == "__main__":
    contract()
    command = sys.argv[1] if len(sys.argv) == 2 else ""
    if command == "contract":
        print("PASS isolated profile contract")
    elif command == "repeat":
        repeat()
    elif command == "stop":
        run(COMPOSE + ["stop"], capture=False)
    elif command == "status":
        print(run(COMPOSE + ["ps", "-a"]))
    else:
        raise SystemExit("usage: run.py contract|repeat|stop|status")
