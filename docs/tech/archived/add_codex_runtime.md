# Add Codex Runtime

## 背景

当前 CSGClaw 里已经有两层相对清晰的边界：

- `internal/runtime/` 负责 agent runtime 生命周期。
- `/api/bots/{bot_id}/events` 与 `/api/bots/{bot_id}/messages/send` 负责把 IM 消息桥接给外部 agent。

现有 PicoClaw 走的是：

- runtime 层负责拉起 sandbox/gateway。
- bot compatibility API 负责事件输入与消息输出。

这次要补的 Codex 方案，本质上也应保持同样分层：

- `codex` runtime 只负责 `codex-acp` 进程生命周期与 ACP session 生命周期。
- 与 CSGClaw IM 的对接不要塞进 `Runtime` 接口，而是单独做一个 channel/bridge。
- bridge 监听 `/api/bots/{id}/events` 收到用户消息后，转成 `acp.PromptRequest` 发给 `codex-acp`。
- `codex-acp` 返回的文本回复、tool call 状态、权限请求结果，再通过 `/api/bots/{id}/messages/send` 回发到房间。

这样可以保持与 PicoClaw 一致：`Runtime` 管“进程和状态”，bridge 管“消息面”。

## 目标

实现一个基于 Codex 的本地 runtime，底层通过 ACP 与 `codex-acp` 通信，并满足两点：

1. 启动前自动准备 `codex-acp` 二进制。
2. 不直接让 CSGClaw 通过 `Runtime` 收发聊天消息，而是增加一个独立 channel/bridge，复用现有 bot compatibility API。

## 非目标

本阶段不建议一起做下面这些事情：

- 不重构现有 `Runtime` 顶层接口。
- 不改造 PicoClaw 的 bridge 协议。
- 不引入通用“多 runtime 对话总线”的大而全抽象。
- 不在第一版里解决多会话并发、复杂工具鉴权 UI、富文本渲染。

先把单 agent、单 ACP session、单 bot stream 跑通，再做抽象上提。

## 现状梳理

### 1. Runtime 注册机制已经具备

当前 `agent.Service` 已支持按 `kind` 注册 runtime：

- `internal/runtime/runtime.go`
- `internal/agent/service.go`
- `internal/agent/runtime_state.go`

已经有 `KindCodex = "codex"` 常量，但还没有真正的实现和接线。

### 2. 现有 worker 创建路径仍然强绑定 PicoClaw

当前 worker 创建时直接取：

- `internal/agent/service.go#CreateWorker`

它内部固定调用：

- `s.runtimeForKind(RuntimeKindPicoClawSandbox)`

另外 `runtimeKindForAgent()` 当前也是按 role 推导：

- `internal/agent/runtime_record.go`

这意味着如果不先把“agent 选择哪种 runtime”显式化，Codex runtime 即使注册了，也不会被实际使用。

### 3. `/api/bots/*` 已经是一个现成 bridge 面

当前已经有：

- `GET /api/bots/{bot_id}/events`
- `POST /api/bots/{bot_id}/messages/send`

实现位置：

- `internal/api/bot_compat.go`
- `internal/im/bot_bridge.go`

这个接口层已经具备：

- SSE 长连接收消息。
- pending/inflight/ack/requeue。
- reconnect 后消息重放。
- 通过 bot 身份把文本再发回房间。

所以 Codex 不需要重新发明对话入口，应该直接复用这组接口。

### 4. ACP client 示例已经足够做第一版参考

仓库里的：

- `example_codex_acp.go`

已经覆盖了第一版需要的核心路径：

- 启动 `codex-acp` 子进程。
- 通过 `acp.NewClientSideConnection(...)` 建立 client 侧连接。
- `Initialize`。
- `NewSession`。
- `Prompt`。
- 实现 `acp.Client` 回调处理：
  - `SessionUpdate`
  - `RequestPermission`
  - 文件读写
  - terminal 相关接口

因此第一版不需要猜测 ACP 用法，直接把示例中的进程管理与 ACP callback 模式收敛成生产代码即可。

