SHELL := /usr/bin/env bash
ROOT := $(abspath ..)
PROFILE := $(ROOT)/deploy/profiles/things-baseline
ENV_FILE ?= $(PROFILE)/.env.local
EXAMPLE_ENV := $(PROFILE)/.env.example
CONTRACT := $(PROFILE)/contract.yaml
TOOLS_DIR := $(ROOT)/backend/tools
BASELINE := cd $(TOOLS_DIR) && GOWORK=off go run ./cmd/thingsstartbaseline
COMPOSE := THINGS_BASELINE_ENV_FILE=$(ENV_FILE) docker compose --project-directory $(PROFILE) --env-file $(ENV_FILE) -f $(PROFILE)/compose.yaml
KAFKA_PROFILE := $(ROOT)/deploy/kafka
KAFKA_COMPOSE := docker compose --project-directory $(KAFKA_PROFILE) -f $(KAFKA_PROFILE)/compose.yaml

.PHONY: things-baseline-contract things-baseline-build things-baseline-image things-baseline-up things-baseline-verify things-baseline-logs things-baseline-down things-baseline-fast kafka-config kafka-up kafka-ready kafka-bootstrap kafka-logs kafka-down

things-baseline-contract:
	@set -eu -o pipefail; \
	check_absent() { \
		if rg -n -- "$$@"; then scan_status=0; else scan_status=$$?; fi; \
		case "$$scan_status" in \
			1) return 0 ;; \
			0) printf '%s\n' 'Forbidden content found in Things baseline inputs' >&2; return 1 ;; \
			*) printf 'Things baseline scan failed (rg exit %s)\n' "$$scan_status" >&2; return 1 ;; \
		esac; \
	}; \
	contract_env="$$(mktemp /tmp/ming-iot-things-contract.XXXXXX)"; \
	trap 'rm -f "$$contract_env"' EXIT; \
	cp $(EXAMPLE_ENV) "$$contract_env"; \
	chmod 0600 "$$contract_env"; \
	$(BASELINE) contract --contract $(CONTRACT) --env-file "$$contract_env"; \
	THINGS_BASELINE_ENV_FILE="$$contract_env" docker compose --project-directory $(PROFILE) --env-file "$$contract_env" -f $(PROFILE)/compose.yaml config --quiet; \
	check_absent '(^|[/@])iThings([?[:space:]/]|$$)|DROP DATABASE|DROP ROLE|TRUNCATE' $(PROFILE)/compose.yaml $(PROFILE)/config $(PROFILE)/scripts; \
	check_absent '(POSTGRES_ADMIN_PASSWORD|THINGS_BASELINE_DB_PASSWORD|CORE_USER_TOKEN_SECRET|THINGS_INTERNAL_AUTH_PASSWORD|THINGS_EMQX_API_KEY|THINGS_EMQX_SECRET_KEY)=[^[:space:]]+' $(EXAMPLE_ENV); \
	printf '%s\n' 'Things baseline safety scans PASS'

things-baseline-build:
	$(MAKE) -C $(ROOT)/backend/saas build.baseline
	$(MAKE) -C $(ROOT)/backend/things build.baseline
	$(BASELINE) manifest --contract $(CONTRACT) --repo $(ROOT) --bin-dir $(ROOT)/backend/bin --output $(ROOT)/deploy/.runtime/build-manifest.json

things-baseline-image: things-baseline-build
	$(BASELINE) stage --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --runtime-root $(ROOT)/deploy/.runtime
	$(BASELINE) image --contract $(CONTRACT) --repo $(ROOT) --runtime-root $(ROOT)/deploy/.runtime

things-baseline-up: things-baseline-image
	$(BASELINE) up --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime

things-baseline-verify:
	$(BASELINE) verify --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime

things-baseline-logs:
	$(BASELINE) logs --contract $(CONTRACT) --repo $(ROOT) --env-file $(ENV_FILE) --compose-file $(PROFILE)/compose.yaml --runtime-root $(ROOT)/deploy/.runtime

things-baseline-down:
	$(COMPOSE) down

things-baseline-fast: things-baseline-build things-baseline-contract
	cd $(TOOLS_DIR) && GOWORK=off go test ./internal/thingsstartbaseline ./cmd/thingsstartbaseline -count=1

kafka-config:
	$(KAFKA_COMPOSE) config --quiet

kafka-up: kafka-config
	$(KAFKA_COMPOSE) up -d kafka

kafka-ready:
	$(KAFKA_PROFILE)/scripts/readiness.sh

kafka-bootstrap: kafka-ready
	$(KAFKA_PROFILE)/scripts/bootstrap-topics.sh

kafka-logs:
	$(KAFKA_COMPOSE) logs --tail=200 kafka

kafka-down:
	$(KAFKA_COMPOSE) down

# 隔离通知基线，停止保留专属持久卷。
.PHONY: notification-baseline-contract notification-baseline-build notification-baseline-up notification-baseline-verify notification-baseline-stop notification-baseline-logs
notification-baseline-contract notification-baseline-build notification-baseline-up notification-baseline-verify notification-baseline-stop notification-baseline-logs:
	$(ROOT)/deploy/profiles/notification-baseline/run.sh $(patsubst notification-baseline-%,%,$@)
