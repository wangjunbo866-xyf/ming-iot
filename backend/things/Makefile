BACKEND_DIR := $(abspath ..)
BIN_DIR := $(BACKEND_DIR)/bin
GO ?= go
GO_BUILD := $(GO) build
BASELINE_GO_BUILD := env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -tags no_k8s -trimpath

.PHONY: build build.api build.dg build.dm build.baseline build.baseline.api build.baseline.dg build.baseline.dm clean

build: build.api build.dg build.dm

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build.api: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr

build.dg: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr

build.dm: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr

build.baseline: build.baseline.api build.baseline.dg build.baseline.dm

build.baseline.api: | $(BIN_DIR)
	rm -f $(BIN_DIR)/thingsvr
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr

build.baseline.dg: | $(BIN_DIR)
	rm -f $(BIN_DIR)/dgsvr
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr

build.baseline.dm: | $(BIN_DIR)
	rm -f $(BIN_DIR)/dmsvr
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr

clean:
	rm -f $(BIN_DIR)/thingsvr $(BIN_DIR)/dgsvr $(BIN_DIR)/dmsvr