## 推荐总体方案

## 方案总览

拆成四块：

1. `codex` runtime
2. `codex-acp` 安装/定位器
3. `codex` bridge/channel
4. agent runtime 选择与持久化补齐

关系如下：

```text
IM message
  -> BotBridge pending/inflight
  -> GET /api/bots/<id>/events
  -> codex bridge subscriber
  -> ACP PromptRequest
  -> codex-acp process
  -> ACP SessionUpdate / ToolCall / Permission
  -> codex bridge renderer
  -> POST /api/bots/<id>/messages/send
  -> IM room
```

同时 runtime 生命周期与消息桥解耦：

```text
agent.Service Start/Stop/Delete
  -> internal/runtime/codex.Runtime
  -> host process: codex-acp

bridge goroutine
  -> connect bot events SSE
  -> deliver PromptRequest
  -> relay reply/tool events back to bot messages
```

## 设计细化

### A. `internal/runtime/codex` 只做生命周期管理

建议新增：

- `internal/runtime/codex/runtime.go`
- `internal/runtime/codex/process.go`
- `internal/runtime/codex/session.go`

职责：

- `Create`
  - 为 agent 创建本地工作目录。
  - 确保 `codex-acp` 已存在。
  - 生成 runtime metadata。
  - 不在这里收发业务消息。
- `Start`
  - 拉起 `codex-acp` 进程。
  - 完成 ACP `Initialize` + `NewSession`。
  - 启动 session update 消费循环。
  - 持久化 pid / session_id / thread-like metadata。
- `Stop`
  - 优雅关闭 ACP session 与子进程。
- `Delete`
  - 清理 runtime 状态目录与残留进程。
- `State` / `Info`
  - 基于进程存活、session 是否可用、状态文件返回。
- `StreamLogs`
  - 读取 host 侧 `codex-acp` stdout/stderr 日志。

这个 runtime 不应该直接依赖 `im.Service` 或 `/api/bots/*`。

### B. 为 Codex 维护独立 runtime home

PicoClaw 当前复用了 sandbox home 概念；Codex 是 host-process runtime，不该强绑 `sandbox.Runtime`。

建议为 codex 单独定义 host 状态目录，例如：

```text
~/.csgclaw/agents/<agent-name>/.csgclaw/codex/
  runtime.json
  session.json
  codex.log
  stdout.log
  stderr.log
  workspace/
  home/
  codex-home/
```

其中：

- `workspace/` 给 agent 文件上下文。
- `home/` 和 `codex-home/` 参照 `example_codex_acp.go` 的 isolated home 思路。
- `runtime.json` 记录 pid、started_at、binary_path。
- `session.json` 记录 ACP session id、最近状态、可恢复信息。

### C. `codex-acp` 下载器做成显式依赖

你提出“需要先下载 `codex-acp`”，这部分建议不要埋在 runtime 内部深处，而是做成显式组件。

建议新增：

- `internal/codexacp/installer.go`
- `internal/codexacp/locator.go`

职责拆分：

- `Locator`
  - 查找用户显式配置路径。
  - 查找缓存路径。
  - 查找 PATH。
- `Installer`
  - 若未找到，则下载指定版本的 `codex-acp`。
  - 写入统一缓存目录。
  - 校验可执行权限。
  - 返回最终 binary path。

建议缓存目录：

```text
~/.csgclaw/sandbox-tools/codex-acp
```

或带版本：

```text
~/.csgclaw/sandbox-tools/codex-acp/<version>/codex-acp
```

建议把“下载地址/版本”收敛为配置项，但第一版可先支持：

- config 固定版本
- 环境变量覆盖版本/路径

关键原则：

- 下载逻辑只负责“准备二进制”。
- runtime 只消费 `binaryPath`，不关心下载细节。

### D. bridge 独立于 runtime，实现为额外 channel

这是这次设计里最关键的一点。

