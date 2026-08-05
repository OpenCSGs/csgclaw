# CSGClaw Electron Desktop 架构与开发指南

## 1. 项目简介

CSGClaw Desktop 是 CSGClaw 的 Electron 桌面版本。它复用现有 Go 后端和 React Web UI，Electron 只负责桌面能力：

- Go：Agent、Team、Task、Runtime、Sandbox、模型、配置和数据。
- React：页面、路由和业务交互。
- Electron Main：窗口、托盘、单实例、Go sidecar 生命周期、系统浏览器和应用更新。
- Preload：向页面暴露少量、受控的桌面接口。

浏览器版和桌面版共享同一套业务实现：

```text
浏览器版
  Browser
    └── CSGClaw Go Server
          ├── Web UI
          ├── HTTP API / SSE
          └── Agent Runtime

桌面版
  Electron Main
    ├── BrowserWindow / Preload
    ├── Tray / Updater
    └── CSGClaw Go Sidecar
          ├── Web UI
          ├── HTTP API / SSE
          └── Agent Runtime
```

Electron 不复制 Go API，也不维护单独的桌面页面。

## 2. 运行架构

### 2.1 Sidecar

Electron 启动应用内置的 Go 可执行文件：

```bash
csgclaw _desktop-serve
```

`_desktop-serve` 复用普通 `serve` 的业务服务，但使用桌面专用启动方式：

- Renderer 固定监听 `127.0.0.1:18080`，保证 Electron 持久化分区始终使用同一个 Web Storage origin，并允许本机外部浏览器使用标准 CSGClaw 地址访问，同时不向局域网暴露。
- Sandbox API 使用独立的动态端口监听宿主机 IPv4 接口，并强制校验 Host、server access token 和空 Origin。
- Electron 通过 stdin 发送启动信息，Go 通过 stdout 返回就绪信息。
- Electron 退出或监督重启 sidecar 时通知 Go 先停止运行中的 Agent，再优雅关闭 HTTP 服务。
- Go 自升级在桌面版中关闭，由 Electron 统一更新完整应用。

### 2.2 启动流程

```text
1. Electron 获取单实例锁
2. Electron 生成 instance ID 和随机 session token
3. Electron 启动 csgclaw _desktop-serve
4. Go 获取 ~/.csgclaw/runtime.lock
5. Go 监听固定的 Renderer 回环端口和动态的 Sandbox API 端口
6. Go 在 ready 消息中只返回 Renderer 回环地址
7. Electron 校验 ready 消息并加载 Web UI
```

如果 sidecar 启动失败，Electron 会重试并提供打开日志或退出的入口。

关闭主窗口只会隐藏应用，托盘、sidecar 和 Agent 继续运行。选择 Quit 后，Electron 才会停止 sidecar 和正在运行的 Agent。

### 2.3 Renderer 与 Sandbox 访问

Renderer 和 Sandbox 复用同一个业务 Router，但通过独立 listener、地址和凭据访问：

```text
Electron Renderer / External Browser
    │ http://127.0.0.1:18080
    │ Electron: desktop session token
    │ External Browser: same-origin loopback request
    ▼
CSGClaw Go Sidecar
    ▲
    │ http://host.docker.internal:<sandbox-port>
    │ 或 http://<host-lan-ip>:<sandbox-port>
    │ server access token
OpenClaw / PicoClaw Sandbox
```

- Desktop session token 由 Electron 每次启动随机生成，只用于当前桌面会话。
- Server access token 来自 CSGClaw 配置，供 CLI 和 Sandbox Runtime 访问服务。
- Docker Desktop 使用 `host.docker.internal` 回连宿主机；容器内的 `127.0.0.1` 只表示容器自己。
- Linux Docker 和非 Docker provider 使用宿主机 LAN IPv4 回连独立的 Sandbox listener。
- Agent 配置创建和后续同步使用同一套 Sandbox 地址解析规则。

### 2.4 单实例

桌面版有两层保护：

- Electron 的单实例锁避免同时打开多个桌面窗口。
- Go 的 `~/.csgclaw/runtime.lock` 避免 Desktop、`serve` 和 daemon 同时操作本地数据。

