# Build and release

This document describes the repository build, test, and release commands.

## Default build

```bash
make
# same as: make build
```

The default build:

1. Builds the Web UI into `web/static-dist/`.
2. Builds `bin/csgclaw` and the host-platform `bin/csgclaw-cli`.
3. Builds a static Linux `csgclaw-cli` for the current CPU architecture into `~/.csgclaw/sandbox-tools/csgclaw-cli`.

`make build-all` is retained as an alias of `make build`. CSGClaw no longer builds derived PicoClaw/OpenClaw images locally.

Useful targets:

| Target | Description |
|---|---|
| `make build-server-bin` | Build `bin/csgclaw` and host-platform `bin/csgclaw-cli` |
| `make install-sandbox-cli` | Build Linux `csgclaw-cli` into `~/.csgclaw/sandbox-tools` |
| `make run` | Build and run `csgclaw serve` |
| `make fmt` | Format Go sources |
| `make test` | Run `go test ./...` |

Override the sandbox CLI destination with `SANDBOX_TOOLS_DIR=/path make install-sandbox-cli`.

## Runtime images

Manager and worker templates have different embedded workspaces but share one image per runtime:

| Runtime | Fixed image |
|---|---|
| OpenClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260610.2-csgclaw` |
| PicoClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.10` |

These refs are stored directly in the builtin `agent.toml` files. They have no template `version` field, no generated tag, and no CSGClaw CI image-build workflow.

## Web UI

| Target | Description |
|---|---|
| `make web-install` | Install dependencies with the pinned pnpm toolchain |
| `make web-dev` | Start the Vite development server |
| `make build-web` | Build assets into `web/static-dist/` |

See [web development](web/development.md) before changing the Vite application.

## Packaging and release

Every official `csgclaw` bundle contains:

```text
csgclaw/
  bin/
    csgclaw[.exe]
    boxlite[.exe]                 # supported platforms only
    csgclaw_dir/
      csgclaw-cli                # Linux, same CPU architecture as the release
```

The installer copies the sandbox CLI to `~/.csgclaw/sandbox-tools/csgclaw-cli`. Runtime startup also synchronizes it from the installed bundle for upgrade compatibility.

| Target | Description |
|---|---|
| `make package` | Package the current platform |
| `make package-all` | Build and package the current platform artifacts |
| `make release` | Build the configured cross-platform release bundles |

Release CI uses `.github/workflows/release.yml` and `.gitlab/ci.yml`. GitLab CI publishes CSGClaw release artifacts and the CSGClaw product image; it does not build PicoClaw/OpenClaw runtime images.

## Related docs

- [Configuration](config.md)
- [Architecture](architecture.md)
- [Web development](web/development.md)
