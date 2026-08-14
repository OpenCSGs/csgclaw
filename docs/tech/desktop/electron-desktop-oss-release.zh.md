# CSGClaw GitHub Release 与 Desktop OSS 线上发布

本文面向 CSGClaw 维护者，说明如何通过 GitHub 页面发布 alpha、beta 和正式版本。

发布不需要在维护者电脑上手工打包或上传 OSS。

## 最简发布流程

先按使用场景选择版本类型：

- **Alpha**：快速验证单个平台桌面包，适合给测试发独立包或复用 CI 的 macOS/Windows
  签名环境；不算完整发布。
- **Beta（预览版）**：迭代中的全平台测试或试用版本。
- **正式版**：迭代完成且验证通过后的正式发布，仅限有发布权限的维护者执行。

发布前确认目标代码已经合入 upstream `main`，然后按下表操作：

| 类型 | 版本号示例 | GitHub 入口 | 操作 |
|---|---|---|---|
| Alpha | `v0.2.1-alpha.1` | [Actions → Release](https://github.com/OpenCSGs/csgclaw/actions/workflows/release.yml) → **Run workflow** | 分支选 `main`，填写版本号和一个 `desktop_target` 后运行 |
| Beta | `v0.2.1-beta.1` | [Releases → Draft a new release](https://github.com/OpenCSGs/csgclaw/releases/new) | 从 `main` 创建同名 tag，勾选 **This is a pre-release** 后发布 |
| 正式版 | `v0.2.1` | [Releases → Draft a new release](https://github.com/OpenCSGs/csgclaw/releases/new) | 从 `main` 创建同名 tag，不勾选 pre-release，保持 **Set as the latest release** 后发布 |

Alpha 手动流程只发布所选平台的桌面包，不创建 GitHub Release，也不更新 beta/release
manifest 或自动更新 feed。成功后可在该 Actions run 的 **Summary → Desktop package
downloads** 直接打开 OSS 下载链接。手动入口只接受 `vX.Y.Z-alpha.N`；填写 beta 或正式
版本号会在 `prepare` 阶段报错并中断。

Beta 和正式版走 tag 触发的完整流程，自动构建全部平台、创建 GitHub Release，并分别更新
beta 或 release OSS channel。

## 发布结果与地址

一次成功的 beta 或正式版完整发布会产生两组结果：

1. GitHub Release：保存全部 CLI、服务端 bundle 和桌面安装包；
2. Desktop OSS：保存官网安装器、Server/CLI bundle、Electron 原生更新 feed 和 channel manifest。

地址规则如下：

| 内容 | 地址 |
|---|---|
| GitHub Releases | [github.com/OpenCSGs/csgclaw/releases](https://github.com/OpenCSGs/csgclaw/releases) |
| 单个 GitHub Release | `https://github.com/OpenCSGs/csgclaw/releases/tag/<tag>` |
| OSS 版本目录 | `https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/releases/<version>/` |
| beta manifest | [channels/beta/downloads.json](https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/channels/beta/downloads.json) |
| release manifest | [channels/release/downloads.json](https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/channels/release/downloads.json) |
| Electron 更新 feed | `channels/<channel>/updates/<platform>/<arch>/` |

其中 `<tag>` 带 `v`，例如 `v0.4.6-beta.3`；OSS 的 `<version>` 不带 `v`，例如
`0.4.6-beta.3`。

以 `v0.4.6-beta.3` 为例：

- [GitHub Release](https://github.com/OpenCSGs/csgclaw/releases/tag/v0.4.6-beta.3)
- [Mac ARM DMG](https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/releases/0.4.6-beta.3/csgclaw-desktop_v0.4.6-beta.3_darwin_arm64.dmg)
- [Mac Intel DMG](https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/releases/0.4.6-beta.3/csgclaw-desktop_v0.4.6-beta.3_darwin_amd64.dmg)
- [Windows x64 EXE](https://opencsg-public-resource.oss-cn-beijing.aliyuncs.com/csgclaw-desktop/releases/0.4.6-beta.3/csgclaw-desktop_v0.4.6-beta.3_windows_amd64.exe)

官网和 Server 升级都应读取对应 channel 的 `downloads.json`，而不是自行拼接文件名。manifest 会记录
`latest`、历史版本和官网安装器；每个版本新增的 `packages` 字段记录 `server` / `cli`、平台、架构、下载 URL、大小和 SHA-256。Electron 使用同一 channel 下的原生更新 feed，不直接解析 `downloads.json`。

## 版本与 channel 规则

项目约定使用以下三种发布方式：

| 版本示例 | 触发方式 | 发布结果 |
|---|---|---|
| `v0.4.6-alpha.1` | 手动 Run workflow | 单个桌面目标的 OSS 不可变包，不更新 manifest |
| `v0.4.6-beta.3` | 创建 Pre-release tag | 完整发布并更新 `channels/beta/downloads.json` |
| `v0.4.6` | 创建正式 Release tag | 完整发布并更新 `channels/release/downloads.json` |

beta 版本使用 `v<major>.<minor>.<patch>-beta.<number>`，例如
`v0.4.6-beta.1`。正式版本去掉 `-beta.<number>`，例如 `v0.4.6`。

版本必须使用合法格式：

- 正确：`v0.4.6-beta.1`
- 错误：`v0.4.6.beta.1`
- 错误：`v0.4.6-beta.01`

tag 一经发布即视为不可变。如果 beta 需要继续修复，应递增序号，例如从
`v0.4.6-beta.3` 发布 `v0.4.6-beta.4`，不要移动或覆盖旧 tag。

Alpha 版本使用 `v<major>.<minor>.<patch>-alpha.<number>`，例如
`v0.4.6-alpha.1`。Alpha 不单独创建 channel；手动发布也不会更新 beta channel 的
`latest`。全新安装的 alpha/beta 桌面包默认选择预览版更新通道，稳定包默认选择正式版
通道；已有用户保存的通道选择保持不变。

## 手动发布 alpha 单个桌面目标

需要先验证单个架构，或只补传一个不可变安装包时，可以在 GitHub Actions 的 Release
workflow 中点击 **Run workflow**，只填写：

- `version`：必须是 `vX.Y.Z-alpha.N`，例如 `v0.2.1-alpha.1`；
- `desktop_target`：例如 `darwin-arm64`。

Beta 或正式版本号不能从这里发布；校验失败时 workflow 会在 `prepare` 阶段中断。

手动运行固定为 desktop-only：不会构建 Server/CLI 平台矩阵，不会创建或修改 GitHub
Release，只会把所选目标的包上传到 OSS `releases/<version>/`。它不会读取、生成或上传
`downloads.json`、`RELEASES`、`RELEASES.json`，因此不会把该版本发布成官网或自动更新
channel 的 `latest`。上传成功后，完整 HTTPS 下载地址会同时打印在上传 step 日志和该 run
的 **Summary → Desktop package downloads** 中。

## 通过 GitHub 页面发布 beta

### 1. 确认准备发布的代码

先确认相关 PR 已经合并到 upstream 仓库的 `main` 分支，并且准备发布的改动都已经存在于
[OpenCSGs/csgclaw main](https://github.com/OpenCSGs/csgclaw/tree/main)。

如果 main 在测试完成后又合入了其他改动，应先确认这些新改动也适合进入本次 beta。

### 2. 选择 beta 版本号

打开 [Releases](https://github.com/OpenCSGs/csgclaw/releases) 或
[Tags](https://github.com/OpenCSGs/csgclaw/tags) 页面，确认下一个尚未使用的 beta 序号。

例如已有 `v0.4.6-beta.3`，下一个版本应使用 `v0.4.6-beta.4`。不要复用已有 tag。

### 3. 创建 beta Release

打开 [Draft a new release](https://github.com/OpenCSGs/csgclaw/releases/new)，按以下方式填写：

1. 点击 **Choose a tag**；
2. 输入新的 beta tag，例如 `v0.4.6-beta.4`；
3. 点击 **Create new tag**；
4. Target 选择已经确认的 `main` 分支；
5. Release title 使用同一个版本号；
6. 根据需要填写 release notes；
7. 勾选 **This is a pre-release**；
8. 点击 **Publish release**。

发布页面后，GitHub 会创建 tag 并触发 Release workflow。Release 资产会在 CI 成功后自动
补充到该页面，beta manifest 也会在最后更新。

## 通过 GitHub 页面发布正式 release

正式版本通常在最后一个 beta 验证完成后发布。例如 beta 为 `v0.4.6-beta.3`，正式版本为
`v0.4.6`。

正式版可以对应最后一个 beta 的相同代码，也可以包含 beta 之后补充并重新验证过的修复。
发布前应再次检查 upstream `main` 是否正好包含准备上线的内容。

打开 [Draft a new release](https://github.com/OpenCSGs/csgclaw/releases/new)，按以下方式填写：

1. 点击 **Choose a tag**；
2. 输入不带 beta 后缀的新 tag，例如 `v0.4.6`；
3. 点击 **Create new tag**；
4. Target 选择已经确认的 `main` 分支；
5. Release title 使用同一个版本号；
6. 填写正式版 release notes；
7. 不要勾选 **This is a pre-release**；
8. 如页面提供 **Set as the latest release**，保持勾选；
9. 点击 **Publish release**。

正式 release 不是把 beta 文件直接改名或复制到稳定 channel。新 tag 会触发完整构建，
生成新的普通 GitHub Release，把安装器写入新的不可变版本目录，最后更新
`channels/release/downloads.json`。

原有 beta Release 和 beta manifest 不会被删除。

## 线上构建与打包规则

创建 tag 后，GitHub Release workflow 会依次完成：

1. 校验版本并确定 beta 或 release channel；
2. 构建 Web UI；
3. 构建各平台的 `csgclaw` 和 `csgclaw-cli` bundle；
4. 在各平台 runner 上构建桌面安装包；
5. 把全部产物附加到 GitHub Release；
6. 把官网安装器、Server/CLI bundle 和 Electron 更新文件上传 OSS；
7. 先更新 Electron 的 `RELEASES` / `RELEASES.json`，最后更新对应 channel 的 `downloads.json`。

正式平台 matrix 维护在
[`scripts/release-platforms.txt`](../../../scripts/release-platforms.txt)，当前包括：

- Linux amd64、arm64；
- macOS arm64、amd64；
- Windows amd64。

### GitHub Release 产物

每个平台都会生成 `csgclaw` bundle 和独立的 `csgclaw-cli`：

```text
csgclaw_v<version>_<os>_<arch>.tar.gz
csgclaw-cli_v<version>_<os>_<arch>.tar.gz
```

Windows 使用 `.zip`。桌面产物规则如下：

| 平台 | GitHub Release 产物 |
|---|---|
| macOS arm64 | DMG、ZIP |
| macOS amd64 | DMG、ZIP |
| Linux amd64 | DEB |
| Linux arm64 | DEB |
| Windows amd64 | Setup EXE |

桌面文件名统一为：

```text
csgclaw-desktop_v<version>_<os>_<arch>.<ext>
```

例如：

```text
csgclaw-desktop_v0.4.6-beta.3_darwin_arm64.dmg
csgclaw-desktop_v0.4.6-beta.3_darwin_amd64.zip
csgclaw-desktop_v0.4.6-beta.3_linux_amd64.deb
csgclaw-desktop_v0.4.6-beta.3_windows_amd64.exe
```

### OSS 产物

OSS 接收官网使用的三个安装器：

- macOS arm64 DMG；
- macOS amd64 DMG；
- Windows amd64 Setup EXE。

Linux DEB 仍只保留在 GitHub Release。Server 和 CLI bundle 会按
`scripts/release-platforms.txt` 的完整矩阵上传到同一个不可变版本目录，并写入
`downloads.json` 的 `packages` 字段，供 Web/server 升级直接使用 OSS。

Electron 原生更新文件使用独立的 channel 路径：

```text
channels/<channel>/updates/darwin/arm64/RELEASES.json
channels/<channel>/updates/darwin/x64/RELEASES.json
channels/<channel>/updates/win32/x64/RELEASES
```

对应 ZIP、NUPKG 等不可变文件与 manifest 放在同一目录。桌面端持久化正式版或预览版选择，启动后和运行期间自动检查；发现更新时由 Electron 在后台下载，下载完成后 UI 才提示安装并重启。

OSS 会先写入不可变的 `releases/<version>/` 目录和 Electron 更新包，再更新原生 feed manifest，最后更新
`channels/<channel>/downloads.json`。这样官网和 Server 不会读取到只有部分平台的版本。

同一版本可以因 CI 重试而重新发布，但不能把 channel 的 `latest` 回滚到更旧版本。

## 通过 GitHub 页面查看和重试

发布后打开仓库的 [Actions](https://github.com/OpenCSGs/csgclaw/actions) 页面：

1. 在左侧选择 **Release** workflow；
2. 打开与本次 tag 对应的 run；
3. 等待所有 jobs 完成；
4. 如果是网络、runner 或打包工具的偶发失败，点击右上角 **Re-run jobs**；
5. 选择 **Re-run failed jobs**。

重跑失败 jobs 不会创建新 tag。失败的构建成功后，之前被跳过的 GitHub Release 汇总和
OSS 发布 jobs 会继续执行。

如果失败原因必须通过修改源码、workflow 或依赖解决，则不要只重跑旧版本。应先合并修复，
再创建更高的 beta 序号或新的正式版本。

## Beta/正式版发布验收

Release workflow 全部成功后：

1. 回到 [Releases](https://github.com/OpenCSGs/csgclaw/releases) 页面；
2. 确认 beta 显示 **Pre-release**，正式版显示为普通 Release；
3. 确认 macOS、Linux、Windows、CLI 和服务端 bundle 资产齐全；
4. 在浏览器中打开对应的 beta 或 release manifest；
5. 确认 `latest` 是本次版本；
6. 确认 manifest 中三个安装器以及完整 `packages` 矩阵的下载 URL 和 SHA-256 已生成；
7. 确认当前 channel 的 macOS/Windows 原生更新 manifest 可以访问；
8. 实际打开或下载至少一个安装器和一个 Server bundle，确认公开地址可用。

## GitHub Environment

线上 OSS 上传使用仓库 Settings → Environments 中的 `oss-publish` Environment，并读取：

- `OSS_ACCESS_KEY_ID`
- `OSS_ACCESS_KEY_SECRET`

Alpha 手动上传、beta 和 release 共用同一个 Environment。Beta 与 release 通过不同的
manifest 地址分流；相同 channel 的完整 OSS 发布会依次排队，两个 channel 互不阻塞。
