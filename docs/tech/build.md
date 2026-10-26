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

## 镜像内预装 CLI 连接器

镜像构建可以通过服务端的内部命令生成完整安装包：

```bash
csgclaw _prepare-connector --id feishu --directory /opt/csgclaw/connector-packages
chmod -R a+rX /opt/csgclaw/connector-packages
```

命令使用当前内置连接器声明和通用安装器，下载并校验对应平台的 CLI、Skills，
生成 `<directory>/<executable>/<version>/{bin,skills,package.json}`，
标准输出为可执行文件的绝对路径。构建阶段需要下载权限，命令不创建连接器资源或账号配置。

运行服务时设置 `CSGCLAW_CONNECTOR_PACKAGES_DIR`，值为上述预装目录。
普通连接和旧配置迁移共用安装流程：有效的资源安装直接复用；需要安装时，
匹配版本的预装包复制到资源目录中的临时目录，通过现有校验后完成目录替换。
最终位置仍为 `~/.csgclaw/apps/<resource-id>/package/<version>/`。
预装目录可以只读，账号配置继续位于资源的 `config/` 中。

未设置预装目录、缺少对应版本，或者 Skills 版本不匹配时，继续在线下载。
已匹配但内容损坏的预装包返回错误，保留现有资源文件。
独立升级 CSGClaw 后，需要镜像提供新的匹配包才能继续离线安装。
该目录应由部署管理员提供，包含可信的可执行文件；复制流程拒绝符号链接和特殊文件。

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
| `make desktop-package-macos-signed VERSION=<version>` | Build, sign, notarize, verify, and locally stage macOS packages. Defaults to both architectures. Use `DESKTOP_MACOS_TARGETS=arm64` or `amd64` for the current chip only, and `DESKTOP_MACOS_FORCE=1` to overwrite an existing local archive. Full commands: [Windows and macOS signing guide](desktop/electron-desktop-signing.zh.md) |
| `scripts\build.cmd desktop-package` | Build native desktop installers on Windows without `make` |
| `make desktop-package-oss VERSION=<version>` | Reuse `desktop-package` and stage this host's OSS desktop artifacts |
| `make desktop-oss-manifest VERSION=<version>` | Validate all three website installers and generate `downloads.json` |
| `make desktop-oss-publish VERSION=<version>` | Validate and publish all three website installers to OSS |
| `make desktop-oss-upload-packages VERSION=<version> DESKTOP_OSS_TARGETS=<os-arch>` | Upload only the selected immutable desktop packages without changing OSS channel JSON |
| `make release` | Build the configured cross-platform release bundles |

Release CI uses `.github/workflows/release.yml` and `.gitlab/ci.yml`. GitHub tag pushes keep the full release path: they attach the server/CLI bundles and native desktop installers to the matching GitHub Release, then publish the complete server/CLI bundle matrix, the two macOS DMGs, the Windows x64 installer, and native Electron update feeds to the beta or release OSS channel. A manual GitHub workflow run accepts only an alpha version matching `vX.Y.Z-alpha.N` and one `desktop_target`; it builds that desktop target, uploads its immutable packages without changing `downloads.json`, `RELEASES`, or `RELEASES.json`, and shows the download URLs in the Actions summary. GitLab keeps its existing SSH upload to `https://csgclaw.opencsg.com/releases/<tag>/` and publishes the CSGClaw product image. Its desktop jobs are optional manual builds: successful installers remain GitLab artifacts for one day and are not uploaded to the public release directory. GitLab macOS and Windows desktop jobs require native runners tagged `csgclaw-macos-arm64`, `csgclaw-macos-amd64`, and `csgclaw-windows-amd64`.

Desktop installers use names such as `csgclaw-desktop_v0.4.3_darwin_arm64.dmg`. The ordinary `desktop-package` target keeps the macOS ad-hoc and Windows unsigned defaults when credentials are absent. The strict local macOS target requires a Developer ID identity plus complete notarization credentials and fails instead of emitting a partial signature. It reuses the same Forge package target and asset collector as CI while leaving the established GitHub workflow unchanged; it writes only under `desktop/out/local/` and never uploads to OSS. GitHub Release CI configures channel-isolated Electron feeds under `channels/<channel>/updates/`; the desktop checks in the background and only prompts after the native package has downloaded.

Local desktop outputs are kept under `desktop/out/`: backend inputs in `input/`, raw Forge artifacts in `make/`, and OSS release files and manifests in `oss/`. The repository-root `dist/` remains the Go release and CI runner staging directory.

Failed GitHub `Release` runs are handled by `.github/workflows/release-retry.yml` after each attempt completes. It re-runs only failed jobs and their dependent jobs, retaining successful platform builds and artifacts. Each release run gets at most three total attempts (the initial execution plus two automatic retries); cancellation stops automatic retries, and failures at the limit remain visible for manual investigation. Manual re-runs count toward the same limit. The retry workflow must be merged into the default branch before it takes effect.

## Related docs

- [Configuration](config.md)
- [Architecture](architecture.md)
- [Web development](web/development.md)
- [Windows and macOS signing guide](desktop/electron-desktop-signing.zh.md)
