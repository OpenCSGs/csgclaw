<p align="center">
  <img src="assets/logo.png" alt="CSGClaw logo" width="600" />
</p>

<p align="center">
  <a href="./README.md">English</a> | 中文
</p>

# CSGClaw

> Your Personal AI Team

CSGClaw 是 OpenCSG 推出的多智能体协作平台。它想解决的不是"怎么把一个 Agent 做得更万能"，而是一个更实际的问题：**当任务开始变复杂时，怎么让一组 AI 像一个团队一样协作，同时又足够轻、足够安全、足够容易启动。**

## 安装

**macOS / Linux：**

```bash
curl -fsSL https://csgclaw.opencsg.com/install.sh | bash
```

**Windows（PowerShell）：**

```powershell
curl.exe -fsSL https://csgclaw.opencsg.com/install.ps1 | powershell -ExecutionPolicy Bypass -Command -
```

安装脚本会下载预编译 release bundle，安装到用户本地目录，并把 `csgclaw` 放到你的 `PATH` 中。

- `install.sh` 当前支持 macOS arm64、Linux amd64 和 Linux arm64。
- `install.ps1` 当前支持 Windows amd64。

官方 release 也会发布 macOS amd64 和 Windows amd64 bundle。当前 Windows bundle 默认使用 Docker，需要本地可用的 Docker 环境。

**源码编译：**

```bash
make build
```

Makefile 详细说明（embed 模板 version、可选 Docker 镜像构建等）见 [docs/build.zh.md](docs/build.zh.md)。

对大多数用户来说，直接使用上面的安装脚本会更简单。

## 快速开始

```bash
csgclaw serve
```

执行后会尽量自动在浏览器中打开 IM 工作区；如果没有自动打开，就手动访问 CLI 打印出的地址即可，例如 `http://127.0.0.1:18080/`。

## 配置

`csgclaw serve` 会使用本地配置中的 server、bootstrap、sandbox 和 channel 设置，并在首次运行时自动补齐缺失的 bootstrap 状态。Agent 模型/provider profile 存储在 agent 状态中，并通过 Web UI 管理。Sandbox provider 选项、Worker 覆盖示例和 agent profile 详情见 [docs/config.zh.md](docs/config.zh.md)。

面向维护者的架构、API 和 IM thread 设计说明见 [docs/architecture.md](docs/architecture.md)、[docs/api.zh.md](docs/api.zh.md) 和 [docs/im-threads.zh.md](docs/im-threads.zh.md)。

## 功能特性

- **多智能体协作** — 通过单一协调入口与一组分工明确的 Worker 协作，而不是轮流操作多个聊天窗口
- **一键安装** — 提供 macOS arm64/amd64、Linux amd64/arm64 和 Windows amd64 预编译版本，几分钟内即可启动
- **开箱即用的 WebUI** — 执行 `csgclaw serve` 后直接在浏览器中使用
- **多通道支持** — 按需接入飞书、微信、Matrix 等通信工具
- **隔离执行** — 每个 Worker 默认运行在安全沙箱中，无需额外配置
- **角色化 Worker** — 可针对前端、后端、测试、文档、调研等职责分别配置 Worker

## CSGClaw 是什么

CSGClaw 提供一位 Manager 和一组可分工的 Worker，让你通过统一入口完成目标表达、任务拆解、角色分配、进度跟踪和结果汇总，而不是直接和多个独立 Agent 逐一沟通。

```text
┌────────────────────────────────────────────────────────────┐
│                         CSGClaw                            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ Manager — 理解目标、拆解任务、协调 Worker            │  │
│  └──────────────────────────────────────────────────────┘  │
│               ↓                      ↓                     │
│        Worker Alice            Worker Bob                  │
│         前端 / UI               后端 / 接口                │
│                                                            │
│      WebUI / 飞书 / 微信 / Matrix / 其他接入通道           │
└────────────────────────────────────────────────────────────┘
                      ↑ 你来做决策
```

**Manager** — 接收目标，拆解任务，选择 Worker，跟踪进度，汇总结果。

**Worker** — 面向具体职责的执行单元（前端、后端、测试、文档、调研……）。角色分工让上下文更干净，协作更容易组织。

**Sandbox** — Worker 执行环境由配置的 sandbox provider 隔离。未显式配置 provider 时，CSGClaw 默认使用 **Docker**；BoxLite 仍可通过显式配置启用。

**Interface** — 默认提供 WebUI；飞书、微信、Matrix 等通道可按需接入。

## 典型工作流

```text
你：做一个简单的产品原型，包含首页、登录页和后台雏形

Manager：收到，拆解任务
  · Alice → 首页和登录页
  · Bob   → 后台接口和数据结构
  · Carol → 联调和验收

你：登录页需要支持 GitHub 登录

Manager：收到，已同步给 Alice 和 Bob

Carol：第一轮联调发现登录返回字段缺少用户头像

Manager：已记录，Bob 先修接口，字段确认后 Alice 再更新展示
```

关键不在于"能不能创建多个 Agent"，而在于**协作关系有没有被组织起来**。

## 设计取舍

**默认选择 PicoClaw，同时保留运行时扩展能力。**
CSGClaw 默认使用 PicoClaw 作为轻量化 Agent Runtime，让 Manager 启动更快、占用更低。运行时仍可插拔，需要时可以集成 OpenClaw 等其他实现。

**默认选择 Docker，设计上不绑定单一 Sandbox。**
隔离不是可选项。Docker 作为默认 sandbox provider 具备更广泛的跨平台可用性；BoxLite 仍然适合偏好轻量本地运行时的环境，并允许按需要显式切换不同的 sandbox provider。

**默认 WebUI，不绑定单一通道。**
很多多智能体系统把某种消息协议当作唯一入口。CSGClaw 自带 WebUI，让你可以立即开始；飞书、微信、Matrix 等通道作为可选集成存在，而不是预设前提。

## 适合谁

- 想把 AI 从单助手升级为协作团队的独立开发者
- 希望降低多智能体使用门槛的小团队
- 更看重启动速度、资源占用和默认体验的用户

## 致谢

CSGClaw 的思路受到了 HiClaw 在多智能体协作体验方面探索的启发。在具体实现上，CSGClaw 更强调轻量化运行时、本地易用性，以及不绑定单一通信通道的产品路线。

## 许可证

CSGClaw 采用 Apache License 2.0 许可发布。具体内容见 [LICENSE](LICENSE)。