## 3. 代码位置

```text
cli/serve/desktop.go
  _desktop-serve、动态端口、启动握手和 Sandbox 回连地址

internal/desktop/contract.go
  bootstrap / ready / shutdown 消息协议

internal/server/desktop_security.go
  Desktop HTTP 的 Host、Origin 和 token 校验

desktop/src/main/
  Electron 生命周期、窗口、托盘、sidecar 和更新

desktop/src/preload/
  受限的 contextBridge 接口

web/app/src/shared/platform/
  Browser / Electron 平台差异

desktop/forge.config.ts
  打包、签名、公证、更新和 Electron fuses
```

## 4. 安全边界

### 4.1 Renderer

Renderer 按普通 Web 页面处理：

- `contextIsolation: true`
- `sandbox: true`
- `nodeIntegration: false`
- 禁止 WebView 和任意新窗口
- 只允许在当前 sidecar origin 内导航
- 默认关闭 DevTools

开发时可启用 DevTools：

```bash
CSGCLAW_DESKTOP_DEVTOOLS=1 make desktop-dev
```

### 4.2 Preload 与 IPC

Preload 只暴露获取桌面信息、打开受信任 OAuth 地址和管理桌面更新等接口。Main 会校验 IPC 的发送窗口、frame、origin 和输入。

不要向 Renderer 暴露 `ipcRenderer`、文件系统、进程、任意命令执行或任意 URL 打开能力。

### 4.3 HTTP

| 调用方   | Host                         | 凭据                          | 额外限制                                                      |
| -------- | ---------------------------- | ----------------------------- | ------------------------------------------------------------- |
| Renderer | `127.0.0.1:18080`            | Electron 使用 session token；同源本机浏览器可直接访问 | 独立回环 listener；固定 origin 保存主题、语言和布局等本地状态 |
| Sandbox  | 启动时计算的 Host 和独立端口 | server token                  | 独立 IPv4 listener；Origin 必须为空                           |

两个 listener 分别拒绝另一入口的 Host；Renderer 允许同源本机浏览器访问，desktop session token 不能用于 Sandbox listener。
Sandbox listener 始终要求 server token，并拒绝浏览器 Origin。

## 5. Web UI 适配

React 页面不直接判断 Electron，也不直接调用 Preload。平台差异集中在 `web/app/src/shared/platform/`：

- `runtime.ts`：识别 Browser 或 Electron。
- `desktopBridge.ts`：读取受控的 Preload bridge。
- `externalNavigation.ts`：统一页面跳转和系统浏览器行为。
- `updatePort.ts`：统一 Go 升级与 Electron 更新状态。

浏览器模式继续使用原有 OAuth 页面跳转；桌面模式通过系统浏览器完成 OAuth。保存服务配置后，桌面模式通过受控 IPC 让 Main 进程停止旧 sidecar 和 Agent，再启动新的 sidecar；浏览器模式继续调用普通服务重启 API。桌面应用更新由 Electron `autoUpdater` 负责，避免只升级 Go 后端造成版本不一致。

## 6. 本地开发

### 6.1 环境

- Go
- Node.js `>=22.13.0` 且 `<25`
- pnpm `>=9` 且 `<12`，或启用了 Corepack 的 Node.js

推荐使用仓库 `.nvmrc` 中的 Node.js 版本。日常开发直接执行：

```bash
make desktop-dev
```

该命令会在需要时安装 Electron 依赖，然后构建 Web UI、当前平台 Go sidecar、Sandbox Linux CLI，并启动 Electron Forge。

常用调试参数：

```bash
# 指定 Go sidecar
CSGCLAW_DESKTOP_BACKEND=/absolute/path/to/csgclaw make desktop-dev

# 指定配置文件
CSGCLAW_DESKTOP_CONFIG=/absolute/path/to/config.toml make desktop-dev

# 打开 DevTools
CSGCLAW_DESKTOP_DEVTOOLS=1 make desktop-dev
```

Electron 和 sidecar 日志位于系统应用日志目录，Go 输出记录在 `backend.log`。启动失败时先检查：

