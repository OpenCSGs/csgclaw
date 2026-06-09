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
# Static CLI for musl-based PicoClaw/BoxLite sandbox images (see release-build-all.sh).
CGO_ENABLED ?= 0
WEB_APP_DIR ?= web/app
WEB_STATIC_DIST_DIR ?= web/static-dist
WEB_PNPM ?= $(CURDIR)/scripts/web-pnpm.sh
TARGET_OS ?= $(shell $(GO) env GOOS)
TARGET_ARCH ?= $(shell $(GO) env GOARCH)
CLI_BIN ?= $(BIN_DIR)/csgclaw-cli

ACR_REGISTRY ?= opencsg-registry.cn-beijing.cr.aliyuncs.com
# Upstream picoclaw base image default: embed Dockerfile ARG PICOCLAW_IMAGE.
# Optional build/CI override: export PICOCLAW_BASE_IMAGE=registry/.../picoclaw:tag
LOCAL_IMAGE ?= picoclaw:local
DOCKER_EMBED_DOCKER_GOOS ?= linux
DOCKER_EMBED_DOCKER_GOARCH ?= $(TARGET_ARCH)
DOCKER_EMBED_CLI ?= $(BIN_DIR)/csgclaw-cli
PICOCLAW_DOCKER_GOOS ?= $(DOCKER_EMBED_DOCKER_GOOS)
PICOCLAW_DOCKER_GOARCH ?= $(DOCKER_EMBED_DOCKER_GOARCH)
PICOCLAW_DOCKER_CLI ?= $(DOCKER_EMBED_CLI)

.DEFAULT_GOAL := build

.PHONY: help fmt test check-web-toolchain check-web-layout ensure-web-deps web-install web-dev build-web build build-server build-server-bin stage-docker-embed-cli stage-picoclaw-docker-cli sync-docker-embed-image-refs sync-picoclaw-embed-image-refs bump-docker-embed-version bump-picoclaw-embed-version ensure-docker-embed-manifests build-docker-embed-images build-docker-embed-images-only build-docker-embed-runtime-embed build-picoclaw-runtime-embed build-all run clean package package-all release tag push publish build-picoclaw-manager-image build-picoclaw-worker-image

help:
	@printf '%s\n' \
		'make            - build Web UI, ensure embed image refs, bin/csgclaw, bin/csgclaw-cli (no docker images)' \
		'make build      - same as default goal' \
		'make build-all  - build-web, bump embed versions, rebuild binaries, docker-build all embed images' \
		'make fmt        - format Go files' \
		'make test       - ensure embed agent.toml refs, then go test ./...' \
		'make web-install - install Web UI dependencies' \
		'make web-dev    - run Vite Web UI dev server' \
		'make build-web  - build Web UI app into web/static-dist' \
		'make build-server-bin - build bin/csgclaw and bin/csgclaw-cli' \
		'make build-docker-embed-runtime-embed - bump versions, stage linux cli, docker-build all embed templates' \
		'make run        - build (no docker images), then run the server' \
		'make clean      - remove local build outputs'

fmt:
	$(GOFMT) -w $(shell find cli cmd internal web -name '*.go')

test: ensure-docker-embed-manifests
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

build: build-web ensure-docker-embed-manifests build-server-bin

build-server-bin:
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/csgclaw ./cmd/csgclaw
	env GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags "$(CLI_LDFLAGS)" -o $(BIN_DIR)/csgclaw-cli ./cmd/csgclaw-cli

build-server: ensure-docker-embed-manifests build-server-bin

$(DOCKER_EMBED_CLI):
	mkdir -p $(BIN_DIR)
	env GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(DOCKER_EMBED_DOCKER_GOOS) GOARCH=$(DOCKER_EMBED_DOCKER_GOARCH) \
		$(GO) build -ldflags "$(CLI_LDFLAGS)" -o $(DOCKER_EMBED_CLI) ./cmd/csgclaw-cli

stage-docker-embed-cli: $(DOCKER_EMBED_CLI)
stage-picoclaw-docker-cli: stage-docker-embed-cli

sync-docker-embed-image-refs:
	chmod +x scripts/list-docker-embed-templates.sh scripts/sync-docker-embed-image-refs.sh
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/sync-docker-embed-image-refs.sh

sync-picoclaw-embed-image-refs: sync-docker-embed-image-refs

bump-docker-embed-version:
	chmod +x scripts/list-docker-embed-templates.sh scripts/bump-docker-embed-version.sh
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/bump-docker-embed-version.sh

bump-picoclaw-embed-version: bump-docker-embed-version

# Sync embed agent.toml image refs from version when missing or out of sync (no docker).
ensure-docker-embed-manifests:
	@mkdir -p "$(GOCACHE)"
	@chmod +x scripts/list-docker-embed-templates.sh scripts/check-docker-embed-manifests.sh
	@if ! scripts/check-docker-embed-manifests.sh; then \
	  printf '%s\n' "docker embed agent.toml version/ref out of sync; running sync-docker-embed-image-refs"; \
	  $(MAKE) sync-docker-embed-image-refs; \
	fi

build-docker-embed-images: stage-docker-embed-cli bump-docker-embed-version
	chmod +x scripts/build-docker-embed-images.sh scripts/read-picoclaw-base-image.sh
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/build-docker-embed-images.sh

build-docker-embed-images-only: stage-docker-embed-cli
	chmod +x scripts/build-docker-embed-images.sh scripts/read-picoclaw-base-image.sh
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/build-docker-embed-images.sh

build-docker-embed-runtime-embed: build-docker-embed-images
build-picoclaw-runtime-embed: build-docker-embed-runtime-embed

build-picoclaw-manager-image: stage-docker-embed-cli
	chmod +x scripts/bump-docker-embed-version.sh scripts/build-docker-embed-images.sh scripts/read-picoclaw-base-image.sh
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/bump-docker-embed-version.sh picoclaw-manager
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/build-docker-embed-images.sh picoclaw-manager

build-picoclaw-worker-image: stage-docker-embed-cli
	chmod +x scripts/bump-docker-embed-version.sh scripts/build-docker-embed-images.sh scripts/read-picoclaw-base-image.sh
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/bump-docker-embed-version.sh picoclaw-worker
	ACR_REGISTRY="$(ACR_REGISTRY)" \
		scripts/build-docker-embed-images.sh picoclaw-worker

# Bump embed versions before go:embed so the server binary matches built images.
build-all: build-web bump-docker-embed-version build-server-bin
	$(MAKE) build-docker-embed-images-only

run: build
	$(BIN) serve

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
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux amd64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=1 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux arm64
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) DIST_DIR=$(DIST_DIR) APP=csgclaw-cli GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 BOXLITE_CLI_VERSION=$(BOXLITE_CLI_VERSION) BOXLITE_CLI_BASE_URL=$(BOXLITE_CLI_BASE_URL) $(CURDIR)/scripts/package-release.sh linux arm64

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(GOCACHE)

tag:
	chmod +x scripts/read-picoclaw-base-image.sh
	docker tag $(LOCAL_IMAGE) $$(scripts/read-picoclaw-base-image.sh)

push:
	chmod +x scripts/read-picoclaw-base-image.sh
	docker push $$(scripts/read-picoclaw-base-image.sh)

publish: tag push