不要把“监听 `/api/bots/{id}/events` 并回发 `/messages/send`”塞进 `Runtime` 接口。建议新增一个运行时外部的 bridge，例如：

- `internal/channel/codexbridge/bridge.go`
- `internal/channel/codexbridge/client.go`
- `internal/channel/codexbridge/render.go`

职责：

- 用 bot access token 连接：
  - `GET /api/bots/{id}/events`
- 把收到的 SSE `message` 事件解析成内部 prompt 输入。
- 找到对应的 codex runtime session。
- 发 `acp.PromptRequest`。
- 消费 ACP `SessionUpdate`。
- 将文本回复、tool call、状态更新渲染成适合 IM 的文本消息。
- 用：
  - `POST /api/bots/{id}/messages/send`
  发回房间。

这层 bridge 可以是：

- agent 启动时由 `codex` runtime 旁路拉起的 goroutine，或
- server 内统一管理的 `CodexBridgeService`

第一版更建议第二种：server 侧统一托管 bridge，runtime 只暴露“如何拿到 agent 的 ACP session”。

原因：

- bridge 本质是消息面，不是进程面。
- 更容易复用 `internal/api/bot_compat.go` 现有恢复逻辑。
- 更容易为不同 runtime 做不同 delivery policy。

### E. ACP SessionUpdate 先做规范化，再渲染

不要把 ACP 原始事件直接拼字符串发到房间。

建议在 bridge 里先定义一层轻量标准事件，例如：

```go
type EventKind string

const (
    EventTextDelta     EventKind = "text_delta"
    EventToolCallStart EventKind = "tool_call_start"
    EventToolCallEnd   EventKind = "tool_call_end"
    EventThoughtDelta  EventKind = "thought_delta"
    EventTurnDone      EventKind = "turn_done"
    EventRuntimeError  EventKind = "runtime_error"
)
```

然后把 ACP `SessionUpdate` 映射为内部事件，再由 renderer 决定哪些要发到 IM：

- `AgentMessageChunk` -> 增量文本，可做聚合后发送。
- `ToolCall` -> 发一条“正在调用工具: xxx”。
- `ToolCallUpdate` -> 发一条“工具完成/失败: xxx”。
- `AgentThoughtChunk` -> 第一版建议默认不外发，或仅 debug 日志记录。

这样后面如果还有 OpenClaw、Claude Code runtime，也能复用同一套消息输出模型。

### F. ACP 权限请求策略要提前定死

`example_codex_acp.go` 里 `RequestPermission` 现在是 REPL 交互式选择。服务端场景不能这么做。

第一版建议做成固定策略：

- 默认 auto-approve 安全白名单选项。
- 若没有可接受选项，则拒绝，并把拒绝结果记录日志。

更具体一点：

- 优先选择 `AllowOnce`。
- 若不存在，再考虑 `AllowAlways`。
- 若策略不允许，则 `Cancelled`。

不要在第一版里做复杂的人机审批回路，否则 bridge、IM、runtime、权限状态机会一起复杂化。

### G. 文件与 terminal ACP 能力分两阶段

`example_codex_acp.go` 里已经列出了：

- `ReadTextFile`
- `WriteTextFile`
- `CreateTerminal`
- `TerminalOutput`
- `WaitForTerminalExit`
- `KillTerminal`

第一版建议：

- 文件能力按 workspace 路径落地，实现完整。
- terminal 能力先做最小闭环：
  - 能创建
  - 能记录输出
  - 能等待退出

如果 terminal 还没想清楚，也可以第一阶段先只支持最简单实现，但要满足 Codex agent 基本工具调用不直接报“unimplemented”。

### H. delivery policy 需要与 PicoClaw 区分

PicoClaw 当前是“消息到了就从 `/events` 推给 agent 侧 consumer”；Codex ACP session 往往是串行 turn 模型。

所以 Codex bridge 需要显式定义 busy policy：

- 若当前 session 正在处理上一条 prompt：
  - 方案 1：排队。
  - 方案 2：拒绝并提示忙碌。
  - 方案 3：合并成下一次 prompt。

