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

`make build-all` 保留为 `make build` 的别名。应用内置 Codex，DeepSeek Harness 按需安装。

常用 target：

| Target | 说明 |
|---|---|
| `make build-server-bin` | 构建 `bin/csgclaw`、宿主平台的 `bin/csgclaw-cli` 和 bundled `bin/codex` |
| `make run` | 构建并运行 `csgclaw serve` |
| `make fmt` | 格式化 Go 源码 |
| `make test` | 运行 `go test ./...` |


## Windows 无 make 时

如果 Windows 环境没有 `make`，可以直接使用 PowerShell 构建脚本：

```powershell
.\scripts\build.cmd build
.\scripts\build.cmd build-server-bin
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


## 独立后端与可选 UI

```bash
make build-headless
./bin/csgclaw-headless serve
# 普通含 UI 的构建也可以在运行时关闭 UI：
./bin/csgclaw serve --api-only
```

`build-headless` 只生成 `bin/csgclaw-headless`（Windows 带 `.exe`），底层使用
`go build -tags headless`。它不执行 Node/pnpm、不编译网页、不下载 Codex，生成的
可执行程序不依赖 `csgclaw/web`。Codex、DSH、伴生 CLI 和沙盒工具等执行依赖由
宿主或镜像按需配置。本地 make 构建继续带 `+local` 版本后缀；正式版本仍要求
已有的官方 bundle 布局。

启动入口将可选的 `http.Handler` 传入 `server.Options.UI`。`internal/server`
维护 API 路由，只消费这个接口，不导入具体 UI。传 nil 时是纯后端：健康检查、
API 和事件流保留；UI 页面及未知 API 返回 404。API-only 启动关闭自动登录探测、
浏览器打开和宿主桌面应用打开，后台启动也保留此模式。headless 产物始终使用此模式，
传 `--api-only=false` 也不会恢复 UI；内部 Electron 入口会明确拒绝启动。
`--no-browser` 只控制不打开浏览器，不会关闭 UI 路由。

普通本地版和 Electron 继续使用内嵌 UI。多套外部客户端可以通过已有 API、事件协议和
访问策略连接同一后端；独立托管的浏览器页面经平台网关或同域反向代理接入。本次未新增
CORS、另一套 UI、Electron 远程后端模式或通用插件生命周期框架。配置与业务数据继续
使用原有格式和位置，无需迁移。

复用现有容器布局构建纯后端：

```bash
docker build --build-arg CSGCLAW_BUILD=headless -t csgclaw:headless .
docker run --rm csgclaw:headless serve
```

默认值是 `CSGCLAW_BUILD=embedded`。BuildKit 在 headless 模式跳过 Web 构建阶段，
最终镜像仍包含伴生 CLI 和 Codex。后续云端沙盒镜像流水线接入时需显式传入该参数；
源码变更不会自动更新已经部署的实例。

## 选择并打包 UI

```bash
make build-ui UI=web
make build-ui UI=custom UI_DIR=/absolute/path/to/another-ui/dist
make build-ui UI=both UI_DIR=/absolute/path/to/another-ui/dist
make build-ui UI=none
```

每个命令生成一个服务端二进制 `bin/csgclaw-<UI>[.exe]`，嵌入选中的网页资源。
`UI_BIN` 可指定输出位置。执行器等运行依赖由宿主提供，与 `build-headless` 相同。
默认 `make`、桌面安装包和 release 打包继续使用现有 UI。

| UI 选择 | 编译进入的 UI 包 | 页面入口 | Docker 的 `CSGCLAW_BUILD` |
| --- | --- | --- | --- |
| `web` | `web` | `/` | `embedded` |
| `custom` | `web/custom` | `/` | `custom` |
| `both` | `web`＋`web/custom` | `/`＋`/ui/custom/` | `both` |
| `none` | 不包含 UI 包 | 没有 UI 路由 | `headless` |

`UI_DIR` 默认使用仓库内的 `examples/manager-ui`，这是一套不需要额外依赖的精简前端，
支持 OpenCSG 登录、会话模型选择、新建 Manager 会话和消息同步。直接执行
`make build-ui UI=both` 即可同时嵌入两套 UI。模型选择仅修改当前会话配置；登录状态、
会话和消息与原 UI 共用。使用说明见 [Manager Chat](../../examples/manager-ui/README.md)。

`UI_DIR` 也可指定其他已经构建好、包含 `index.html` 的静态前端目录。打包脚本不替另一套 UI
执行构建，只复制产物到忽略提交的 `web/custom/static-dist/`，替换旧资源，跳过隐藏
文件和 `node_modules`，拒绝符号链接及特殊文件。同一 worktree 的不同自定义 UI 构建
应串行执行，或分别使用隔离 worktree。二进制运行时不再依赖 `UI_DIR`；自定义产物按
与现有前端相同的可信应用代码对待。

默认 `make`、`make package`、release 和 Electron 打包继续使用现有 CSGClaw UI 与根入口。
`UI=both` 只增加 `/ui/custom/` 分流，其余路径原样交给现有 UI，浏览器默认打开地址、
静态资源和 SSE SharedWorker 的 URL 保持原样。无尾斜杠入口采用相对重定向，保留网关
路径前缀及查询参数。

构建标签仍选择一个 `http.Handler` 传给 `server.Options.UI`；双 UI handler 只分发网页
请求，不再启动一个后端。`serve --api-only` 可以同时关闭两套页面。API、健康检查和
事件流继续由同一后端提供，不增加插件注册表或生命周期框架。

自定义前端需配置相对资源地址、路由和 API base。custom 单独打包时，相对于应用根路径
使用 `./api/v1/`；both 中相对于 `/ui/custom/` 使用 `../../api/v1/`。相对地址必须基于
前端根路径解析，不能基于任意 history 页面；hash 路由更直接，history 路由需显式配置
资源和路由 base。鉴权和事件协议保持原样。

生成带选定 UI 的完整压缩包：

```bash
make package-ui UI=both UI_DIR=/absolute/path/to/another-ui/dist
```

它复用原打包脚本、可执行文件名及 bundle 布局，包含伴生 CLI 和 Codex。
变体默认输出到 `dist/ui-<UI>/`（可用 `UI_DIST_DIR` 覆盖），避免覆盖普通发布产物。
解压后仍使用原来的 `csgclaw` 命令，both 默认进入现有 UI `/`。原 package/release/
桌面命令的默认行为不变；变体安装器和更新通道需后续单独接入。

用最小测试页面验证双 UI 打包和 API 接入：

```bash
make build-ui UI=both UI_DIR=web/custom/testdata
./bin/csgclaw-both serve --no-browser
# 现有 UI 在 /；测试页面在 /ui/custom/，读取相同后端的版本接口。
```

测试页面不作为完整产品 UI。构建 custom 或 both 容器前，先执行
`make prepare-custom-ui UI_DIR=...`，然后执行
`docker build --build-arg CSGCLAW_BUILD=custom -t csgclaw:custom .`。
选择 `CSGCLAW_BUILD=both` 可包含两套 UI，省略参数时仍为 `embedded`。Docker 检查预备
产物的 `index.html`；custom/headless 模式跳过现有前端编译阶段。
云端流水线、签名桌面安装包和正式 release 的变体接入需要另外显式选择构建方式。

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
```

