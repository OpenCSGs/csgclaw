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
3. Builds a static Linux `csgclaw-cli` for the current CPU architecture into `bin/sandbox-tools/csgclaw-cli`.

`make build-all` is retained as an alias of `make build`. CSGClaw no longer builds derived PicoClaw/OpenClaw images locally.

Useful targets:

| Target | Description |
|---|---|
| `make build-server-bin` | Build `bin/csgclaw` and host-platform `bin/csgclaw-cli` |
| `make build-sandbox-cli` | Build Linux `csgclaw-cli` into `bin/sandbox-tools` |
| `make install-sandbox-cli` | Compatibility alias of `make build-sandbox-cli` |
| `make run` | Build and run `csgclaw serve` |
| `make fmt` | Format Go sources |
| `make test` | Run `go test ./...` |

Override the local bundle destination with `SANDBOX_BUNDLE_TOOLS_DIR=/path make build-sandbox-cli`.

## Windows without make

On Windows hosts that do not have `make`, use the PowerShell build script:

```powershell
.\scripts\build.cmd build
.\scripts\build.cmd build-server-bin
.\scripts\build.cmd build-sandbox-cli
.\scripts\build.cmd test
```

The `build.cmd` wrapper runs `scripts/build.ps1` with `-ExecutionPolicy Bypass`
for the current process only, avoiding machine-wide PowerShell policy changes.
If you call the PowerShell script directly, use:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 build
```

The default `build` target mirrors `make build`:

1. Builds the Web UI into `web/static-dist/`.
2. Builds `bin/csgclaw.exe` and the host-platform `bin/csgclaw-cli.exe`.
3. Builds a Linux `csgclaw-cli` into `bin/sandbox-tools/csgclaw-cli`.

When a locally built `bin/csgclaw[.exe]` starts a sandbox, it synchronizes the adjacent `bin/sandbox-tools/csgclaw-cli` into `~/.csgclaw/sandbox-tools/csgclaw-cli`, then mounts that managed directory read-only at `/opt/csgclaw/bin` inside the sandbox. Official installers perform the same initial synchronization from the installed bundle.

## Runtime images

Sandbox runtimes use these fixed default images:

| Runtime | Fixed image |
|---|---|
| OpenClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260723.2-csgclaw` |
| PicoClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.10` |

The OpenClaw ref is stored in its builtin `agent.toml`. PicoClaw no longer has a builtin template, so its ref is maintained as a runtime default. CSGClaw does not generate these image tags or build the runtime images in CI.

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
    csgclaw-cli[.exe]            # companion CLI for the release host platform
    boxlite[.exe]                 # supported platforms only
    sandbox-tools/
      csgclaw-cli                # Linux, same CPU architecture as the release
```

The installer exposes both host binaries from the same `INSTALL_DIR` and copies the sandbox CLI to `~/.csgclaw/sandbox-tools/csgclaw-cli`. Bundle replacement during upgrade updates both companion host binaries together, and built-in upgrade creates or refreshes a missing companion entry for older installer layouts. Runtime asset refresh only synchronizes the sandbox CLI. Older bundles that stored it at `bin/csgclaw_dir/csgclaw-cli` remain readable during upgrades.

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
