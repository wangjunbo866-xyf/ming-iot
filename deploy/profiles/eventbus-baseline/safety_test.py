"""用危险配置反例检验隔离边界；仅读取本地 Compose 渲染。"""
import copy
import json
import subprocess
import sys
import unittest

import run


class IsolationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.config = json.loads(run.run(run.COMPOSE + ["--profile", "verify", "config", "--format", "json"]))

    def rejects(self, mutate):
        config = copy.deepcopy(self.config)
        mutate(config)
        with self.assertRaises(ValueError):
            run.check(config)

    def test_normal_profile(self):
        run.check(self.config)

    def test_host_port_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["kafka"].update(ports=[{"published": "9092"}]))

    def test_shared_network_is_not_allowed(self):
        self.rejects(lambda c: c["networks"]["isolated"].update(external=True))

    def test_old_volume_name_is_not_allowed(self):
        self.rejects(lambda c: c["volumes"]["redis_data"].update(name="old_redis_data"))

    def test_host_mount_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["cache-a"].update(volumes=[{"type": "bind", "source": "/", "target": "/host"}]))

    def test_external_broker_environment_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["cache-b"].update(environment={"KAFKA_BROKERS": "remote:9092"}))

    def test_privileged_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["cache-a"].update(privileged=True))

    def test_volume_driver_options_are_not_allowed(self):
        self.rejects(lambda c: c["volumes"]["redis_data"].update(
            driver_opts={"type": "none", "o": "bind", "device": "/existing-environment/redis-data"}))

    def test_external_volume_driver_is_not_allowed(self):
        self.rejects(lambda c: c["volumes"]["redis_data"].update(driver="remote-storage"))

    def test_borrowing_an_existing_container_volume_is_not_allowed(self):
        self.rejects(lambda c: c["services"]["redis"].update(
            volumes_from=["container:existing-old-redis:rw"]))

    def test_default_local_volume_driver_is_allowed(self):
        config = copy.deepcopy(self.config)
        config["volumes"]["redis_data"]["driver"] = "local"
        run.check(config)

    def test_optimized_python_still_checks_isolation(self):
        config = copy.deepcopy(self.config)
        script = """
import json, sys, run
try:
    run.check(json.load(sys.stdin))
except ValueError:
    sys.exit(42)
"""
        for unsafe in (False, True):
            with self.subTest(unsafe=unsafe):
                if unsafe:
                    config["services"]["cache-a"]["ports"] = [{"published": "8080"}]
                result = subprocess.run([sys.executable, "-O", "-c", script],
                                        cwd=run.PROFILE, input=json.dumps(config),
                                        text=True, capture_output=True, timeout=10)
                self.assertEqual(result.returncode, 42 if unsafe else 0, result.stderr)

    def test_optimized_python_performs_write_and_rejects_failed_acceptance(self):
        # Stub only external effects; exercise the real acceptance procedure under -O.
        script = """
import json, tempfile
from pathlib import Path
import run
writes = []
def call(instance, path, body=None):
    if path == '/write':
        writes.append(body)
    return {'ok': False, 'ready': True, 'generation': 0, 'pid': 1, 'notifications': {}}
run.call = call
run.run = lambda args, **kwargs: args[-1]
run.wait = lambda *args, **kwargs: None
run.time.sleep = lambda *args: None
error = None
with tempfile.TemporaryDirectory() as directory:
    try:
        run.verify(Path(directory), 1)
    except Exception as exc:
        error = type(exc).__name__
print()
print(json.dumps({'writes': writes, 'error': error}))
"""
        result = subprocess.run([sys.executable, "-O", "-c", script], cwd=run.PROFILE,
                                text=True, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        outcome = json.loads(result.stdout.splitlines()[-1])
        self.assertEqual(len(outcome["writes"]), 1, outcome)
        self.assertEqual(outcome["error"], "ValueError", outcome)


if __name__ == "__main__":
    unittest.main()
