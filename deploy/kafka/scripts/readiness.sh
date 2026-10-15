#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
compose_file="${MING_KAFKA_COMPOSE_FILE:-${script_dir}/../compose.yaml}"
service="${MING_KAFKA_SERVICE:-kafka}"
bootstrap_server="${MING_KAFKA_INTERNAL_BOOTSTRAP:-localhost:29092}"
attempts="${MING_KAFKA_READY_ATTEMPTS:-60}"
interval_seconds="${MING_KAFKA_READY_INTERVAL_SECONDS:-2}"

for ((attempt = 1; attempt <= attempts; attempt++)); do
  if docker compose -f "$compose_file" exec -T "$service" \
    /opt/kafka/bin/kafka-broker-api-versions.sh \
    --bootstrap-server "$bootstrap_server" >/dev/null 2>&1; then
    printf 'Kafka is ready at %s after %d attempt(s).\n' "$bootstrap_server" "$attempt"
    exit 0
  fi

  container_id="$(docker compose -f "$compose_file" ps -q "$service")"
  if [[ -z "$container_id" ]]; then
    printf 'Kafka service %s has no container.\n' "$service" >&2
    exit 1
  fi
  container_status="$(docker inspect --format '{{.State.Status}}' "$container_id")"
  case "$container_status" in
    exited|dead)
      printf 'Kafka container stopped before readiness (status=%s).\n' "$container_status" >&2
      exit 1
      ;;
  esac
  sleep "$interval_seconds"
done

printf 'Kafka did not become ready at %s after %d attempt(s).\n' "$bootstrap_server" "$attempts" >&2
exit 1
