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
CGO_ENABLED ?= 0
WEB_APP_DIR ?= web/app
WEB_STATIC_DIST_DIR ?= web/static-dist
WEB_PNPM ?= $(CURDIR)/scripts/web-pnpm.sh
TARGET_OS ?= $(shell $(GO) env GOOS)
TARGET_ARCH ?= $(shell $(GO) env GOARCH)
SANDBOX_BUNDLE_TOOLS_DIR ?= $(BIN_DIR)/sandbox-tools
SANDBOX_CLI_BIN ?= $(SANDBOX_BUNDLE_TOOLS_DIR)/csgclaw-cli

.DEFAULT_GOAL := build

.PHONY: help fmt test check-web-toolchain check-web-layout ensure-web-deps web-install web-dev build-web build build-all build-server build-server-bin build-sandbox-cli install-sandbox-cli run clean package package-all release

help:
	@printf '%s\n' \
		'make            - build Web UI, companion host binaries, and the Linux sandbox CLI' \
		'make build      - same as default goal' \
		'make build-all  - same as build (runtime images are remote fixed refs)' \
		'make fmt        - format Go files' \
		'make test       - run go test ./...' \
		'make web-install - install Web UI dependencies' \
		'make web-dev    - run Vite Web UI dev server' \
		'make build-web  - build Web UI app into web/static-dist' \
		'make build-server-bin - build bin/csgclaw and the host-platform bin/csgclaw-cli' \
		'make build-sandbox-cli - build Linux csgclaw-cli into bin/sandbox-tools' \
		'make run        - build (no docker images), then run the server' \
		'make clean      - remove local build outputs'

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

build: build-web build-server-bin build-sandbox-cli

build-all: build

build-server-bin:
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/csgclaw ./cmd/csgclaw
	env GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags "$(CLI_LDFLAGS)" -o $(BIN_DIR)/csgclaw-cli ./cmd/csgclaw-cli

build-server: build-server-bin build-sandbox-cli

build-sandbox-cli:
	mkdir -p "$(SANDBOX_BUNDLE_TOOLS_DIR)"
	env GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=linux GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags "$(CLI_LDFLAGS)" -o "$(SANDBOX_CLI_BIN)" ./cmd/csgclaw-cli

install-sandbox-cli: build-sandbox-cli

run: build
	env PATH="$(abspath $(BIN_DIR)):$$PATH" $(BIN) serve

package: build-web
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=$(APP) GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=$(INCLUDE_BOXLITE) BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

package-all: build-all
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=$(INCLUDE_BOXLITE) BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh $$(go env GOOS) $$(go env GOARCH)

release: build-web
	mkdir -p $(DIST_DIR)
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh darwin arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh darwin amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh darwin amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh windows amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh windows amd64

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(GOCACHE)
