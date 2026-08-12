APP ?= csgclaw
BIN_DIR ?= bin
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
NODE ?= node
CGO_ENABLED ?= 0
WEB_APP_DIR ?= web/app
WEB_STATIC_DIST_DIR ?= web/static-dist
WEB_PNPM ?= $(CURDIR)/scripts/web-pnpm.sh
WEB_BUILD_REPORT ?= summary
WEB_TYPECHECK_PNPM_ARGS ?= $(if $(filter summary,$(WEB_BUILD_REPORT)),--silent typecheck,typecheck)
WEB_BUILD_PNPM_ARGS ?= $(if $(filter summary,$(WEB_BUILD_REPORT)),--silent exec vite build --logLevel warn,exec vite build)
DESKTOP_DIR ?= desktop
DESKTOP_PNPM ?= $(CURDIR)/scripts/desktop-pnpm.sh
DESKTOP_PACKAGE_REPORT ?= summary
DESKTOP_OSS_RELEASE_SCRIPT ?= $(CURDIR)/scripts/desktop-oss-release.mjs
DESKTOP_OSS_CHANNEL ?=
DESKTOP_OSS_TARGETS ?=
DESKTOP_OSS_RELEASE_DIR ?=
DESKTOP_OSS_OUTPUT_ROOT ?=
DESKTOP_OSS_ENV_FILE ?=
DESKTOP_OSS_FORCE ?= 0
DESKTOP_OSS_ALLOW_PARTIAL ?= 0
DESKTOP_OSS_REQUIRE_PACKAGES ?= 0
DESKTOP_OSS_COMMON_ARGS = --version "$(VERSION)" $(if $(strip $(DESKTOP_OSS_CHANNEL)),--channel "$(DESKTOP_OSS_CHANNEL)",) $(if $(strip $(DESKTOP_OSS_RELEASE_DIR)),--release-directory "$(DESKTOP_OSS_RELEASE_DIR)",) $(if $(strip $(DESKTOP_OSS_OUTPUT_ROOT)),--output-root "$(DESKTOP_OSS_OUTPUT_ROOT)",) $(if $(filter 1 true yes,$(DESKTOP_OSS_REQUIRE_PACKAGES)),--require-packages,)
TARGET_OS ?= $(shell $(GO) env GOOS)
TARGET_ARCH ?= $(shell $(GO) env GOARCH)
DESKTOP_PLATFORM ?= $(if $(filter windows,$(TARGET_OS)),win32,$(TARGET_OS))
DESKTOP_ARCH ?= $(if $(filter amd64,$(TARGET_ARCH)),x64,$(TARGET_ARCH))
HOST_BINARY_SUFFIX := $(if $(filter windows,$(TARGET_OS)),.exe,)
SERVER_BIN ?= $(BIN_DIR)/$(APP)$(HOST_BINARY_SUFFIX)
HOST_CLI_BIN ?= $(BIN_DIR)/csgclaw-cli$(HOST_BINARY_SUFFIX)
BIN ?= $(SERVER_BIN)
SANDBOX_BUNDLE_TOOLS_DIR ?= $(BIN_DIR)/sandbox-tools
SANDBOX_CLI_BIN ?= $(SANDBOX_BUNDLE_TOOLS_DIR)/csgclaw-cli

.DEFAULT_GOAL := build

.PHONY: help fmt test check-web-toolchain check-web-layout ensure-web-deps web-install web-dev web-typecheck web-build-assets build-web check-desktop-layout ensure-desktop-deps desktop-dev desktop-backend-bundle desktop-package desktop-package-oss desktop-oss-manifest desktop-oss-publish desktop-oss-release build build-all build-server build-server-bin build-codex-cli build-sandbox-cli install-sandbox-cli run clean package package-all release

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
		'make desktop-dev - install dependencies when needed, build the local backend, and start Electron Forge' \
		'make desktop-package - create platform Electron installers/archives (set CSGCLAW_DESKTOP_WINDOWS_CHANNEL=store for MSIX)' \
		'make desktop-package-oss - build and stage this host desktop OSS artifacts' \
		'make desktop-oss-manifest - validate the collected macOS/Windows installers and write downloads.json' \
		'make desktop-oss-publish - validate the complete installer set, then upload artifacts and downloads.json to OSS' \
		'make build-server-bin - build bin/csgclaw, bin/csgclaw-cli, and bundled Codex' \
		'make build-sandbox-cli - build Linux csgclaw-cli into bin/sandbox-tools' \
		'make run        - build (no docker images), then run the server' \
		'make clean      - remove local build outputs'

fmt:
	$(GOFMT) -w $(shell find cli cmd internal web -name '*.go')

