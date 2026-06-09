# Build Guide

English | [中文](build.zh.md)

This document describes the repository `Makefile` targets for local development, testing, packaging, and optional PicoClaw embed image builds.

Run all commands from the repository root. For a quick summary at any time:

```bash
make help
```

## Prerequisites

- Go toolchain (see `go.mod` for the required version)
- `pnpm` for the Web UI (`scripts/web-pnpm.sh` wraps installation checks)
- Docker, when building embed template images locally
- `boxlite` CLI, when running or testing with the BoxLite sandbox provider (see [docs/config.md](config.md))

## Default build

The default goal is `build`:

```bash
make
# same as:
make build
```

This builds:

1. Web UI assets into `web/static-dist/`
2. Embed template `agent.toml` image refs when missing or out of sync with `version` (see [Embed templates](#embed-templates))
3. `bin/csgclaw` (server CLI)
4. `bin/csgclaw-cli` for the current platform

**Docker images are not built.** Use `make build-all` for a full local build including embed template images.

After a successful build:

```bash
./bin/csgclaw serve
# or
make run
```

## Full build

`build-all` builds the Web UI, bumps each embed template's `version` and syncs `image.ref`, rebuilds `csgclaw` (so embed matches image tags), then docker-builds all templates with a `Dockerfile` (**no `dist/` output**):

```bash
make build-all
```

Use this when you need local `picoclaw-manager` / `picoclaw-worker` images. It can be slow.

## Web UI

| Target | Description |
|--------|-------------|
| `make web-install` | Install Web UI dependencies (`pnpm install --frozen-lockfile`) |
| `make web-dev` | Run the Vite dev server |
| `make build-web` | Build the Web UI into `web/static-dist/` |

`make build-web` auto-runs `web-install` when `node_modules` is missing.

Frontend structure and verification details: [docs/web/development.md](web/development.md).

## Go binaries

| Target | Output | Notes |
|--------|--------|-------|
| `make build-server-bin` | `bin/csgclaw`, `bin/csgclaw-cli` | Both binaries for the current platform; CLI uses `CGO_ENABLED=0` |
| `make stage-docker-embed-cli` | `bin/csgclaw-cli` (linux, host arch) | Linux CLI copied into PicoClaw Docker images |

`csgclaw-cli` is built with `CGO_ENABLED=0` so it runs inside musl-based PicoClaw and BoxLite sandbox images. Release CI uses the same setting (`scripts/release-build-all.sh`).

## Embed templates

Builtin PicoClaw templates live under `internal/templates/embed/<name>/` and are embedded directly via `go:embed` (`agent.toml`, `workspace/`, etc.). Each docker embed template carries a `version` field and matching `image.ref`.

**Local (before PR)**: `make build-all` bumps `version`, syncs `image.ref`, rebuilds `csgclaw` (so embed matches image tags), then builds Docker images.

**GitLab CI (main)**: reads committed `version` / `image.ref`, builds and pushes images **without** modifying `agent.toml`; runs when embed `agent.toml` changed in the pushed range (`CI_COMMIT_BEFORE_SHA..HEAD`) or `version` differs from the compare base.

| Target | Description |
|--------|-------------|
| `make sync-docker-embed-image-refs` | Sync `image.ref` from current `version` (no bump) |
| `make bump-docker-embed-version` | Bump `version` and sync `image.ref` for all docker embed templates |
| `make ensure-docker-embed-manifests` | Run `sync-docker-embed-image-refs` when `image.ref` is missing or out of sync with `version` (used by `make build` / `make test`) |

Templates with a `Dockerfile` are discovered by `scripts/list-docker-embed-templates.sh` (currently `picoclaw-manager` and `picoclaw-worker`).

If `image.ref` is empty or out of sync with `version`, run:

```bash
make sync-docker-embed-image-refs
```

## Docker embed images (optional)

Local Docker image builds are **optional** and can be slow. The usual entry point is `make build-all`. Use individual targets when you only need one image.

### When images are built

| Command | Builds Docker images? |
|---------|------------------------|
| `make` / `make build` | No |
| `make build-all` | Yes |
| `make build-docker-embed-runtime-embed` | Yes (bump versions + images, without rebuilding binaries) |

### Image build targets

| Target | Description |
|--------|-------------|
| `make build-docker-embed-images` | Bump versions and build all embed templates that have a `Dockerfile` |
| `make build-picoclaw-manager-image` | Bump manager version and build manager image only |
| `make build-picoclaw-worker-image` | Bump worker version and build worker image only |
| `make build-docker-embed-runtime-embed` | Alias for `build-docker-embed-images` |

Alias targets `build-picoclaw-runtime-embed`, `sync-picoclaw-embed-image-refs`, `bump-picoclaw-embed-version`, and similar names remain for compatibility.

### Useful variables

```bash
# Registry (default shown)
ACR_REGISTRY=opencsg-registry.cn-beijing.cr.aliyuncs.com

# Upstream picoclaw base image defaults to embed Dockerfile ARG PICOCLAW_IMAGE
# Optional override: PICOCLAW_BASE_IMAGE=registry.example/opencsghq/picoclaw:tag make build-all

# Example: local pre-PR image test (bumps version and updates image.ref, e.g. 0.1.0 -> 0.1.1)
make build-all
```

Resulting images follow:

```text
${ACR_REGISTRY}/opencsghq/picoclaw-manager:<agent.toml version>
${ACR_REGISTRY}/opencsghq/picoclaw-worker:<agent.toml version>
```

After a successful image build, the matching `agent.toml` files are updated in place (`version` and `image.ref`).

Build context is the repository root; Dockerfiles expect `bin/csgclaw-cli` (linux) to be present—`stage-docker-embed-cli` creates it.

### BoxLite vs local Docker images

Images built with `make build-docker-embed-images` are stored in the **Docker** image store. **BoxLite does not use Docker images directly**; pull images into BoxLite with `boxlite pull …` or use a registry BoxLite can reach. See [docs/config.md](config.md) for sandbox provider settings.

## Test, format, and clean

| Target | Description |
|--------|-------------|
| `make test` | `ensure-docker-embed-manifests`, then `go test ./...` (does not overwrite committed version/ref) |
| `make fmt` | Format Go sources under `cli/`, `cmd/`, `internal/`, `web/` |
| `make clean` | Remove `bin/`, `dist/`, and `.gocache/` |

For targeted Go tests:

```bash
go test ./internal/agent/ -run TestName
go test ./...
```

## Packaging and release

Maintainer targets:

| Target | Description |
|--------|-------------|
| `make package` | Build Web UI and package current platform binary |
| `make package-all` | Full build plus `csgclaw` and `csgclaw-cli` packages |
| `make release` | Cross-platform release bundles (darwin/linux, arm64/amd64) |

CI release flows use `.github/workflows/release.yml` (tag) and GitLab CI (tag for release archives; **main branch** builds and pushes picoclaw images from committed `version`, without editing `agent.toml`). Tag releases embed `agent.toml` from the tag commit as-is.

## Related docs

- [docs/config.md](config.md) — sandbox providers (BoxLite, Docker, CSGHub)
- [docs/web/development.md](web/development.md) — Web UI development
- [docs/architecture.md](architecture.md) — system layout
- `Makefile` — source of truth for targets and defaults
