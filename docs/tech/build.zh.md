# 构建与发布

本文说明仓库中的构建、测试与发布命令。

## 默认构建

```bash
make
# 等价于：make build
```

默认构建会：

1. 将 Web UI 构建到 `web/static-dist/`。
2. 构建 `bin/csgclaw`、宿主平台的 `bin/csgclaw-cli` 和 bundled `bin/codex`。
3. 按当前 CPU 架构构建静态 Linux `csgclaw-cli` 到 `bin/sandbox-tools/csgclaw-cli`。

`make build-all` 保留为 `make build` 的别名。CSGClaw 不再在本地构建派生的 PicoClaw/OpenClaw 镜像。

常用 target：

| Target | 说明 |
|---|---|
| `make build-server-bin` | 构建 `bin/csgclaw`、宿主平台的 `bin/csgclaw-cli` 和 bundled `bin/codex` |
| `make build-sandbox-cli` | 将 Linux `csgclaw-cli` 构建到 `bin/sandbox-tools` |
| `make install-sandbox-cli` | `make build-sandbox-cli` 的兼容别名 |
| `make run` | 构建并运行 `csgclaw serve` |
| `make fmt` | 格式化 Go 源码 |
| `make test` | 运行 `go test ./...` |

可以通过 `SANDBOX_BUNDLE_TOOLS_DIR=/path make build-sandbox-cli` 覆盖本地 bundle 输出目录。

## Windows 无 make 时

如果 Windows 环境没有 `make`，可以直接使用 PowerShell 构建脚本：

```powershell
.\scripts\build.cmd build
.\scripts\build.cmd build-server-bin
.\scripts\build.cmd build-sandbox-cli
.\scripts\build.cmd desktop-package
.\scripts\build.cmd test
```

`build.cmd` 包装器会以当前进程级别的 `-ExecutionPolicy Bypass` 调用
`scripts/build.ps1`，不需要修改整机 PowerShell 执行策略。如果直接调用
PowerShell 脚本，请使用：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 build
```

其中默认的 `build` 会对齐 `make build` 的行为：

1. 构建 Web UI 到 `web/static-dist/`。
2. 构建 `bin/csgclaw.exe`、宿主平台的 `bin/csgclaw-cli.exe` 和 bundled `bin/codex.exe`。
3. 构建 Linux 版 `csgclaw-cli` 到 `bin/sandbox-tools/csgclaw-cli`。

本地编译的 `bin/csgclaw[.exe]` 启动沙盒时，会把相邻的 `bin/sandbox-tools/csgclaw-cli` 同步到 `~/.csgclaw/sandbox-tools/csgclaw-cli`，然后将这个托管目录只读挂载到沙盒内的 `/opt/csgclaw/bin`。正式安装器也会从已安装 bundle 完成同样的首次同步。

## 运行时镜像

Sandbox runtime 使用以下固定默认镜像：

| Runtime | 固定镜像 |
|---|---|
| OpenClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260723.2-csgclaw` |
| PicoClaw | `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.6.10` |

OpenClaw 的固定引用保存在内置 `agent.toml` 中。PicoClaw 不再提供内置模板，其引用改为 runtime 默认值。CSGClaw 不负责生成这些镜像 tag，也不在 CI 中构建这些运行时镜像。

## Web UI

| Target | 说明 |
|---|---|
| `make web-install` | 使用固定 pnpm 工具链安装依赖 |
| `make web-dev` | 启动 Vite 开发服务器 |
| `make build-web` | 构建到 `web/static-dist/` |

修改 Vite 应用前请阅读 [Web 开发文档](web/development.zh.md)。

## 打包与发布

每个正式 `csgclaw` bundle 包含：

```text
csgclaw/
  bin/
    csgclaw[.exe]
    csgclaw-cli[.exe]            # release 宿主平台的伴生 CLI
    codex[.exe]                  # release 宿主平台的 bundled Codex CLI
    boxlite[.exe]                 # 仅支持的平台
    sandbox-tools/
      csgclaw-cli                # Linux，CPU 架构与 release 一致
```

CSGClaw 平台到 Codex artifact 的统一映射维护在
[`scripts/codex-cli-platforms.txt`](../../scripts/codex-cli-platforms.txt)；Shell、
PowerShell 和 Docker 打包均读取该文件。正式 release 平台仍维护在
[`scripts/release-platforms.txt`](../../scripts/release-platforms.txt)。

安装脚本会从同一个 `INSTALL_DIR` 暴露两个宿主程序，并把沙盒 CLI 复制到 `~/.csgclaw/sandbox-tools/csgclaw-cli`。升级替换 bundle 时会同时更新两个伴生宿主程序，内置升级也会为旧安装布局创建或刷新缺失的伴生程序入口；runtime asset 刷新只同步沙盒 CLI。升级时仍兼容旧 bundle 的 `bin/csgclaw_dir/csgclaw-cli` 路径。

| Target | 说明 |
|---|---|
| `make package` | 打包当前平台 |
| `make package-all` | 构建并打包当前平台产物 |
| `make desktop-package` | 在 macOS/Linux 构建当前平台的桌面安装包 |
| `scripts\build.cmd desktop-package` | 在 Windows 无需 `make` 构建桌面安装包 |
| `make desktop-package-oss VERSION=<version>` | 复用 `desktop-package` 构建并归档本机支持的 OSS 桌面产物 |
| `make desktop-oss-manifest VERSION=<version>` | 收齐三平台安装器后生成 `downloads.json` |
| `make desktop-oss-publish VERSION=<version>` | 严格检查三平台安装器并发布到 OSS |
| `make release` | 构建配置的跨平台 release bundle |

发布 CI 使用 `.github/workflows/release.yml` 和 `.gitlab/ci.yml`。GitHub 将 Server/CLI bundle 和原生桌面安装包附加到对应的 GitHub Release，并把完整 Server/CLI 平台矩阵、两个 macOS DMG、Windows x64 安装器和 Electron 原生更新 feed 发布到 beta 或 release OSS channel。GitLab 保留原有上传到 `https://csgclaw.opencsg.com/releases/<tag>/` 的 SSH 发布链路，并继续发布 CSGClaw 产品镜像。其桌面 job 是可选手动构建：成功的安装包作为 GitLab artifact 保留一天，不会上传到公开发布目录。GitLab 的 macOS 和 Windows 桌面 job 需要提供并打上 `csgclaw-macos-arm64`、`csgclaw-macos-amd64`、`csgclaw-windows-amd64` tag 的原生 runner。

桌面安装包采用如 `csgclaw-desktop_v0.4.3_darwin_arm64.dmg` 的命名。签名和公证信息是可选 CI secrets/variables：未配置或配置不完整时，Electron Forge 保持 macOS ad-hoc、Windows 未签名的默认行为。GitHub Release CI 会把桌面更新源配置到相互隔离的 `channels/<channel>/updates/`；桌面端后台检查并下载，只有下载完成后才提示用户安装。

OSS 桌面发布命令的完整说明见 [Electron Desktop OSS 发布](desktop/electron-desktop-oss-release.zh.md)。`desktop-package` 仍是单平台原子构建，OSS 上传只放在聚合命令中，避免并行 runner 使用不完整产物发布。

本地桌面构建文件统一维护在 `desktop/out/`：`input/` 是后端 bundle 中间件，`make/` 是 Forge 原始产物，`oss/` 是可上传版本、外部导入包和 channel manifest。仓库根 `dist/` 继续作为 Go release 与 CI runner 的临时平铺汇总目录。

## 相关文档

- [配置](config.zh.md)
- [架构](architecture.md)
- [Web 开发](web/development.zh.md)