test:
	env GOCACHE=$(GOCACHE) $(GO) test ./...

check-web-toolchain:
	@$(WEB_PNPM) --check

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

web-typecheck:
	@printf '%s\n' "Checking Web UI TypeScript..."
	@$(WEB_PNPM) $(WEB_TYPECHECK_PNPM_ARGS) || { \
		status=$$?; \
		printf '%s\n' "Web UI TypeScript check failed."; \
		exit $$status; \
	}
	@printf '%s\n' "Web UI TypeScript check passed."

web-build-assets:
	@printf '%s\n' "Building Web UI assets..."
	@mkdir -p "$(WEB_STATIC_DIST_DIR)"
	@$(WEB_PNPM) $(WEB_BUILD_PNPM_ARGS) || { \
		status=$$?; \
		printf '%s\n' "Failed to build Web UI."; \
		printf '%s\n' "If the error mentions vite not found, rerun make web-install and check the install output."; \
		exit $$status; \
	}
	@test -f "$(WEB_STATIC_DIST_DIR)/index.html" || { \
		printf '%s\n' "Web UI build did not produce $(WEB_STATIC_DIST_DIR)/index.html."; \
		exit 1; \
	}
	@if [ "$(WEB_BUILD_REPORT)" = "summary" ]; then \
		file_count=$$(find "$(WEB_STATIC_DIST_DIR)" -type f | wc -l | tr -d '[:space:]'); \
		total_size=$$(du -sh "$(WEB_STATIC_DIST_DIR)" | awk '{print $$1}'); \
		printf 'Web UI ready: %s (%s files, %s total).\n' "$(WEB_STATIC_DIST_DIR)" "$$file_count" "$$total_size"; \
	fi

build-web: ensure-web-deps
	+@$(MAKE) --no-print-directory -j2 web-typecheck web-build-assets

check-desktop-layout:
	@if [ ! -d "$(DESKTOP_DIR)" ]; then \
		printf '%s\n' "Electron Desktop source directory is missing: $(DESKTOP_DIR)."; \
		exit 1; \
	fi
	@if [ ! -f "$(DESKTOP_DIR)/package.json" ]; then \
		printf '%s\n' "Electron Desktop package.json is missing: $(DESKTOP_DIR)/package.json."; \
		exit 1; \
	fi
	@if [ ! -f "$(DESKTOP_DIR)/pnpm-lock.yaml" ]; then \
		printf '%s\n' "Electron Desktop pnpm lockfile is missing: $(DESKTOP_DIR)/pnpm-lock.yaml."; \
		exit 1; \
	fi

ensure-desktop-deps: check-desktop-layout
	@if [ ! -d "$(DESKTOP_DIR)/node_modules" ] || [ ! -x "$(DESKTOP_DIR)/node_modules/.bin/electron-forge" ]; then \
		printf '%s\n' "Electron Desktop dependencies are missing; installing them before continuing."; \
		$(DESKTOP_PNPM) install --frozen-lockfile || { \
			status=$$?; \
			printf '%s\n' "Failed to install Electron Desktop dependencies."; \
			printf '%s\n' "Check npm registry network/proxy access, then rerun the make command."; \
			exit $$status; \
		}; \
	fi

desktop-dev: ensure-desktop-deps build-web build-server-bin build-sandbox-cli
	$(DESKTOP_PNPM) start

desktop-backend-bundle: build-web
	@printf 'Building desktop backend (%s/%s)...\n' "$(TARGET_OS)" "$(TARGET_ARCH)"
	@VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) GOCACHE=$(GOCACHE) INCLUDE_BOXLITE=0 $(CURDIR)/scripts/package-desktop-backend.sh $(TARGET_OS) $(TARGET_ARCH)

