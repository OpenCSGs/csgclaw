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
2. Embed template `dist/` trees when missing (see [Embed dist](#embed-dist))
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

`build-all` runs `build`, then docker-builds all embed templates with a `Dockerfile` and patches image refs:

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

## Embed dist

Builtin PicoClaw templates ship source under `internal/templates/embed/<name>/`. Runtime files for `go:embed` live in each template's `dist/` directory and are generated locally or in CI.

| Target | Description |
|--------|-------------|
| `make prepare-docker-embed-dist` | Copy template source into `embed/*/dist/` (no Docker) |
| `make patch-docker-embed-image-refs` | Patch `dist/agent.toml` image refs (default tag `:dev`) |
| `make stage-docker-embed-dist` | `prepare` + `patch` |
| `make ensure-docker-embed-dist` | Stage dist when missing (used by `build-all`) |

Templates with a `Dockerfile` are discovered by `scripts/list-docker-embed-templates.sh` (currently `picoclaw-manager` and `picoclaw-worker`).

If `make build-server-bin` fails with `pattern embed/.../dist: no matching files found`, run:

```bash
make stage-docker-embed-dist
```

## Docker embed images (optional)

Local Docker image builds are **optional** and can be slow. The usual entry point is `make build-all`. Use individual targets when you only need one image.

### When images are built

| Command | Builds Docker images? |
|---------|------------------------|
| `make` / `make build` | No |
| `make build-all` | Yes |
| `make build-docker-embed-runtime-embed` | Yes (images + patch refs, without rebuilding binaries) |

### Image build targets

| Target | Description |
|--------|-------------|
| `make build-docker-embed-images` | Build all embed templates that have a `Dockerfile` |
| `make build-picoclaw-manager-image` | Build manager image only |
| `make build-picoclaw-worker-image` | Build worker image only |
| `make build-docker-embed-runtime-embed` | Build linux CLI, docker-build all templates, patch refs |

Alias targets `build-picoclaw-runtime-embed`, `stage-picoclaw-embed-dist`, and similar names remain for compatibility.

### Useful variables

```bash
# Registry and tags (defaults shown)
ACR_REGISTRY=opencsg-registry.cn-beijing.cr.aliyuncs.com
DOCKER_EMBED_IMAGE_TAG=dev
PICOCLAW_BASE_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.8

# Example: build worker image with a custom tag
make build-picoclaw-worker-image DOCKER_EMBED_IMAGE_TAG=local
```

Resulting images follow:

```text
${ACR_REGISTRY}/opencsghq/picoclaw-manager:${DOCKER_EMBED_IMAGE_TAG}
${ACR_REGISTRY}/opencsghq/picoclaw-worker:${DOCKER_EMBED_IMAGE_TAG}
```

Build context is the repository root; Dockerfiles expect `bin/csgclaw-cli` (linux) to be present—`stage-docker-embed-cli` creates it.

### BoxLite vs local Docker images

Images built with `make build-docker-embed-images` are stored in the **Docker** image store. **BoxLite does not use Docker images directly**; pull images into BoxLite with `boxlite pull …` or use a registry BoxLite can reach. See [docs/config.md](config.md) for sandbox provider settings.

## Test, format, and clean

| Target | Description |
|--------|-------------|
| `make test` | `stage-docker-embed-dist`, then `go test ./...` |
| `make fmt` | Format Go sources under `cli/`, `cmd/`, `internal/`, `web/` |
| `make clean` | Remove `bin/`, `dist/`, `.gocache/`, and generated embed `dist/` contents |

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

CI release flows use `.github/workflows/release.yml` and GitLab jobs for embed dist and image builds.

## Related docs

- [docs/config.md](config.md) — sandbox providers (BoxLite, Docker, CSGHub)
- [docs/web/development.md](web/development.md) — Web UI development
- [docs/architecture.md](architecture.md) — system layout
- `Makefile` — source of truth for targets and defaults
