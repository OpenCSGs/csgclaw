# Build and release

This document describes the repository build, test, and release commands.

## Default build

```bash
make
# same as: make build
```

The default build:

1. Builds the Web UI into `web/static-dist/`.
2. Builds `bin/csgclaw`, the host-platform `bin/csgclaw-cli`, and the bundled `bin/codex`.
3. Builds a static Linux `csgclaw-cli` for the current CPU architecture into `bin/sandbox-tools/csgclaw-cli`.

`make build-all` is retained as an alias of `make build`. CSGClaw no longer builds derived PicoClaw/OpenClaw images locally.

Useful targets:

| Target | Description |
|---|---|
| `make build-server-bin` | Build `bin/csgclaw`, host-platform `bin/csgclaw-cli`, and bundled `bin/codex` |
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
.\scripts\build.cmd desktop-package
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
2. Builds `bin/csgclaw.exe`, the host-platform `bin/csgclaw-cli.exe`, and bundled `bin/codex.exe`.
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
    codex[.exe]                  # bundled Codex CLI for the release host platform
    boxlite[.exe]                 # supported platforms only
    sandbox-tools/
      csgclaw-cli                # Linux, same CPU architecture as the release
```

The canonical CSGClaw-platform-to-Codex-artifact mapping is in
[`scripts/codex-cli-platforms.txt`](../../scripts/codex-cli-platforms.txt). Shell,
PowerShell, and Docker packaging consume that file; formal release targets remain
in [`scripts/release-platforms.txt`](../../scripts/release-platforms.txt).

The installer exposes both host binaries from the same `INSTALL_DIR` and copies the sandbox CLI to `~/.csgclaw/sandbox-tools/csgclaw-cli`. Bundle replacement during upgrade updates both companion host binaries together, and built-in upgrade creates or refreshes a missing companion entry for older installer layouts. Runtime asset refresh only synchronizes the sandbox CLI. Older bundles that stored it at `bin/csgclaw_dir/csgclaw-cli` remain readable during upgrades.

| Target | Description |
|---|---|
| `make package` | Package the current platform |
| `make package-all` | Build and package the current platform artifacts |
| `make desktop-package` | Build native desktop installers on macOS/Linux |
| `scripts\build.cmd desktop-package` | Build native desktop installers on Windows without `make` |
| `make desktop-package-oss VERSION=<version>` | Reuse `desktop-package` and stage this host's OSS desktop artifacts |
| `make desktop-oss-manifest VERSION=<version>` | Validate all three website installers and generate `downloads.json` |
| `make desktop-oss-publish VERSION=<version>` | Validate and publish all three website installers to OSS |
| `make release` | Build the configured cross-platform release bundles |

Release CI uses `.github/workflows/release.yml` and `.gitlab/ci.yml`. GitHub attaches the server/CLI bundles and native desktop installers to the matching GitHub Release. It also publishes the server/CLI bundle matrix, the two macOS DMGs, the Windows x64 installer, and the native Electron update feeds to the beta or release OSS channel. GitLab keeps its existing SSH upload to `https://csgclaw.opencsg.com/releases/<tag>/` and publishes the CSGClaw product image. Its desktop jobs are optional manual builds: successful installers remain GitLab artifacts for one day and are not uploaded to the public release directory. GitLab macOS and Windows desktop jobs require native runners tagged `csgclaw-macos-arm64`, `csgclaw-macos-amd64`, and `csgclaw-windows-amd64`.

Desktop installers use names such as `csgclaw-desktop_v0.4.3_darwin_arm64.dmg`. Signing and notarization values are optional CI secrets/variables: when they are absent or incomplete, Electron Forge keeps its macOS ad-hoc and Windows unsigned defaults. GitHub Release CI configures channel-isolated Electron feeds under `channels/<channel>/updates/`; the desktop checks in the background and only prompts after the native package has downloaded.

Local desktop outputs are kept under `desktop/out/`: backend inputs in `input/`, raw Forge artifacts in `make/`, and OSS release files and manifests in `oss/`. The repository-root `dist/` remains the Go release and CI runner staging directory.

## Related docs

- [Configuration](config.md)
- [Architecture](architecture.md)
- [Web development](web/development.md)