第一版建议：

- 每个 bot 一个串行队列。
- bridge 内保证同一时刻只发送一个 `PromptRequest`。
- 新消息进入内存队列，前一轮 `turn_done` 后继续发送。

这是最稳妥的做法，也最符合 ACP/Codex 的交互模型。

### I. runtime kind 必须从“按 role 推导”改为“显式存储”

这是落地 Codex 的前置改造。

当前：

- `runtimeKindForAgent(a Agent)` 按 role 推导。
- `CreateWorker` 固定取 `RuntimeKindPicoClawSandbox`。

建议改成：

- `Agent` 增加 `RuntimeKind string`
- `CreateAgentSpec` / `CreateAgentRequest` 增加 `RuntimeKind`
- `persistedAgent` 一并持久化
- `runtimeKindForAgent(a Agent)` 改成：
  - agent 上有显式值则用显式值
  - 没有则按旧逻辑回退，保证兼容旧数据

否则 Codex worker 永远会被解释为 PicoClaw worker。

### J. manager 是否使用 Codex runtime

第一版不建议动 manager runtime。

建议范围收敛为：

- manager 继续用 `picoclaw_sandbox`
- 只有 worker 支持显式选择 `codex`

原因：

- manager 当前依赖大量 PicoClaw 目录、workspace、技能和调度假设。
- 先把 worker runtime 多态跑通，风险最小。

## 推荐模块拆分

建议新增或修改的模块如下。

### 新增

- `internal/runtime/codex/runtime.go`
- `internal/runtime/codex/process.go`
- `internal/runtime/codex/session.go`
- `internal/runtime/codex/store.go`
- `internal/codexacp/installer.go`
- `internal/codexacp/locator.go`
- `internal/channel/codexbridge/bridge.go`
- `internal/channel/codexbridge/sse_client.go`
- `internal/channel/codexbridge/render.go`

### 修改

- `internal/runtime/runtime.go`
  - 保留 `KindCodex`
  - 如有必要补充 codex 日志/metadata 约定
- `internal/agent/model.go`
  - `Agent` / `CreateAgentSpec` 增加 `RuntimeKind`
- `internal/apitypes/agent.go`
  - API 入参与出参增加 `runtime_kind`
- `cli/agent/agent.go`
  - `agent create` 支持 `--runtime`
- `internal/agent/runtime_record.go`
  - runtime kind 由显式字段优先
- `internal/agent/service.go`
  - `CreateWorker` 根据 spec/runtime kind 选 runtime
- `internal/agent/runtime_state.go`
  - `runtimeForAgent()` 基于 agent.RuntimeKind
- `cli/serve/serve.go` 或相关 wiring
  - 注册 codex runtime
  - 注册 codex bridge service

## 增量修改步骤

下面按“每一步可独立提交、可单独验证”的方式拆。

### 第 1 步：补 runtime kind 显式化

修改目标：

- `Agent` 增加 `RuntimeKind`
- `CreateAgentSpec` / `CreateAgentRequest` 增加 `RuntimeKind`
- 持久化结构同步增加字段
- `runtimeKindForAgent()` 改为显式值优先，旧逻辑回退

原因：

- 这是 Codex runtime 真正可选中的前提。

验证：

- 旧 agent 数据不丢兼容。
- 新建 worker 可保存 `runtime_kind=codex`。
- `RuntimeView()` 能返回正确 runtime kind。

### 第 2 步：让 CreateWorker 按 runtime kind 选择实现

修改目标：

- `internal/agent/service.go#CreateWorker`
- 移除对 `RuntimeKindPicoClawSandbox` 的硬编码。
- 改为读取 `spec.RuntimeKind`，为空时默认 PicoClaw。

同时约束：

- manager 仍固定 PicoClaw。
- worker 才允许选 `codex`。

验证：

- 未指定 runtime 时行为与现状一致。
- 指定 `codex` 时创建逻辑走到 codex runtime。

### 第 3 步：实现 `codex-acp` locator/installer

