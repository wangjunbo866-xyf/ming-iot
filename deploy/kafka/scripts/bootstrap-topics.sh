#!/usr/bin/env bash
set -Eeuo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
compose_file="${MING_KAFKA_COMPOSE_FILE:-${script_dir}/../compose.yaml}"
catalog_file="${MING_KAFKA_CATALOG_FILE:-${script_dir}/../catalog/data-plane.yaml}"
service="${MING_KAFKA_SERVICE:-kafka}"
bootstrap_server="${MING_KAFKA_INTERNAL_BOOTSTRAP:-localhost:29092}"
replication_factor="${MING_KAFKA_LOCAL_REPLICATION_FACTOR:-1}"
min_isr="${MING_KAFKA_LOCAL_MIN_ISR:-1}"

if [[ "$replication_factor" != "1" || "$min_isr" != "1" ]]; then
  printf 'local-only bootstrap requires replication_factor=1 and min_isr=1\n' >&2
  exit 1
fi

if [[ ! -r "$catalog_file" ]]; then
  printf 'Kafka catalog is not readable: %s\n' "$catalog_file" >&2
  exit 1
fi

readarray -t topics < <(
  awk '
    function flush() {
      if (topic != "") {
        print topic "|" partitions "|" retention
      }
      topic = ""
      partitions = ""
      retention = ""
    }
    /^managed_topics:/ { managed = 1; next }
    managed && /^[^[:space:]]/ { flush(); exit }
    managed && /^  - logical_name:/ { flush(); next }
    managed && /^    physical_name:/ {
      line = $0
      sub(/^    physical_name:[[:space:]]*/, "", line)
      topic = line
      next
    }
    managed && /^    partitions:/ {
      line = $0
      sub(/^    partitions:[[:space:]]*/, "", line)
      partitions = line
      next
    }
    managed && /^    retention_ms:/ {
      line = $0
      sub(/^    retention_ms:[[:space:]]*/, "", line)
      retention = line
      next
    }
    END { flush() }
  ' "$catalog_file"
)

if (( ${#topics[@]} == 0 )); then
  printf 'Kafka catalog contains no managed topics: %s\n' "$catalog_file" >&2
  exit 1
fi

for entry in "${topics[@]}"; do
  IFS='|' read -r topic partitions retention <<<"$entry"
  if [[ -z "$topic" || ! "$partitions" =~ ^[1-9][0-9]*$ || ! "$retention" =~ ^[1-9][0-9]*$ ]]; then
    printf 'Invalid managed topic entry: %s\n' "$entry" >&2
    exit 1
  fi

  docker compose -f "$compose_file" exec -T "$service" \
    /opt/kafka/bin/kafka-topics.sh \
    --bootstrap-server "$bootstrap_server" \
    --create --if-not-exists \
    --topic "$topic" \
    --partitions "$partitions" \
    --replication-factor "$replication_factor"

  docker compose -f "$compose_file" exec -T "$service" \
    /opt/kafka/bin/kafka-configs.sh \
    --bootstrap-server "$bootstrap_server" \
    --alter --entity-type topics --entity-name "$topic" \
    --add-config "cleanup.policy=delete,retention.ms=${retention},message.timestamp.type=LogAppendTime,min.insync.replicas=${min_isr}"
done

printf 'Bootstrapped %d managed Kafka topic(s) from %s.\n' "${#topics[@]}" "$catalog_file"
