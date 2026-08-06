# Session API Demo 前端指南

CSGClaw 运行后，可通过 `/#/session-demo` 访问独立 demo。
该页面是真实 Agent Session API client，不是模拟聊天页面。

## 目标

该页面展示创建或复用 Agent session 所需的最小专业前端边界。
页面独立于主 workspace shell，但复用现有 React providers、design tokens、UI primitives、国际化和 Vite build。

Demo 覆盖以下行为：

- 页面只加载阶段 1 Session Endpoint 支持的运行中 Codex Agent。
- 页面通过 `crypto.randomUUID()` 生成 session ID，也允许输入符合 path-safe 规则的现有 session ID。
- 页面使用 `stream: true` 调用 `POST /api/v1/agents/{agent}/sessions/{session_id}/responses`，并在 Text Delta 到达时立即渲染。
- 页面在第一次请求前展示确定性的 Conversation 标识。
- 页面说明 Session 直接使用 Agent Engine 执行，不创建 IM Room 或 Message。
- 页面禁止重叠发送，并允许显式取消当前 Runtime turn。
- 页面在浏览器本地保存最多 20 个 session，每个 session 最多保存 100 条消息。

## 源码边界

源码按职责拆分，使专业前端或测试 mock 可以单独替换每一层。

- `src/api/agentSessions.ts` 负责真实 HTTP 请求、SSE 解析和 OpenAI 风格错误提取。
- `src/models/agentSessions.ts` 负责 wire types、Conversation Label 格式化和 session 校验。
- `src/pages/SessionDemoPage/sessionDemoStorage.ts` 负责 transcript 上限和 storage 归一化。
- `src/pages/SessionDemoPage/useSessionDemo.ts` 负责页面状态机并导出 `SessionDemoTransport` seam。
- `src/pages/SessionDemoPage/SessionDemoPage.tsx` 负责无障碍渲染和用户交互。
- `src/pages/SessionDemoPage/SessionDemoPage.module.css` 负责响应式页面展示。

`SessionDemoTransport` 是 mock 边界：

```ts
export type SessionDemoTransport = {
  listAgents: () => Promise<SessionAgent[]>;
  streamResponse: (
    request: CreateAgentSessionResponseRequest,
    onTextDelta?: (delta: string) => void,
  ) => Promise<AgentSessionStreamResult>;
  cancelResponse: (request: CancelAgentSessionResponseRequest) => Promise<void>;
};
```

确定性的 component test 可以向 `useSessionDemo` 传入 fixture implementation，而不需要拦截全局 `fetch`。
生产 client 也可以用同一个 interface 接入生成的 SDK、gateway 或其它请求库。

## Session 与 Transcript 行为

URL query parameters 为 `agent` 和 `session`，例如 `/#/session-demo?agent=reviewer&session=review-01`。
Demo 的页面 URL 和 Agent Session API request path 都使用稳定的 Agent name，本地 transcript record 则保留规范 Agent ID。
URL 中的显式值优先于上次保存的选择。

服务端拥有 Runtime Conversation Context。
Demo 只把可见且成功的 user 与 assistant 消息保存到 `csgclaw.session-demo.v1` localStorage。
从另一个浏览器复用服务端 session 时，Agent context 会继续，但可见 transcript 从空白开始。
Streaming Text 是临时 UI State，只在 `message_stop` 完成 Turn 后持久化。

第一次成功 turn 后 Agent selector 会锁定，因为全局 session 不能切换 Agent。
New Session action 会生成新的 client ID、清空可见状态并解锁 Agent selection。
Runtime Conversation 在第一条消息发送时才会按需创建或续接。

## 错误与取消行为

Transport 会从 API envelope 中提取 `error.message` 与 `error.code`，同时保留 HTTP status。
失败或取消的输入会回到 composer，用户可以编辑或重试。
取消会调用 `POST /api/v1/agents/{agent}/sessions/{session_id}/responses/cancel`，由服务端取消 Engine Context，并在 Runtime cleanup 完成后关闭 browser stream。
只有显式取消请求失败时，浏览器才会主动终止 stream。
服务端确认 Runtime cancellation 和 turn cleanup 完成前，composer 会保持禁用，避免新请求与被取消的 turn 发生竞争。

## 生产强化

Demo 面向可信的本地 `csgclaw serve` 边界。
对外暴露的生产应用需要增加 authentication、authorization、rate limiting、origin controls、审计保留策略，以及明确的 admin 代理信任模型。

V1 API 支持 Text 和 Tool Event Streaming，但不提供服务端 Transcript Read Endpoint、Attachment 或 Session Ownership Token。
生产前端应在 API 边界增加这些能力，而不是从 localStorage 推断服务端状态。
