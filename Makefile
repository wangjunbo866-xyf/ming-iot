SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c

ROOT := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
BACKEND := $(ROOT)/backend
GO ?= go
VERIFY_SCOPE ?=
VERIFY_SUITE ?=
MING_HEAVY_VERIFY ?= 0
CONTEXT_TARGET ?= $(MAKECMDGOALS)

PROTOBUF_TOOLS := $(BACKEND)/bin/protobuf-tools
PROTOBUF_TOOLS_STAMP := $(PROTOBUF_TOOLS)/.verification-profile.sha256
THINGS_BASELINE_ENV := $(ROOT)/deploy/profiles/things-baseline/.env.local

PKGS_share := gitee.com/ming-iot/share/...
PKGS_saas := gitee.com/ming-iot/saas/...
PKGS_things := gitee.com/ming-iot/things/...
PKGS_tools := ming-iot/backend/tools/...
ALL_PKGS := $(PKGS_share) $(PKGS_saas) $(PKGS_things) $(PKGS_tools)

.PHONY: help context diff-check protobuf-tools build test vet contract-static boundary-scan \
        verify-fast verify-pr verify-integration verify-release

help:
	@printf '%s\n' \
		'ming-iot verification targets:' \
		'  ci-tools / publication-check / publication-ready   audit tools, technical boundary, release readiness' \
		'  knowledge-check                            check local knowledge entrypoints and specs regressions' \
		'  build                                      build SaaS and Things service binaries' \
		'  test                                       run all Go package tests' \
		'  vet                                        run go vet for all Go packages' \
		'  contract-static                            run repository-only contract checks' \
		'  verify-fast VERIFY_SCOPE=<scope>           test and vet one of: share, saas, things, tools' \
		'  verify-pr                                  run the complete service-free PR gate' \
		'  verify-integration VERIFY_SUITE=<suite>    run one of: kafka, things-startup, notification, eventbus (heavy)' \
		'  verify-release MING_HEAVY_VERIFY=1         run PR and all release-required suites (heavy)'

context:
	@printf 'target=%s scope=%s suite=%s\n' '$(CONTEXT_TARGET)' '$(VERIFY_SCOPE)' '$(VERIFY_SUITE)'
	@$(GO) version
	@printf 'head=%s\n' "$$(git -C $(ROOT) rev-parse HEAD)"

diff-check:
	@git -C $(ROOT) diff --check

protobuf-tools:
	@set -eu; \
	tool_state() { \
		sha256sum \
			$(BACKEND)/tools/protobuf-tools.lock \
			$(PROTOBUF_TOOLS)/bin/protoc \
			$(PROTOBUF_TOOLS)/bin/protoc-gen-go \
			$(PROTOBUF_TOOLS)/bin/protoc-gen-go-grpc | sha256sum | awk '{print $$1}'; \
	}; \
	if [[ -x $(PROTOBUF_TOOLS)/bin/protoc \
		&& -x $(PROTOBUF_TOOLS)/bin/protoc-gen-go \
		&& -x $(PROTOBUF_TOOLS)/bin/protoc-gen-go-grpc \
		&& -f $(PROTOBUF_TOOLS_STAMP) \
		&& "$$(cat $(PROTOBUF_TOOLS_STAMP))" == "$$(tool_state)" ]]; then \
		printf '%s\n' 'reusing verified protobuf-tools cache'; \
		exit 0; \
	fi; \
	bash $(BACKEND)/tools/scripts/install-protobuf-tools.sh install; \
	tool_state > $(PROTOBUF_TOOLS_STAMP).tmp; \
	mv $(PROTOBUF_TOOLS_STAMP).tmp $(PROTOBUF_TOOLS_STAMP)

build: context
	@$(MAKE) -C $(BACKEND)/saas build
	@$(MAKE) -C $(BACKEND)/things build

test: context protobuf-tools
	@$(GO) -C $(BACKEND) test $(ALL_PKGS) -count=1

vet: context
	@$(GO) -C $(BACKEND) vet $(ALL_PKGS)

contract-static: context protobuf-tools
	@$(MAKE) knowledge-check
	@$(GO) -C $(BACKEND) run ./tools/cmd/kafkacontract --repo-root .. --format text
	@$(MAKE) -C $(ROOT)/deploy things-baseline-contract
	@$(GO) -C $(BACKEND)/tools run ./cmd/apicheck verify --backend ..
	@$(GO) -C $(BACKEND)/tools run ./cmd/rpcgen check --manifest ../saas/service/syssvr/proto/rpcgen.yaml --saas ../saas
	@$(GO) -C $(BACKEND)/tools run ./cmd/rpcgen check --manifest ../things/service/dmsvr/proto/rpcgen.yaml --module-root ../things

boundary-scan:
	@$(GO) -C $(BACKEND) test ./tools/internal/repositorycontract -run 'TestRepositoryDeliveryBoundary' -count=1

