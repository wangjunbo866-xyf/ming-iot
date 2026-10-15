BACKEND_DIR := $(abspath ..)
BIN_DIR := $(BACKEND_DIR)/bin
GO ?= go
GO_BUILD := GOWORK=off $(GO) build

.PHONY: build build.api build.dg build.dm clean

build: build.api build.dg build.dm

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

build.api: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/thingsvr ./service/apisvr

build.dg: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/dgsvr ./service/dgsvr

build.dm: | $(BIN_DIR)
	umask 022 && $(GO_BUILD) -o $(BIN_DIR)/dmsvr ./service/dmsvr

clean:
	rm -f $(BIN_DIR)/thingsvr $(BIN_DIR)/dgsvr $(BIN_DIR)/dmsvr
