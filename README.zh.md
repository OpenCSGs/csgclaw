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
curl -fsSL https://raw.githubusercontent.com/OpenCSGs/csgclaw/main/scripts/install.sh | bash
```

安装脚本会从 GitHub Releases 下载预编译二进制并放到你的 `PATH` 目录中。目前提供 macOS arm64 和 Linux amd64 的预编译版本。

**源码编译：**

```bash
export CGO_ENABLED=1
go mod download
(cd third_party/boxlite-go && BOXLITE_SDK_VERSION=v0.7.6 go run ./cmd/setup)
go build ./cmd/csgclaw
```

## 快速开始

```bash
csgclaw onboard
csgclaw serve
```

执行后 CLI 会打印访问地址（例如 `http://127.0.0.1:18080/`），在浏览器中打开即可进入 IM 工作区。
全新交互式配置会默认选择 `csghub-lite`，访问 `http://127.0.0.1:11435/v1`，也可以在提示中覆盖这个 URL，并从 OpenAI 兼容的 `/v1/models` 自动导入模型。请先启动 CSGHub-lite，例如：

```bash
csghub-lite run Qwen/Qwen3-0.6B-GGUF
```

脚本或非交互环境请显式传入模型参数：

```bash
csgclaw onboard --provider csghub-lite --models Qwen/Qwen3-0.6B-GGUF
csgclaw onboard --base-url <url> --api-key <key> --models <model[,model...]> [--reasoning-effort <effort>]
```

## 配置

`csgclaw onboard` 会写入本地配置，包含 server、model、bootstrap、sandbox 和 channel 设置。模型 provider 示例、sandbox provider 选项和 Worker 覆盖示例见 [docs/config.zh.md](docs/config.zh.md)。

## 功能特性

- **多智能体协作** — 通过单一协调入口与一组分工明确的 Worker 协作，而不是轮流操作多个聊天窗口
- **一键安装** — 预编译二进制，安装完即可使用
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

**Sandbox** — Worker 执行环境由配置的 sandbox provider 隔离。默认是 **BoxLite**，也支持 `boxlite-cli` 和自定义 provider。

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

**默认选择 BoxLite，同时把 Sandbox 作为扩展点。**
隔离是必要的，但 Docker 对本地优先场景往往过重。BoxLite 无需容器运行时，也能为 Worker 提供有意义的安全边界。需要不同隔离模型时，可以扩展自定义 sandbox provider。

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
