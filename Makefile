APP ?= csgclaw
BIN_DIR ?= bin
BIN ?= $(BIN_DIR)/$(APP)
DIST_DIR ?= dist
GOCACHE ?= $(CURDIR)/.gocache
VERSION ?= $(shell sh $(CURDIR)/scripts/version.sh)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG ?= csgclaw/internal/version
LDFLAGS ?= -X $(VERSION_PKG).Version=$(VERSION) -X $(VERSION_PKG).Commit=$(COMMIT) -X $(VERSION_PKG).BuildTime=$(BUILD_TIME)
CLI_LDFLAGS ?= -s -w $(LDFLAGS)
CMD_PATH ?= ./cmd/$(APP)
BOXLITE_CLI_VERSION ?= v0.9.0
BOXLITE_CLI_BASE_URL ?= https://github.com/boxlite-ai/boxlite/releases/download

GO ?= go
GOFMT ?= gofmt
WEB_APP_DIR ?= web/app
WEB_STATIC_DIST_DIR ?= web/static-dist
WEB_PNPM ?= $(CURDIR)/scripts/web-pnpm.sh
TARGET_OS ?= $(shell $(GO) env GOOS)
TARGET_ARCH ?= $(shell $(GO) env GOARCH)
CLI_BIN ?= $(BIN_DIR)/csgclaw-cli

IMAGE ?= opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw
TAG ?= 2026.5.27
LOCAL_IMAGE ?= picoclaw:local
PICOCLAW_BASE_IMAGE ?= opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:$(TAG)
PICOCLAW_MANAGER_IMAGE ?= opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-manager
PICOCLAW_WORKER_IMAGE ?= opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw-worker
PICOCLAW_DOCKER_GOOS ?= linux
PICOCLAW_DOCKER_GOARCH ?= $(TARGET_ARCH)

.DEFAULT_GOAL := build-all

.PHONY: help fmt test check-web-toolchain check-web-layout ensure-web-deps web-install web-dev build-web build build-server build-csgclaw build-csgclaw-cli build-csgclaw-cli-for-picoclaw stage-picoclaw-docker-cli build-all run clean package package-all release tag push publish build-picoclaw-manager-image build-picoclaw-worker-image

help:
	@printf '%s\n' \
		'make fmt       - format Go files' \
		'make test      - run Go tests' \
		'make web-install - install Web UI dependencies' \
		'make web-dev   - run Vite Web UI dev server' \
		'make build-web - build Web UI app into web/static-dist' \
		'make build     - build Web UI, then $(BIN)' \
		'make build-server - build $(BIN) without rebuilding Web UI' \
		'make build-csgclaw - alias for build-server' \
		'make build-csgclaw-cli - build $(CLI_BIN) for TARGET_OS/TARGET_ARCH (defaults to current platform)' \
		'make build-csgclaw-cli-for-picoclaw - build PicoClaw CLI binaries for linux/amd64 and linux/arm64' \
		'make build-picoclaw-manager-image - build picoclaw-manager Docker image (repo root context)' \
		'make build-picoclaw-worker-image - build picoclaw-worker Docker image (repo root context)' \
		'make build-all - build Web UI, bin/csgclaw, and bin/csgclaw-cli' \
		'make run       - run the server in foreground' \
		'make package   - package APP binary into dist/' \
		'make package-all - package csgclaw and csgclaw-cli for current platform' \
		'make release   - build csgclaw and csgclaw-cli release archives for macOS/Linux' \
		'make clean     - remove local build outputs' \
		'make tag       - tag local manager image' \
		'make push      - push manager image' \
		'make publish   - tag and push manager image'

fmt:
	$(GOFMT) -w $(shell find cli cmd internal web -name '*.go')

test:
	env GOCACHE=$(GOCACHE) $(GO) test ./...

check-web-toolchain:
	$(WEB_PNPM) --check

check-web-layout:
	@if [ ! -d "$(WEB_APP_DIR)" ]; then \
		printf '%s\n' "Web UI source directory is missing: $(WEB_APP_DIR)."; \
		printf '%s\n' "Run make from the csgclaw repository root, or set WEB_APP_DIR=/absolute/path/to/web/app."; \
		exit 1; \
	fi
	@if [ ! -f "$(WEB_APP_DIR)/package.json" ]; then \
		printf '%s\n' "Web UI package.json is missing: $(WEB_APP_DIR)/package.json."; \
		exit 1; \
	fi
	@if [ ! -f "$(WEB_APP_DIR)/pnpm-lock.yaml" ]; then \
		printf '%s\n' "Web UI pnpm lockfile is missing: $(WEB_APP_DIR)/pnpm-lock.yaml."; \
		printf '%s\n' "Restore the lockfile before running make build-web."; \
		exit 1; \
	fi

