APP ?= csgclaw
BIN_DIR ?= bin
BIN ?= $(BIN_DIR)/$(APP)
DIST_DIR ?= dist
GOCACHE ?= $(CURDIR)/.gocache
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG ?= csgclaw/internal/version
LDFLAGS ?= -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)
CLI_LDFLAGS ?= -s -w $(LDFLAGS)
CMD_PATH ?= ./cmd/$(APP)

GO ?= go
GOFMT ?= gofmt
TARGET_OS ?= $(shell $(GO) env GOOS)
TARGET_ARCH ?= $(shell $(GO) env GOARCH)

ONBOARD_BASE_URL ?= http://127.0.0.1:4000
ONBOARD_API_KEY ?= sk-1234567890
ONBOARD_MODEL_ID ?= minimax-m2.7
ONBOARD_MANAGER_IMAGE ?= ghcr.io/russellluo/picoclaw:2026.4.15.3

IMAGE ?= ghcr.io/russellluo/picoclaw
TAG ?= 2026.4.15.3
LOCAL_IMAGE ?= picoclaw:local

.DEFAULT_GOAL := build-all

.PHONY: help fmt test build build-csgclaw build-csgclaw-cli build-csgclaw-cli-for-picoclaw build-all run onboard clean package package-all release tag push publish boxlite-setup sync-agent-runtimes

help:
	@printf '%s\n' \
		'make fmt       - format Go files' \
		'make sync-agent-runtimes - stage PicoClaw runtime workspaces for Go embed' \
		'make boxlite-setup - fetch BoxLite native library if missing' \
		'make test      - run Go tests with local build cache' \
		'make build     - build $(BIN) from $(CMD_PATH)' \
		'make build-csgclaw-cli - build bin/csgclaw-cli for TARGET_OS/TARGET_ARCH (defaults to current platform)' \
		'make build-csgclaw-cli-for-picoclaw - build bin/csgclaw-cli for linux/arm64' \
		'make build-all - build bin/csgclaw and bin/csgclaw-cli' \
		'make run       - run the server in foreground' \
		'make onboard   - initialize ~/.csgclaw/config.toml with defaults' \
		'make package   - package APP binary into dist/' \
		'make package-all - package csgclaw and csgclaw-cli for current platform' \
		'make release   - build csgclaw and csgclaw-cli release archives for macOS/Linux' \
		'make clean     - remove local build outputs' \
		'make tag       - tag local manager image' \
		'make push      - push manager image' \
		'make publish   - tag and push manager image'

fmt:
	$(GOFMT) -w $(shell find cli cmd internal -name '*.go')

sync-agent-runtimes:
	$(CURDIR)/scripts/sync-agent-runtimes.sh

boxlite-setup:
	@if [ ! -f third_party/boxlite-go/libboxlite.a ]; then \
		echo "fetching BoxLite native library..."; \
		cd third_party/boxlite-go && BOXLITE_SDK_VERSION=v0.7.6 $(GO) run ./cmd/setup; \
	fi

test: boxlite-setup sync-agent-runtimes
	env GOCACHE=$(GOCACHE) $(GO) test ./...

build: boxlite-setup sync-agent-runtimes
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD_PATH)

build-csgclaw: boxlite-setup
	$(MAKE) build APP=csgclaw

build-csgclaw-cli: sync-agent-runtimes
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GO) build -ldflags "$(CLI_LDFLAGS)" -o $(BIN_DIR)/csgclaw-cli ./cmd/csgclaw-cli

build-csgclaw-cli-for-picoclaw:
	$(MAKE) build-csgclaw-cli TARGET_OS=linux TARGET_ARCH=arm64

build-all: build-csgclaw build-csgclaw-cli

run: boxlite-setup
	env GOCACHE=$(GOCACHE) $(GO) run -ldflags "$(LDFLAGS)" ./cmd/csgclaw serve

onboard: boxlite-setup
	env GOCACHE=$(GOCACHE) $(GO) run -ldflags "$(LDFLAGS)" ./cmd/csgclaw onboard \
		--base-url $(ONBOARD_BASE_URL) \
		--api-key $(ONBOARD_API_KEY) \
		--models $(ONBOARD_MODEL_ID) \
		--manager-image $(ONBOARD_MANAGER_IMAGE)

package: boxlite-setup sync-agent-runtimes
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=$(APP) GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

package-all: boxlite-setup sync-agent-runtimes
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

release: boxlite-setup sync-agent-runtimes
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh linux amd64

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(GOCACHE)

tag:
	docker tag $(LOCAL_IMAGE) $(IMAGE):$(TAG)

push:
	docker push $(IMAGE):$(TAG)

publish: tag push