verify-fast:
	@case '$(VERIFY_SCOPE)' in \
		share|saas|things|tools) ;; \
		'') printf '%s\n' 'VERIFY_SCOPE is required (share, saas, things, or tools)' >&2; exit 2 ;; \
		*) printf 'unknown VERIFY_SCOPE: %s\n' '$(VERIFY_SCOPE)' >&2; exit 2 ;; \
	esac
	@$(MAKE) context CONTEXT_TARGET=verify-fast
	@$(MAKE) diff-check
	@if [[ '$(VERIFY_SCOPE)' == tools ]]; then $(MAKE) protobuf-tools; fi
	@$(GO) -C $(BACKEND) test $(PKGS_$(VERIFY_SCOPE)) -count=1
	@$(GO) -C $(BACKEND) vet $(PKGS_$(VERIFY_SCOPE))

verify-pr: context diff-check build test vet contract-static boundary-scan

verify-integration:
	@case '$(VERIFY_SUITE)' in \
		kafka|things-startup|notification|eventbus) ;; \
		'') printf '%s\n' 'VERIFY_SUITE is required (kafka, things-startup, notification or eventbus)' >&2; exit 2 ;; \
		*) printf 'unknown VERIFY_SUITE: %s\n' '$(VERIFY_SUITE)' >&2; exit 2 ;; \
	esac
	@if [[ '$(VERIFY_SUITE)' == things-startup ]]; then \
		if [[ ! -f '$(THINGS_BASELINE_ENV)' || -L '$(THINGS_BASELINE_ENV)' ]]; then \
			printf 'NOT_RUN: Things startup requires a regular 0600 environment file at %s\n' '$(THINGS_BASELINE_ENV)' >&2; \
			exit 3; \
		fi; \
		mode="$$(stat -c '%a' '$(THINGS_BASELINE_ENV)')"; \
		if [[ "$$mode" != 600 ]]; then \
			printf 'NOT_RUN: Things startup environment file mode is %s, want 600\n' "$$mode" >&2; \
			exit 3; \
		fi; \
	fi
	@$(MAKE) context CONTEXT_TARGET=verify-integration
	@case '$(VERIFY_SUITE)' in \
		kafka) \
			set -eu -o pipefail; \
			cleanup() { \
				status=$$?; cleanup_status=0; trap - EXIT; \
				$(MAKE) -C $(ROOT)/deploy kafka-down || cleanup_status=$$?; \
				if [[ $$status -eq 0 && $$cleanup_status -ne 0 ]]; then status=$$cleanup_status; fi; \
				exit $$status; \
			}; \
			trap cleanup EXIT; \
			$(MAKE) -C $(ROOT)/deploy kafka-up; \
			$(MAKE) -C $(ROOT)/deploy kafka-ready; \
			$(GO) -C $(BACKEND) run ./tools/cmd/kafkacontract --repo-root .. --integration --format text \
			;; \
		eventbus) \
			python3 $(ROOT)/deploy/profiles/eventbus-baseline/run.py repeat \
			;; \
		notification) \
			$(ROOT)/deploy/profiles/notification-baseline/run.sh repeat \
			;; \
		things-startup) \
			set -eu -o pipefail; \
			cleanup() { \
				status=$$?; cleanup_status=0; trap - EXIT; \
				if [[ $$status -ne 0 ]]; then $(MAKE) -C $(ROOT)/deploy things-baseline-logs ENV_FILE='$(THINGS_BASELINE_ENV)' || true; fi; \
				$(MAKE) -C $(ROOT)/deploy things-baseline-down ENV_FILE='$(THINGS_BASELINE_ENV)' || cleanup_status=$$?; \
				if [[ $$status -eq 0 && $$cleanup_status -ne 0 ]]; then status=$$cleanup_status; fi; \
				exit $$status; \
			}; \
			trap cleanup EXIT; \
			$(MAKE) -C $(ROOT)/deploy things-baseline-up ENV_FILE='$(THINGS_BASELINE_ENV)'; \
			$(MAKE) -C $(ROOT)/deploy things-baseline-verify ENV_FILE='$(THINGS_BASELINE_ENV)' \
			;; \
		*) exit 2 ;; \
	esac

verify-release:
	@if [[ '$(MING_HEAVY_VERIFY)' != 1 ]]; then \
		printf '%s\n' 'MING_HEAVY_VERIFY=1 is required for verify-release' >&2; \
		exit 2; \
	fi
	@$(MAKE) verify-pr
	@$(MAKE) verify-integration VERIFY_SUITE=kafka
	@$(MAKE) verify-integration VERIFY_SUITE=things-startup

.PHONY: knowledge-check
knowledge-check:
	@python3 tooling/knowledge.py check
	@python3 -m unittest discover -s tooling/tests
	@python3 -m unittest discover -s .agents/skills/ming-specs/scripts -p test_preflight.py

.PHONY: ci-tools publication-check publication-ready
ci-tools:
	@python3 tooling/install_ci_tools.py

publication-check:
	@python3 -m unittest discover -s tooling/tests -p test_publication.py
	@python3 tooling/publication.py check

publication-ready: publication-check
	@python3 tooling/publication.py ready

# 显式选择真实依赖通知验证，不并入默认无服务 PR 检查。
.PHONY: notification-baseline-contract notification-baseline-build notification-baseline-up notification-baseline-verify notification-baseline-stop notification-baseline-logs
notification-baseline-contract notification-baseline-build notification-baseline-up notification-baseline-verify notification-baseline-stop notification-baseline-logs:
	$(MAKE) -C deploy $@