修改目标：

- 新增 `internal/codexacp/*`

能力：

- 解析配置路径。
- 解析缓存路径。
- 不存在时下载。
- 返回最终 binary path。

建议同时加一个面向测试的接口：

```go
type BinaryProvider interface {
    Ensure(ctx context.Context) (string, error)
}
```

验证：

- 已存在 binary 时不重复下载。
- 缓存缺失时能完成安装。
- 权限位正确。

### 第 4 步：实现 host-process 型 `codex` runtime

修改目标：

- 新增 `internal/runtime/codex/*`

能力：

- 创建 runtime home。
- 启动 `codex-acp` 子进程。
- 建立 ACP 连接。
- 初始化 session。
- 保存 pid/session metadata。
- 提供 `Start/Stop/Delete/State/Info/StreamLogs`。

建议设计一个内部 session manager：

```go
type Manager interface {
    Start(ctx context.Context, agentID string, spec Spec) (Handle, error)
    Stop(ctx context.Context, handle Handle) error
    Session(handle Handle) (*Session, error)
}
```

这样 bridge 以后只依赖 session manager，不需要关心 runtime 细节。

验证：

- 启停稳定。
- 异常退出能识别为 failed/exited。
- 重启后能恢复最小状态。

### 第 5 步：把 ACP client callback 从示例代码收敛成服务实现

修改目标：

- 从 `example_codex_acp.go` 提炼：
  - permission policy
  - file ops
  - terminal ops
  - session update handler

注意：

- 示例里是 REPL/stdio 交互。
- 服务实现里要改成：
  - structured logging
  - runtime session state machine
  - bridge 可订阅的事件流

建议抽成：

```go
type SessionEventSink interface {
    Publish(SessionEvent)
}
```

验证：

- `PromptRequest` 可收到文本 delta。
- tool call/update 能被捕获。
- permission request 能自动处理。

### 第 6 步：实现 `codexbridge`

修改目标：

- 新增 `internal/channel/codexbridge/*`

能力：

- 连 `GET /api/bots/{id}/events`
- 消费 SSE
- 过滤并解析 bot event
- 按 botID 找到 codex session
- 串行发送 `acp.PromptRequest`
- 监听 ACP event sink
- 回发 `/api/bots/{id}/messages/send`

建议 bridge 内部维护：

- 每 bot 一个 worker goroutine
- 一个输入消息队列
- 一个“当前 turn 是否忙碌”的状态
- 一个近端去重集合，避免重连 replay 时重复 prompt

验证：

- 单条消息可打通完整往返。
- 重连 SSE 不会重复回答同一消息。
- 忙碌期间多条消息可排队。

### 第 7 步：接入 server wiring

修改目标：

- `cli/serve/serve.go`
- 相关 app wiring

内容：

- 注册 `codex` runtime 到 `agent.Service`
- 创建 `codexbridge` service
- server 启动时对所有 `runtime_kind=codex` 且状态需要运行的 agent 启动 bridge
- server 关闭时优雅停止 bridge

验证：

- 进程启动后 codex worker 能自动建立 bridge。
- 重启 server 后 bridge 可恢复。

### 第 8 步：补 CLI / API / 文档

修改目标：

- `cli/agent/agent.go`
- `docs/tech/api.md`
- `README.md` / `README.zh.md` 相关段落

内容：

- `agent create --runtime codex`
- API 支持 `runtime_kind`
- 如有必要，补一个“如何准备 codex-acp”的运维说明

验证：

- CLI 能创建 codex worker。
- API 返回中能看到 `runtime_kind=codex`。

### 第 9 步：补测试

优先级建议：

1. 单元测试
2. 轻量集成测试
3. 端到端手工验证

至少补这些测试：

- `runtime kind` 持久化与兼容测试
- `CreateWorker` runtime 选择测试
- `codexacp` locator/installer 测试
- `codex runtime` 状态机测试
- `codexbridge` 去重、排队、回发测试
- `/api/bots/*` replay 与恢复相关回归测试