ensure-web-deps: check-web-toolchain check-web-layout
	@if [ ! -d "$(WEB_APP_DIR)/node_modules" ] || [ ! -x "$(WEB_APP_DIR)/node_modules/.bin/vite" ]; then \
		printf '%s\n' "Web UI dependencies are missing; running make web-install before build."; \
		$(MAKE) web-install; \
	fi

web-install: check-web-toolchain check-web-layout
	@printf '%s\n' "Installing Web UI dependencies in $(WEB_APP_DIR)."
	@printf '%s\n' "If this appears stuck on registry downloads, check npm registry network/proxy access."
	@$(WEB_PNPM) install --frozen-lockfile || { \
		status=$$?; \
		printf '%s\n' "Failed to install Web UI dependencies."; \
		printf '%s\n' "Check npm registry network/proxy access, then rerun make web-install or make build-web."; \
		exit $$status; \
	}

web-dev: ensure-web-deps
	$(WEB_PNPM) dev

build-web: ensure-web-deps
	@mkdir -p "$(WEB_STATIC_DIST_DIR)"
	@$(WEB_PNPM) build || { \
		status=$$?; \
		printf '%s\n' "Failed to build Web UI."; \
		printf '%s\n' "If the error mentions vite not found, rerun make web-install and check the install output."; \
		exit $$status; \
	}
	@test -f "$(WEB_STATIC_DIST_DIR)/index.html" || { \
		printf '%s\n' "Web UI build did not produce $(WEB_STATIC_DIST_DIR)/index.html."; \
		exit 1; \
	}

build: build-web
	$(MAKE) build-server

build-server:
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD_PATH)

build-csgclaw:
	$(MAKE) build-server APP=csgclaw

build-csgclaw-cli:
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) $(GO) build -ldflags "$(CLI_LDFLAGS)" -o $(CLI_BIN) ./cmd/csgclaw-cli

build-csgclaw-cli-for-picoclaw:
	$(MAKE) build-csgclaw-cli TARGET_OS=linux TARGET_ARCH=amd64 CLI_BIN=$(BIN_DIR)/csgclaw-cli_linux_amd64
	$(MAKE) build-csgclaw-cli TARGET_OS=linux TARGET_ARCH=arm64 CLI_BIN=$(BIN_DIR)/csgclaw-cli_linux_arm64

stage-picoclaw-docker-cli:
	$(MAKE) build-csgclaw-cli TARGET_OS=$(PICOCLAW_DOCKER_GOOS) TARGET_ARCH=$(PICOCLAW_DOCKER_GOARCH) CLI_BIN=$(BIN_DIR)/csgclaw-cli

build-picoclaw-manager-image: stage-picoclaw-docker-cli
	docker build -f internal/templates/embed/picoclaw-manager/Dockerfile \
	  --build-arg PICOCLAW_IMAGE=$(PICOCLAW_BASE_IMAGE) \
	  -t $(PICOCLAW_MANAGER_IMAGE):$(TAG) .

build-picoclaw-worker-image: stage-picoclaw-docker-cli
	docker build -f internal/templates/embed/picoclaw-worker/Dockerfile \
	  --build-arg PICOCLAW_IMAGE=$(PICOCLAW_BASE_IMAGE) \
	  -t $(PICOCLAW_WORKER_IMAGE):$(TAG) .

build-all: build-web
	$(MAKE) build-server APP=csgclaw
	$(MAKE) build-csgclaw-cli

run: build-server
	$(BIN) serve

package: build-web
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=$(APP) GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=$(INCLUDE_BOXLITE) BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

package-all: build-web
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=$(INCLUDE_BOXLITE) BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

release: build-web
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux arm64

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(GOCACHE)

tag:
	docker tag $(LOCAL_IMAGE) $(IMAGE):$(TAG)

push:
	docker push $(IMAGE):$(TAG)

publish: tag push