CSGClaw 平台到 Codex artifact 的统一映射维护在
[`scripts/codex-cli-platforms.txt`](../../scripts/codex-cli-platforms.txt)；Shell、
PowerShell 和 Docker 打包均读取该文件。正式 release 平台仍维护在
[`scripts/release-platforms.txt`](../../scripts/release-platforms.txt)。


| Target | 说明 |
|---|---|
| `make package` | 打包当前平台 |
| `make package-all` | 构建并打包当前平台产物 |
| `make desktop-package` | 在 macOS/Linux 构建当前平台的桌面安装包 |
| `make desktop-package-macos-signed VERSION=<version>` | 构建、签名、公证、验证并在本地归档 macOS 安装包；默认双架构。本机只打当前芯片时加 `DESKTOP_MACOS_TARGETS=arm64` 或 `amd64`；覆盖已有归档时加 `DESKTOP_MACOS_FORCE=1`。完整命令见 [Windows 与 macOS 发布指南](desktop/electron-desktop-signing.zh.md) |
| `scripts\build.cmd desktop-package` | 在 Windows 无需 `make` 构建桌面安装包 |
| `make desktop-package-oss VERSION=<version>` | 复用 `desktop-package` 构建并归档本机支持的 OSS 桌面产物 |
| `make desktop-oss-manifest VERSION=<version>` | 收齐三平台安装器后生成 `downloads.json` |
| `make desktop-oss-publish VERSION=<version>` | 严格检查三平台安装器并发布到 OSS |
| `make desktop-oss-upload-packages VERSION=<version> DESKTOP_OSS_TARGETS=<os-arch>` | 只上传指定桌面目标的不可变包，不修改 OSS channel JSON |
| `make release` | 构建配置的跨平台 release bundle |