## 关键实现决策

### 决策 1：bridge 不进 `Runtime`

原因：

- 现有 PicoClaw 已经证明“生命周期”和“消息面”分开更稳定。
- `/api/bots/*` 已经是成熟兼容层。
- 如果把对话接口塞进 `Runtime`，后面很容易把 API、IM、ACP 强耦合在一起。

### 决策 2：第一版只支持 worker 使用 Codex

原因：

- manager 对 PicoClaw 生态耦合太深。
- 先做 worker，可最小化影响面。

### 决策 3：busy policy 先串行排队

原因：

- 最符合 ACP turn 模型。
- 易于保证消息顺序和幂等。

### 决策 4：ACP 事件先规范化再输出

原因：

- 避免把 runtime 私有协议扩散到全项目。
- 后续可支持多 runtime 共用同一消息渲染层。

## 风险与注意点

### 1. 当前 agent 持久化结构可能没有 runtime kind

所以必须保留回退逻辑，避免旧数据升级失败。

### 2. `/api/bots/{id}/events` 是 replay + SSE 语义

bridge 必须做幂等，不能简单把每次收到的事件都转成新的 `PromptRequest`。

建议至少用：

- `message_id`
- `room_id`
- 最近处理窗口

做去重。

### 3. ACP 输出是增量流

如果每个 chunk 都发 IM，会严重刷屏。

建议第一版策略：

- 文本先在 bridge 内聚合。
- 到 `turn_done` 或达到时间/长度阈值后再发。

### 4. tool call 是否对用户可见要克制

第一版只发高价值状态：

- 开始执行工具
- 工具完成/失败

不要把细碎内部事件全发出来。

### 5. 下载器要注意离线/代理/失败重试

即便第一版不把所有网络环境问题都处理完，也要做到：

- 错误可解释。
- 日志明确。
- 支持手动指定 binary path 作为兜底。

## 建议的首个落地顺序

如果要尽快进入代码实现，建议按下面顺序开工：

1. runtime kind 显式化
2. `CreateWorker` runtime 选择
3. `codex-acp` locator/installer
4. `codex` runtime 生命周期
5. ACP callback 事件规范化
6. `codexbridge`
7. wiring
8. CLI/API/文档
9. 测试补齐

这个顺序的好处是：

- 前三步先把架子和依赖准备好。
- 中间三步把核心链路打通。
- 最后三步做接线、暴露和回归。

## 最小可用版本定义

满足下面这些条件，就可以认为第一版可用：

- 能创建 `runtime_kind=codex` 的 worker。
- 启动时自动准备 `codex-acp`。
- 能通过 `/api/bots/{id}/events` 收到房间消息。
- 能将消息转成 `acp.PromptRequest`。
- 能将 Codex 文本回复通过 `/api/bots/{id}/messages/send` 发回房间。
- 能处理最基础的 tool call 状态提示。
- 重连后不会重复处理同一条消息。
- agent stop/delete 能正确回收 `codex-acp` 进程。

## 不建议的实现方式

以下做法不建议采用：

- 直接把 SSE 监听逻辑塞进 `Runtime.Start()`
- 继续让 worker runtime 只按 role 推导
- 把 ACP 原始 payload 散落在多个 package 里直接使用
- 每个文本 chunk 都直接发 IM
- 把下载 `codex-acp` 的逻辑写死在 `CreateWorker`

这些做法都会让后续维护成本明显升高。

## 结论

这次改造最稳妥的落点是：

- 用 `internal/runtime/codex` 实现 host-process 型 ACP runtime。
- 用单独的 `codexbridge` 复用 `/api/bots/*` 完成消息面桥接。
- 先把 `runtime kind` 从隐式推导改成显式选择。
- 第一版仅支持 worker 使用 Codex，manager 保持 PicoClaw。

如果按上面的增量步骤推进，可以在不扰动现有 PicoClaw 主链路的前提下，把 Codex runtime 以较低风险落进当前架构。
