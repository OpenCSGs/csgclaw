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

GO ?= go
GOFMT ?= gofmt

ONBOARD_BASE_URL ?= http://127.0.0.1:4000
ONBOARD_API_KEY ?= sk-1234567890
ONBOARD_MODEL_ID ?= minimax-m2.7
ONBOARD_MANAGER_IMAGE ?= ghcr.io/russellluo/picoclaw:2026.4.8.1

IMAGE ?= ghcr.io/russellluo/picoclaw
TAG ?= 2025.3.25
LOCAL_IMAGE ?= picoclaw:local

.DEFAULT_GOAL := build

.PHONY: help fmt test build run onboard clean package release tag push publish boxlite-setup

help:
	@printf '%s\n' \
		'make fmt       - format Go files' \
		'make boxlite-setup - fetch BoxLite native library if missing' \
		'make test      - run Go tests with local build cache' \
		'make build     - build $(BIN)' \
		'make run       - run the server in foreground' \
		'make onboard   - initialize ~/.csgclaw/config.toml with defaults' \
		'make package   - package current platform binary into dist/' \
		'make release   - build release archives for macOS/Linux/Windows' \
		'make clean     - remove local build outputs' \
		'make tag       - tag local manager image' \
		'make push      - push manager image' \
		'make publish   - tag and push manager image'

fmt:
	$(GOFMT) -w $(shell find cmd internal -name '*.go')

boxlite-setup:
	@if [ ! -f third_party/boxlite-go/libboxlite.a ]; then \
		echo "fetching BoxLite native library..."; \
		cd third_party/boxlite-go && BOXLITE_SDK_VERSION=v0.7.6 $(GO) run ./cmd/setup; \
	fi

test: boxlite-setup
	env GOCACHE=$(GOCACHE) $(GO) test ./...

build: boxlite-setup
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/csgclaw

run: boxlite-setup
	env GOCACHE=$(GOCACHE) $(GO) run -ldflags "$(LDFLAGS)" ./cmd/csgclaw serve

onboard: boxlite-setup
	env GOCACHE=$(GOCACHE) $(GO) run -ldflags "$(LDFLAGS)" ./cmd/csgclaw onboard \
		--base-url $(ONBOARD_BASE_URL) \
		--api-key $(ONBOARD_API_KEY) \
		--model-id $(ONBOARD_MODEL_ID) \
		--manager-image $(ONBOARD_MANAGER_IMAGE)

package: boxlite-setup
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=$(APP) GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

release: boxlite-setup
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=$(APP) GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=$(APP) GOCACHE=$(GOCACHE) $(CURDIR)/scripts/package-release.sh linux amd64

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(GOCACHE)

tag:
	docker tag $(LOCAL_IMAGE) $(IMAGE):$(TAG)

push:
	docker push $(IMAGE):$(TAG)

publish: tag push