发布 CI 使用 `.github/workflows/release.yml` 和 `.gitlab/ci.yml`。GitHub tag 推送保持完整发布路径：将 Server/CLI bundle 和原生桌面安装包附加到对应的 GitHub Release，并把完整 Server/CLI 平台矩阵、两个 macOS DMG、Windows x64 安装器和 Electron 原生更新 feed 发布到 beta 或 release OSS channel。手动运行 GitHub workflow 只接受 `vX.Y.Z-alpha.N`，并只填写一个 `desktop_target`；任务只构建该桌面目标并上传不可变包，不修改 `downloads.json`、`RELEASES` 或 `RELEASES.json`，下载地址会显示在 Actions Summary。GitLab 保留原有上传到 `https://csgclaw.opencsg.com/releases/<tag>/` 的 SSH 发布链路，并继续发布 CSGClaw 产品镜像。其桌面 job 是可选手动构建：成功的安装包作为 GitLab artifact 保留一天，不会上传到公开发布目录。GitLab 的 macOS 和 Windows 桌面 job 需要提供并打上 `csgclaw-macos-arm64`、`csgclaw-macos-amd64`、`csgclaw-windows-amd64` tag 的原生 runner。

桌面安装包采用如 `csgclaw-desktop_v0.4.3_darwin_arm64.dmg` 的命名。普通 `desktop-package` 在凭据缺失时仍保持 macOS ad-hoc、Windows 未签名的默认行为；严格本地 macOS target 在 Developer ID 或公证凭据不完整时会直接失败，不会产出半签名包。它复用 CI 已使用的 Forge 打包 target 和产物收集器，但不修改现有 GitHub workflow；只写入 `desktop/out/local/`，绝不上传 OSS。GitHub Release CI 会把桌面更新源配置到相互隔离的 `channels/<channel>/updates/`；桌面端后台检查并下载，只有下载完成后才提示用户安装。

OSS 桌面发布命令的完整说明见 [Electron Desktop OSS 发布](desktop/electron-desktop-oss-release.zh.md)。`desktop-package` 仍是单平台原子构建，OSS 上传只放在聚合命令中，避免并行 runner 使用不完整产物发布。

本地桌面构建文件统一维护在 `desktop/out/`：`input/` 是后端 bundle 中间件，`make/` 是 Forge 原始产物，`oss/` 是可上传版本、外部导入包和 channel manifest。仓库根 `dist/` 继续作为 Go release 与 CI runner 的临时平铺汇总目录。

GitHub `Release` 每次执行结束后，失败的 run 由 `.github/workflows/release-retry.yml` 自动处理：只重跑失败 job 及其依赖的后续 job，复用已经成功的平台构建和产物。每个发布 run 最多执行三次（首次执行加两次自动重试）；取消后不会自动重试，达到上限仍失败时保留失败状态供人工排查。手动重跑也计入同一上限。重试 workflow 合入默认分支后生效。

CI 和本地 macOS 签名打包共用 `scripts/macos-stapler.mjs` 处理 App/DMG 的贴票与票据验证。每条命令超时为三分钟；遇到临时网络错误时最多执行三次，两次等待分别为 30 秒、90 秒。重试复用现有产物，签名或票据错误立即失败，网络重试耗尽仍使 job 失败。命令级恢复失败后，继续由现有 workflow 级重试兜底。

## 相关文档

- [配置](config.zh.md)
- [架构](architecture.md)
- [Web 开发](web/development.zh.md)
- [Windows 与 macOS 发布指南](desktop/electron-desktop-signing.zh.md)