1. 是否已有 `csgclaw serve` 或 daemon 占用 `runtime.lock`。
2. `backend.log` 中是否有启动、端口或协议错误。
3. Agent 配置中的服务地址是否为 `host.docker.internal:<sandbox-port>` 或宿主机 LAN IPv4 对应的动态端口。

## 7. 打包

macOS/Linux 桌面打包使用：

```bash
make desktop-package
```

Windows 无需安装 `make`，使用：

```powershell
.\scripts\build.cmd desktop-package
```

它会构建 backend bundle，并通过 Electron Forge 生成当前平台的应用和分发产物，不需要先运行其他打包命令。

Desktop backend 默认不打包 BoxLite CLI，因此打包过程不会下载 BoxLite。

常用目标：

```text
# macOS Apple Silicon
make desktop-package TARGET_OS=darwin TARGET_ARCH=arm64

# macOS Intel
make desktop-package TARGET_OS=darwin TARGET_ARCH=amd64

# Windows x64（在 Windows 终端中）
.\scripts\build.cmd desktop-package
```

产物：

- macOS：`.app`、DMG、ZIP。
- Windows：Squirrel Setup 和更新文件。
- Linux：DEB。

打包配置按目标平台分别使用 `csgclaw.icns`、`csgclaw.ico` 和 `csgclaw.png`，Windows 安装器和 Linux DEB 也显式复用对应图标。

所有本地桌面构建产物统一位于 `desktop/out/`：backend 中间产物在 `desktop/out/input/<os>-<arch>/backend/`，Forge 原始安装包在 `desktop/out/make/`，OSS 发布工作区在 `desktop/out/oss/`。GitHub Release CI 会从 `desktop/out/make/` 归档最终文件到 runner 的临时 `dist/`，生成如 `csgclaw-desktop_v0.4.3_darwin_arm64.dmg` 的稳定名称并附加到对应 GitHub Release。GitLab 的桌面 job 仅供可选手动构建，成功产物作为 GitLab artifact 保留一天，不上传到 `https://csgclaw.opencsg.com/releases/<tag>/`。

正式安装包建议在目标系统构建。跨平台编译成功不代表签名、安装、启动和更新已经通过目标系统验证。

macOS 正式发布需要配置签名和公证账号；Windows 可通过 `CSGCLAW_WINDOWS_SIGN_*` 和证书变量接入签名。CI 仅在完整签名变量组存在时传递这些配置；否则 macOS 保持 ad-hoc 签名、Windows 保持未签名。GitLab 的 macOS/Windows 桌面 job 必须运行在对应原生 runner 上，且为可选手动 job。更新源可通过 `CSGCLAW_DESKTOP_UPDATE_BASE_URL` 设置且必须使用 HTTPS，但当前发布 CI 不设置它。

面向中国大陆公司从官网发布 Windows 和 macOS 应用的账号申请、签名、公证及 CI 流程，参见 [Electron 桌面应用签名与发布指南](electron-desktop-signing.zh.md)。

## 8. 开发约定

- Agent、模型、Task、Team、配置和数据逻辑放在 Go。
- 通用页面和业务交互放在 React。
- 窗口、托盘、系统浏览器和安装更新放在 Electron Main。
- Renderer 需要系统能力时，通过最小的 Preload + IPC 接口提供。
- Browser 与 Electron 的差异放在 `shared/platform`。
- 修改 `_desktop-serve` 时，同时检查普通 `serve`、运行锁、关闭流程和 backend bundle。

新增桌面能力时，先定义最小接口，再实现 Main 和 Preload，最后通过平台适配层给 React 使用。不要为桌面版复制 Go API 或 React 页面。

## 9. 验证

修改桌面代码后至少执行：

```bash
./scripts/desktop-pnpm.sh run build
go test ./cli/serve ./internal/agent ./internal/app ./internal/desktop ./internal/server ./internal/api
make build-web
```

需要实际运行或打包时再执行：

```bash
make desktop-dev
make desktop-package TARGET_OS=<os> TARGET_ARCH=<arch>
```
