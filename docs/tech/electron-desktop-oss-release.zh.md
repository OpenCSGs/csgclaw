# CSGClaw Desktop OSS 发布

桌面端官网下载安装包沿用 CSGLite 的 channel manifest 结构：版本文件不可变，`channels/beta/downloads.json` 和 `channels/release/downloads.json` 分别指向当前 beta 和稳定版本。

发布流程只按 SemVer 是否带预发布段分流，构建、归档、校验和上传步骤完全相同：`v0.4.6-beta.1` 进入 `beta` channel，`v0.4.6` 进入 `release` channel。版本必须使用 `-beta.1` 形式；`v0.4.6.beta.1` 不是有效 SemVer，会在构建 matrix 启动前失败。

## 首个 beta 版本

命令接受带或不带 `v` 的 SemVer，例如 `0.4.5-beta.1`。版本目录和 `downloads.json` 中的版本号不带 `v`；安装包文件名保留现有 GitHub Release 的 `v` 前缀，例如 `csgclaw-desktop_v0.4.5-beta.1_darwin_arm64.dmg`。

在 macOS 上同时构建 Apple Silicon 和 Intel 包：

```bash
make desktop-package-oss \
  VERSION=0.4.5-beta.1 \
  DESKTOP_OSS_TARGETS=darwin-arm64,darwin-amd64 \
  DESKTOP_OSS_FORCE=1
```

在 Apple Silicon Mac 上交叉构建 Intel 且没有配置正式证书时，脚本会跳过容易失败的临时 ad-hoc 签名。该包仅用于内部验证；公开上传前必须在签名构建机或 macOS CI 中配置 Developer ID，并完成签名、公证和 staple。

Windows Squirrel 安装器必须在 Windows 上构建。在 Windows PowerShell 或 Windows CI 中执行：

```powershell
node scripts/desktop-oss-release.mjs build `
  --version 0.4.5-beta.1 `
  --channel beta `
  --targets windows-amd64 `
  --force
```

把两台构建机生成的文件合并到同一个目录：

```text
desktop/out/oss/releases/0.4.5-beta.1/
```

生成官网清单：

```bash
make desktop-oss-manifest VERSION=0.4.5-beta.1
```

生成位置：

```text
desktop/out/oss/channels/beta/downloads.json
```

## 上传 OSS

安装并配置 `ossutil` 后，复制凭证模板：

```bash
cp .desktop-release-oss.env.example .desktop-release-oss.env
```

只在本地 `.desktop-release-oss.env` 填写 `OSS_ACCESS_KEY_ID` 和 `OSS_ACCESS_KEY_SECRET`。该文件已加入 `.gitignore`。

上传时先传不可变的版本文件，最后覆盖 channel manifest：

```bash
make desktop-oss-publish VERSION=0.4.5-beta.1
```

`make desktop-oss-release` 是 `desktop-oss-publish` 的发布别名。两个命令都会先严格检查 macOS arm64、macOS Intel 和 Windows x64 三个安装器是否已经收齐；不完整时不会上传。脚本上传前会读取远端现有 `downloads.json` 并保留历史版本；上传成功后会再次读取公开清单，验证 `latest` 是否等于本次版本。

## GitHub Release 与 OSS 自动发布

`.github/workflows/release.yml` 与本地流程复用相同的三层能力：

- 构建层：`make desktop-package` / `scripts/build.cmd desktop-package`，继续统一调用 Electron Forge；
- 产物契约层：`desktop-release-artifacts.mjs`，集中维护版本、平台和最终文件名；
- 归档层：`collect-desktop-release-assets.mjs`，按产物契约筛选 Forge 输出，供现有 CI 和本地脚本共同调用。

`desktop-oss-release.mjs build` 是本地编排层，只组合以上能力，不另写一套打包实现。CI 继续负责各平台 runner、macOS/Windows 签名、公证以及 GitHub Release；本地和 CI 生成的文件可直接交给同一套 manifest/upload 命令。

GitHub Release 成功后，`publish-desktop-oss` job 会重新下载 `desktop-dist-*`，严格确认 Mac ARM DMG、Mac Intel DMG 和 Windows x64 Setup EXE 均存在，然后安装经过 SHA-256 校验的 `ossutil` 并执行：

```bash
make desktop-oss-publish \
  VERSION="$VERSION" \
  DESKTOP_OSS_RELEASE_DIR=desktop-release
```

GitHub Release 中仍保留 macOS ZIP 和 Linux DEB，但 OSS 上传只包含官网清单使用的三个安装器，不会上传 Linux 包。预发布版本自动发布到 `beta` channel，稳定版本自动发布到 `release` channel；两个 channel 共用 `oss-publish` GitHub Environment，只通过不同的 `downloads.json` 路径分流。

在仓库 Settings → Environments 中创建 `oss-publish`，并配置以下 secrets：

- `OSS_ACCESS_KEY_ID`
- `OSS_ACCESS_KEY_SECRET`

`oss-publish` 只负责为最终上传 job 提供凭证和部署记录；版本中的预发布段仍由 `release_channel` 单独解析，不影响凭证选择。如果将来稳定 channel 需要独立的人工审批或独立凭证，再拆分为两个受保护的 GitHub Environment。

OSS 区域、endpoint、bucket、对象前缀和公开地址由发布脚本统一维护，GitHub Environment 不需要重复配置。CI 只读取上述两项 secrets。

OSS job 不完整、凭证缺失、上传失败或公开清单回读验证失败时会明确失败。版本对象先上传，最后才替换 channel manifest。

可选 Make 变量：

| 变量 | 用途 |
|---|---|
| `DESKTOP_OSS_TARGETS` | 本机构建目标，逗号分隔 |
| `DESKTOP_OSS_RELEASE_DIR` | 已汇总安装包的目录；CI 使用 `desktop-release` |
| `DESKTOP_OSS_OUTPUT_ROOT` | 覆盖默认的 `desktop/out/oss` 工作区 |
| `DESKTOP_OSS_CHANNEL` | 显式指定 `beta` 或 `release` |
| `DESKTOP_OSS_ENV_FILE` | 指定本地 OSS 凭证文件 |
| `DESKTOP_OSS_FORCE=1` | 覆盖本地已有的同版本归档 |
| `DESKTOP_OSS_ALLOW_PARTIAL=1` | 仅供本地检查部分 manifest；发布命令不会使用 |
