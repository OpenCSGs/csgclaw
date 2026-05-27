# CSGClaw IM Threads

本文说明 CSGClaw 本地 IM 的 thread 设计，方便维护者理解 thread 在存储、
API、bot 兼容层、agent 上下文和 Web UI 中的协作方式。

## 摘要

CSGClaw 在现有 IM API 内增量采用 Matrix 形状的 thread 模型，但当前不实现
完整 Matrix Client-Server 协议。这样可以复用 Matrix `m.thread` 关系语义，
同时保留 CSGClaw 现有的 room、user、bot、auth 和本地状态模型。

一个 thread 是 room 或 DM 内的子会话。它从一条已有顶层消息开启，这条消息
称为 root message。规范 thread ID 就是 root message ID。

Thread reply 使用 Matrix 风格的 relation 元数据：

```json
{
  "relates_to": {
    "rel_type": "m.thread",
    "event_id": "msg-root"
  }
}
```

## 为什么当前不做完整 Matrix

本地 IM 当前没有 Matrix homeserver 语义，例如 Matrix auth、`/sync`、event
ID、room state、power level、federation 或 state resolution。如果现在直接
替换成原始 Matrix API，会让现有 CSGClaw 概念和半套 Matrix 行为产生割裂。

因此当前策略是保留现有 API namespace 和数据模型，只在能自然映射的地方加入
Matrix 形状的 relation 字段和 relation 查询接口。

## 核心模型

- 一个 thread 只属于一个 room 或 DM。
- Thread root 必须是同一 room 内已有的顶层消息。
- 不允许嵌套 thread root；thread reply 不能再开启新 thread。
- Root message ID 是稳定的 thread ID。
- Thread reply 是普通 message，但带有 `relates_to.rel_type = "m.thread"` 和
  `relates_to.event_id = <root_message_id>`。
- 当 thread 存在时，root message 会带有 `thread` summary。
- Thread reply 自身不带 thread summary。

## 持久化

消息仍以 JSONL 保存。Thread 字段都是可选字段，因此旧消息记录仍可读取。

每个 room 还可以持久化 `threads`，其中每个 `ThreadState` 包含：

- `root_message_id`：规范 root/thread ID。
- `created_at`：显式开启 thread 的时间。
- `context`：thread 开启时捕获的隐藏上下文快照。
- `summary`：用于展示和 agent prompt 的确定性上下文摘要。

`ThreadState` 是 room 内局部状态。未来如果添加完整 Matrix adapter，应把这层
状态翻译为 Matrix relation，而不是先改掉本地存储契约。

## 隐藏上下文快照

开启 thread 时会捕获 root 周围的隐藏上下文：

- root 前最多 5 条顶层消息，
- root 本身，
- root 后最多 2 条顶层消息，
- 同时受 payload 大小限制。

这些上下文不会插入为可见 thread 消息。它的目的，是让 LLM-backed agent 在
thread 中获得一个干净的新会话，同时仍理解 thread 是从什么语境中开启的。

v1 的 summary 是确定性生成，不调用 LLM：

- `root_excerpt`
- `message_count`
- `before_count`
- `after_count`

预览文本会去掉带 language label 的开头 markdown code fence，例如 `text`，
这样 thread 列表和面板标题展示的是实际内容，而不是格式标记。

## 时间线语义

Thread reply 默认不进入 room/DM 主时间线。这接近 Slack 的 thread 行为，也
能保持 room 可读。

默认时间线入口只返回顶层消息：

- `GET /api/v1/messages?room_id=...`
- `GET /api/v1/rooms`
- `GET /api/v1/bootstrap`
- csgclaw channel mirror 路由

如果调用方需要包含 thread replies 的完整消息历史，可以在消息列表接口添加
`include_thread_replies=true`。

发送 thread reply 仍会产生 `message.created`，同时也会通过
`thread.updated` 更新 root thread summary。

## API 入口

Thread API 位于 CSGClaw namespace，不使用原始 `/_matrix` namespace：

```text
POST /api/v1/rooms/{room_id}/threads
GET  /api/v1/rooms/{room_id}/threads?include=all|participated&limit=&from=
GET  /api/v1/rooms/{room_id}/threads/{root_message_id}
GET  /api/v1/rooms/{room_id}/relations/{event_id}/m.thread
POST /api/v1/messages
GET  /api/v1/messages?room_id=...&include_thread_replies=true
```

