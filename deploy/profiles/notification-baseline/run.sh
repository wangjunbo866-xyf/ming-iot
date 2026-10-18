#!/usr/bin/env bash
# 固定本机 Docker 与独立项目，提供不删除数据的启动、验证和停止入口。
set -euo pipefail
profile="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$profile/../../.." && pwd)"
runtime="$profile/.runtime"
# 不继承远程 Docker context、TLS 或 Compose 覆盖入口。
unset DOCKER_CONTEXT DOCKER_HOST DOCKER_TLS_VERIFY DOCKER_CERT_PATH COMPOSE_FILE COMPOSE_PROJECT_NAME COMPOSE_PROFILES
local_docker() { docker --host unix:///var/run/docker.sock "$@"; }
compose() { local_docker compose --project-name ming-iot-notification-baseline --project-directory "$profile" --env-file /dev/null -f "$profile/compose.yaml" "$@"; }
contract() { compose --profile verify config --format json | python3 "$profile/safety.py"; }
case "${1:-}" in
  contract)
    contract
    python3 -m unittest discover -s "$profile" -p safety_test.py
    ;;
  build)
    contract
    mkdir -p "$runtime/etc"
    cp "$profile/config/sys.yaml" "$runtime/etc/sys.yaml"
    cp -R "$repo/backend/saas/service/apisvr/etc/i18n" "$runtime/etc/"
    CGO_ENABLED=0 go -C "$repo/backend/saas" build -o "$runtime/syssvr" ./service/syssvr
    CGO_ENABLED=0 go -C "$repo/backend/saas" build -o "$runtime/probe" ./service/syssvr/notificationbaseline
    local_docker build --network none -t ming-iot-notification-baseline:local -f "$profile/Dockerfile" "$runtime"
    ;;
  repeat)
    contract
    evidence_root="${NOTIFICATION_BASELINE_EVIDENCE_DIR:-$runtime/repeats}"
    mkdir -p "$evidence_root"
    evidence="$(mktemp -d "$evidence_root/repeat-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")"
    printf 'notification repeat evidence: %s\n' "$evidence"
    cleanup_repeat() {
      result=$?; cleanup_result=0; trap - EXIT
      compose logs --no-color > "$evidence/services.log" 2>&1 || true
      compose stop > "$evidence/final-stop.log" 2>&1 || cleanup_result=$?
      if [[ "$result" -eq 0 && "$cleanup_result" -ne 0 ]]; then result=$cleanup_result; fi
      printf 'exit_code=%s\nevidence=%s\n' "$result" "$evidence" > "$evidence/result.txt"
      printf 'notification repeat exit_code=%s evidence=%s\n' "$result" "$evidence"
      exit "$result"
    }
    trap cleanup_repeat EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    "$profile/run.sh" build 2>&1 | tee "$evidence/build.log"
    "$profile/run.sh" up 2>&1 | tee "$evidence/up-first.log"
    "$profile/run.sh" verify 2>&1 | tee "$evidence/verify-first.log"
    # 固定新库只读 SQL：保存所有 nb 标记消息及对应接收人，逐字段比较重启前后。
    snapshot_sql="SELECT json_build_object('message',to_jsonb(m),'recipients',(SELECT jsonb_agg(to_jsonb(u) ORDER BY u.id) FROM sys_user_message u WHERE u.message_id=m.id)) FROM sys_message_info m WHERE m.str1 LIKE 'nb-%' ORDER BY m.id"
    compose exec -T postgres psql -U notification -d notification -At -v ON_ERROR_STOP=1 -c "$snapshot_sql" > "$evidence/messages-before.jsonl"
    test "$(wc -l < "$evidence/messages-before.jsonl")" -ge 4
    "$profile/run.sh" stop 2>&1 | tee "$evidence/stop-first.log"
    "$profile/run.sh" up 2>&1 | tee "$evidence/up-repeat.log"
    compose exec -T postgres psql -U notification -d notification -At -v ON_ERROR_STOP=1 -c "$snapshot_sql" > "$evidence/messages-after.jsonl"
    cmp "$evidence/messages-before.jsonl" "$evidence/messages-after.jsonl"
    printf 'PASS preserved_message_and_recipient_rows=%s\n' "$(wc -l < "$evidence/messages-after.jsonl")" | tee "$evidence/persistence.log"
    "$profile/run.sh" verify 2>&1 | tee "$evidence/verify-repeat.log"
    ;;
  up)
    contract
    compose up -d --wait --wait-timeout 180 postgres redis nats syssvr
    ;;
  verify)
    contract
    compose run --rm --no-deps probe
    ;;
  stop)
    contract
    compose stop
    ;;
  logs)
    compose logs --no-color --tail 150
    ;;
  status)
    compose ps -a
    ;;
  *) echo 'usage: run.sh contract|build|up|verify|repeat|stop|logs|status' >&2; exit 2 ;;
esac