desktop-package: WEB_BUILD_REPORT := summary
desktop-package: ensure-desktop-deps desktop-backend-bundle
	@printf 'Building desktop packages (%s/%s)...\n' "$(DESKTOP_PLATFORM)" "$(DESKTOP_ARCH)"
	@run_package() { \
		CSGCLAW_DESKTOP_GOOS=$(TARGET_OS) \
		CSGCLAW_DESKTOP_GOARCH=$(TARGET_ARCH) \
		CSGCLAW_DESKTOP_ARCH=$(DESKTOP_ARCH) \
		CSGCLAW_DESKTOP_VERSION=$(VERSION) \
		$(DESKTOP_PNPM) $(if $(filter summary,$(DESKTOP_PACKAGE_REPORT)),--silent make,make) --platform=$(DESKTOP_PLATFORM) --arch=$(DESKTOP_ARCH); \
	}; \
	print_artifacts() { \
		artifacts=$$(find "$(abspath $(DESKTOP_DIR)/out/make)" -type f -size +0c \( \
			-name '*.dmg' -o -name '*.zip' -o -name '*.deb' -o \
			-name '*.rpm' -o -name '*-Setup.exe' -o -name '*.msi' -o \
			-name '*.msix' \
		\) -print 2>/dev/null); \
		if [ -z "$$artifacts" ]; then \
			printf 'Desktop packaging produced no distributables under %s\n' "$(abspath $(DESKTOP_DIR)/out/make)" >&2; \
			return 1; \
		fi; \
		printf '%s\n' "Desktop packages ready:"; \
		printf '%s\n' "$$artifacts" | sed 's/^/  /'; \
	}; \
	rm -rf "$(DESKTOP_DIR)/out/make"; \
	if [ "$(DESKTOP_PACKAGE_REPORT)" = "summary" ]; then \
		log_file=$$(mktemp); \
		trap 'rm -f "$$log_file"' EXIT; \
		run_package >"$$log_file" 2>&1 & \
		package_pid=$$!; \
		elapsed=0; \
		while kill -0 "$$package_pid" 2>/dev/null; do \
			if [ -t 1 ]; then printf '\rPackaging in progress... %ss' "$$elapsed"; fi; \
			sleep 1; \
			elapsed=$$((elapsed + 1)); \
		done; \
		wait "$$package_pid"; \
		status=$$?; \
		if [ -t 1 ]; then printf '\rPackaging finished in %ss.   \n' "$$elapsed"; fi; \
		if [ $$status -ne 0 ]; then \
			printf '%s\n' "Desktop packaging failed; Electron Forge output follows:" >&2; \
			cat "$$log_file" >&2; \
			exit $$status; \
		fi; \
		print_artifacts || { \
			printf '%s\n' "Electron Forge output follows:" >&2; \
			cat "$$log_file" >&2; \
			exit 1; \
		}; \
	else \
		run_package && print_artifacts; \
	fi

# These are thin release wrappers. desktop-package remains the single-platform
# build primitive used by CI; the Node script calls it and then reuses the same
# release asset collector as the existing GitHub workflow.
desktop-package-oss:
	@$(NODE) "$(DESKTOP_OSS_RELEASE_SCRIPT)" build $(DESKTOP_OSS_COMMON_ARGS) $(if $(strip $(DESKTOP_OSS_TARGETS)),--targets "$(DESKTOP_OSS_TARGETS)",) $(if $(filter 1 true yes,$(DESKTOP_OSS_FORCE)),--force,)

desktop-oss-manifest:
	@$(NODE) "$(DESKTOP_OSS_RELEASE_SCRIPT)" manifest $(DESKTOP_OSS_COMMON_ARGS) $(if $(filter 1 true yes,$(DESKTOP_OSS_ALLOW_PARTIAL)),--allow-partial,)

desktop-oss-publish:
	@$(NODE) "$(DESKTOP_OSS_RELEASE_SCRIPT)" manifest $(DESKTOP_OSS_COMMON_ARGS)
	@$(NODE) "$(DESKTOP_OSS_RELEASE_SCRIPT)" upload $(DESKTOP_OSS_COMMON_ARGS) $(if $(strip $(DESKTOP_OSS_ENV_FILE)),--env-file "$(DESKTOP_OSS_ENV_FILE)",)

desktop-oss-release: desktop-oss-publish

build: build-web build-server-bin build-sandbox-cli

build-all: build

build-server-bin: build-codex-cli
	mkdir -p $(BIN_DIR)
	@if [ "$(TARGET_OS)" = "windows" ]; then \
		rm -f "$(BIN_DIR)/csgclaw" "$(BIN_DIR)/csgclaw-cli"; \
	else \
		rm -f "$(BIN_DIR)/csgclaw.exe" "$(BIN_DIR)/csgclaw-cli.exe"; \
	fi
	env GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags "$(LDFLAGS)" -o "$(SERVER_BIN)" ./cmd/csgclaw
	env GOCACHE=$(GOCACHE) CGO_ENABLED=$(CGO_ENABLED) GOOS=$(TARGET_OS) GOARCH=$(TARGET_ARCH) \
		$(GO) build -ldflags "$(CLI_LDFLAGS)" -o "$(HOST_CLI_BIN)" ./cmd/csgclaw-cli

build-codex-cli:
	mkdir -p "$(BIN_DIR)"
	"$(CURDIR)/scripts/fetch-codex-cli.sh" "$(TARGET_OS)" "$(TARGET_ARCH)" "$(BIN_DIR)"

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