csgclaw channel mirror 也在
`/api/v1/channels/csgclaw/rooms/{room_id}/...` 下暴露等价 thread 路由。

请求和响应结构见 [api.zh.md](./api.zh.md)。

## 事件

本地 IM 事件流可能发布：

- `thread.created`：为 root message 创建了新的 `ThreadState`。
- `thread.updated`：thread replies 或 summary 数据发生变化。
- `message.created`：普通消息和 thread reply 都仍会发布。

Thread-aware 客户端应把 `thread.created` 和 `thread.updated` 应用到 root
message summary 和 thread 列表；处理 `message.created` 时，只有非 thread
reply 才进入主时间线。

## Bot 兼容与 PicoClaw

Bot 兼容 API 是 PicoClaw 风格集成和 Codex bridge 使用的消息桥：

```text
GET  /api/bots/{id}/events
POST /api/bots/{id}/messages/send
```

Thread-aware bot event 可能包含：

- `thread_root_id`：事件位于 thread 内时的 root message ID。
- `thread_context`：该 thread root 的隐藏上下文快照和 summary。
- `context.topic_id`：PicoClaw 原生 topic/session ID。对 CSGClaw IM
  threads 来说，它与 `thread_root_id` 相同。

`thread_context` 是 prompt context，不是可见 thread 历史。

Bot send 可以传入 CSGClaw 字段（`room_id`、`text`、`thread_root_id`），
也可以传入 PicoClaw outbound 字段（`chat_id`、`content`、
`context.topic_id`）。存在 thread root/topic 时，消息会作为该 thread 内的
reply 发送。如果 bot send 同时省略 `thread_root_id`、`topic_id` 和
`context.topic_id`，CSGClaw 会按 room/DM 顶层消息处理，不会根据该 bot 在
房间中最近收到的事件推断 thread。

这对应 PicoClaw/topic 隔离需求：runtime 应把 `room_id` 视为普通会话 key，
把 `room_id:thread_root_id` 视为 thread 会话 key。生成的 PicoClaw config
会把 session dimensions 设为 `["chat", "topic"]`，因此
`context.topic_id` 会为每个 CSGClaw IM thread 建立独立的 PicoClaw session。

## Codex Bridge 会话隔离

Codex bridge 根据消息作用域派生 runtime conversation identity：

- room/DM 顶层消息：`room_id`
- thread 消息：`room_id:thread_root_id`

这样可以避免 thread 工作污染 room 级 Codex session，也避免不同 thread 共享
prompt/tool 上下文。

Tool-call result message 会附着到对应 response thread 中，减少 room 主时间线
噪声，同时保留工具调用可追踪性。

## 删除用户与 Thread 清理

删除用户会从 room history 中移除该用户发送的消息。随后 thread state 会基于
剩余消息重建：

- 被删除用户发送的 thread root 会被移除；
- 隐藏上下文快照会重新生成，不包含已删除用户的消息；
- 被删除用户的 replies 不再进入 thread summary；
- 能保留原 thread 创建时间时会尽量保留。

## UI 行为

Web UI 采用接近 Slack 的 thread 行为：

- message row 悬停时显示 “Reply in thread” 操作；
- thread reply 在右侧 thread panel 中打开；
- thread composer 默认只回复到 thread；
- thread panel 会自动滚到最新 reply；
- thread 列表按 room/DM 分组；
- thread 预览使用清理后的短文本；
- thread panel 不应让 room 时间线产生横向滚动。

当前 Threads 是 Messages 旁边的独立 workspace tab，因为它作为跨 room 的
thread inbox 使用。它仍然属于消息域视图；如果未来 UI 增加 Messages 下的
二级导航，可以把这个视图移动过去，而不需要改存储或 API 模型。

## 非目标

- 本次不实现完整 Matrix homeserver 行为。
- 不暴露原始 `/_matrix` Client-Server namespace。
- 在 Feishu adapter 暴露稳定 thread/topic ID 前，不实现 Feishu-native
  thread/topic 支持。
- v1 不使用 LLM 生成 thread summary。
