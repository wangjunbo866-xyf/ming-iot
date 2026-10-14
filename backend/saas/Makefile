BACKEND_DIR := $(abspath ..)
BIN_DIR := $(BACKEND_DIR)/bin
GO ?= go
GO_BUILD := $(GO) build -tags no_k8s
BASELINE_GO_BUILD := env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -tags no_k8s -trimpath

.PHONY: build build.api build.sys build.data build.timedjob build.timedscheduler build.baseline build.baseline.sys build.baseline.timedjob stage.config clean

build: build.api build.sys build.data build.timedjob build.timedscheduler

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build.api: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/saassvr ./service/apisvr

build.sys: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/syssvr ./service/syssvr

build.data: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/datasvr ./service/datasvr

build.timedjob: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/timedjobsvr ./service/timed/timedjobsvr

build.timedscheduler: | $(BIN_DIR)
	$(GO_BUILD) -o $(BIN_DIR)/timedschedulersvr ./service/timed/timedschedulersvr

build.baseline: build.baseline.sys build.baseline.timedjob

build.baseline.sys: | $(BIN_DIR)
	rm -f $(BIN_DIR)/syssvr
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/syssvr ./service/syssvr

build.baseline.timedjob: | $(BIN_DIR)
	rm -f $(BIN_DIR)/timedjobsvr
	umask 022 && $(BASELINE_GO_BUILD) -o $(BIN_DIR)/timedjobsvr ./service/timed/timedjobsvr

stage.config: | $(BIN_DIR)
	mkdir -p $(BIN_DIR)/etc
	cp -R service/apisvr/etc/. $(BIN_DIR)/etc/

clean:
	rm -f $(BIN_DIR)/saassvr \
		$(BIN_DIR)/syssvr \
		$(BIN_DIR)/datasvr \
		$(BIN_DIR)/timedjobsvr \
		$(BIN_DIR)/timedschedulersvr
