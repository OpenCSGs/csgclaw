# Session API Demo 前端指南

CSGClaw 运行后，可通过 `/#/session-demo` 访问独立 demo。
该页面是真实 Agent Session API client，不是模拟聊天页面。

## 目标

该页面展示创建或复用 Agent session 所需的最小专业前端边界。
页面独立于主 workspace shell，但复用现有 React providers、design tokens、UI primitives、国际化和 Vite build。

Demo 覆盖以下行为：

- 页面只加载具备 session endpoint 所需本地 active participant 的 Agent。
- 页面通过 `crypto.randomUUID()` 生成 session ID，也允许输入符合 path-safe 规则的现有 session ID。
- 页面调用 `POST /api/v1/agents/{agent}/sessions/{session_id}/responses` 并渲染最终 `output_text`。
- 页面在第一次请求前展示确定性的审计 room title。
- 页面说明 Anonymous 以 `user-admin` 持久化，并且 notify-all 会保持开启。
- 页面禁止重叠发送，并允许取消当前 HTTP 请求。
- 页面在浏览器本地保存最多 20 个 session，每个 session 最多保存 100 条消息。

## 源码边界

源码按职责拆分，使专业前端或测试 mock 可以单独替换每一层。

- `src/api/agentSessions.ts` 负责真实 HTTP 请求和 OpenAI 风格错误提取。
- `src/models/agentSessions.ts` 负责 wire types、room title 格式化和 session 校验。
- `src/pages/SessionDemoPage/sessionDemoStorage.ts` 负责 transcript 上限和 storage 归一化。
- `src/pages/SessionDemoPage/useSessionDemo.ts` 负责页面状态机并导出 `SessionDemoTransport` seam。
- `src/pages/SessionDemoPage/SessionDemoPage.tsx` 负责无障碍渲染和用户交互。
- `src/pages/SessionDemoPage/SessionDemoPage.module.css` 负责响应式页面展示。

`SessionDemoTransport` 是 mock 边界：

```ts
export type SessionDemoTransport = {
  listAgents: () => Promise<SessionAgent[]>;
  createResponse: (request: CreateAgentSessionResponseRequest) => Promise<AgentSessionResponse>;
};
```

确定性的 component test 可以向 `useSessionDemo` 传入 fixture implementation，而不需要拦截全局 `fetch`。
生产 client 也可以用同一个 interface 接入生成的 SDK、gateway 或其它请求库。

## Session 与 Transcript 行为

URL query parameters 为 `agent` 和 `session`，例如 `/#/session-demo?agent=reviewer&session=review-01`。
Demo 的页面 URL 和 Agent Session API request path 都使用稳定的 Agent name，持久化 room metadata 仍保留规范 Agent ID。
URL 中的显式值优先于上次保存的选择。

服务端拥有 room history 和 runtime conversation context。
Demo 只把可见且成功的 user 与 assistant 消息保存到 `csgclaw.session-demo.v1` localStorage。
从另一个浏览器复用服务端 session 时，Agent context 会继续，但可见 transcript 从空白开始。

第一次成功 turn 后 Agent selector 会锁定，因为全局 session 不能切换 Agent。
New Session action 会生成新的 client ID、清空可见状态并解锁 Agent selection。
Room 在第一条消息发送时才会真正创建。

## 错误与取消行为

Transport 会从 API envelope 中提取 `error.message` 与 `error.code`，同时保留 HTTP status。
失败或取消的输入会回到 composer，用户可以编辑或重试。
取消会终止浏览器请求，服务端会请求 runtime cancellation，并在 turn 结束或 cleanup grace period 到期前保持 session busy。

## 生产强化

Demo 面向可信的本地 `csgclaw serve` 边界。
对外暴露的生产应用需要增加 authentication、authorization、rate limiting、origin controls、审计保留策略，以及明确的 admin 代理信任模型。

V1 API 不提供 streaming output、服务端 transcript read endpoint、attachments 或 session ownership token。
生产前端应在 API 边界增加这些能力，而不是从 localStorage 推断服务端状态。
