# CSGClaw API 简要文档

本文基于当前代码中的实际 HTTP 路由整理，默认服务地址为 `http://127.0.0.1:18080`。

## 约定

- 内容类型：除 SSE 接口外，请求和响应均为 `application/json`
- 时间格式：使用 RFC3339 / ISO8601，例如 `2026-03-28T12:00:00Z`
- 认证：大部分接口当前不需要认证；`/api/bots/*` 和 `GET /api/v1/channels/feishu/bots/{id}/events` 需要 `Authorization: Bearer <token>`
- 错误返回：失败时通常返回纯文本错误信息，不是统一 JSON 结构

## 1. 基础接口

### `GET /healthz`

健康检查。

响应示例：

```text
ok
```

## 2. Worker 接口

统一后的 `worker` 对象字段如下。除 PicoClaw Bot 兼容接口外，`bot / worker / agent` 在 API 和内部结构里都按这一套字段表达：

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "role": "worker",
  "status": "running",
  "created_at": "2026-03-28T12:00:03Z",
  "profile": "codex.gpt-5.4",
  "provider": "llm-api",
  "model_id": "gpt-5.4",
  "reasoning_effort": "medium",
  "image": "ghcr.io/russellluo/picoclaw:2026.4.8.1"
}
```

补充说明：

- `role` 当前常见值：`manager`、`worker`、`agent`
- `image` 仍可能出现在响应中，用于表示容器镜像；它不是统一身份字段的一部分
- `/api/v1/workers` 只是 `worker` 视角的路由别名，返回对象仍然是统一的 `agent`

### `GET /api/v1/workers`

获取全部 `role=worker` 的 agent 列表。

### `POST /api/v1/workers`

创建 `role=worker` 的 agent，并自动同步到 IM 用户体系。

请求体：

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "profile": "codex.gpt-5.4",
  "role": "worker"
}
```

响应：`201 Created`

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "role": "worker",
  "status": "running",
  "created_at": "2026-03-28T12:00:03Z",
  "profile": "codex.gpt-5.4",
  "provider": "llm-api",
  "model_id": "gpt-5.4",
  "reasoning_effort": "medium",
  "image": "ghcr.io/russellluo/picoclaw:2026.4.8.1"
}
```

说明：

- `name` 必填
- `name` 不能是 `manager`
- `id` 可选；未传时服务端会自动生成
- `profile` 可选；它引用配置中的 `models.default` 或显式 selector（例如 `codex.gpt-5.4`）
- `provider`、`model_id`、`reasoning_effort` 是服务端解析后的快照字段，便于调试
- `status`、`created_at` 以实际 box 启动结果为准
- `manager` 嵌套字段已不再支持
- 若 IM 服务可用，会自动创建对应 IM 用户，并创建 `Admin & <Worker>` 私聊
- 校验失败通常返回 `400 Bad Request`

## 3. IM 接口

### `GET /api/v1/im/bootstrap`

获取 IM 初始化数据，供 WebUI 首次加载使用。
响应返回 `rooms`。

响应示例：

```json
{
  "current_user_id": "u-admin",
  "users": [
    {
      "id": "u-admin",
      "name": "Admin",
      "handle": "admin",
      "role": "Admin",
      "avatar": "AD",
      "is_online": true,
      "accent_hex": "#dc2626"
    }
  ],
  "rooms": []
}
```

### `GET /api/v1/im/events`

订阅 IM 事件流，返回 `text/event-stream`。

事件格式：

```text
: connected

data: {"type":"message.created","room_id":"room-1","message":{"id":"msg-1","sender_id":"u-admin","content":"hello","created_at":"2026-04-13T11:15:01.848093Z","mentions":["u-alice"]},"sender":{"id":"u-admin","name":"Admin","handle":"admin","role":"Admin","avatar":"AD","is_online":true,"accent_hex":"#dc2626"}}
```

当前事件类型：

- `message.created`
- `room.created`
- `room.members_added`

### `POST /api/v1/im/messages`

在 room 中发送消息。

请求体：

```json
{
  "room_id": "room-1",
  "sender_id": "u-admin",
  "content": "hello @alice"
}
```

响应：`201 Created`

```json
{
  "id": "msg-1743139200000000001",
  "sender_id": "u-admin",
  "content": "hello @alice",
  "created_at": "2026-03-28T12:00:00Z",
  "mentions": [
    "u-alice"
  ]
}
```

说明：

- `room_id`、`sender_id`、`content` 必填
- `content` 中的 `@handle` 会解析为 `mentions`

### `POST /api/v1/im/conversations`

创建新 room。

请求体：

```json
{
  "title": "Frontend Sync",
  "description": "Discuss frontend tasks",
  "creator_id": "u-admin",
  "participant_ids": ["u-manager", "u-alice"],
  "locale": "zh-CN"
}
```

响应：`201 Created`

```json
{
  "id": "room-1743139200000000000",
  "title": "Frontend Sync",
  "subtitle": "3 members",
  "description": "Discuss frontend tasks",
  "participants": ["u-admin", "u-manager", "u-alice"],
  "messages": [
    {
      "id": "msg-1743139200000000002",
      "sender_id": "u-admin",
      "content": "已创建房间“Frontend Sync”，欢迎大家加入。",
      "created_at": "2026-03-28T12:00:00Z"
    }
  ]
}
```

说明：

- `title`、`creator_id` 必填
- `participant_ids` 会和 `creator_id` 合并去重
- 返回里的 `subtitle` 会根据人数自动生成

### `POST /api/v1/im/conversations/members`

向 room 中添加成员。

请求体：

```json
{
  "room_id": "room-1",
  "inviter_id": "u-admin",
  "user_ids": ["u-alice", "u-bob"],
  "locale": "zh-CN"
}
```

响应：`200 OK`，返回更新后的 room 对象。

说明：

- `room_id`、`inviter_id`、`user_ids` 必填
- `inviter_id` 必须已经在 room 内
- 若没有任何新成员被加入，会返回 `400 Bad Request`

### `GET /api/v1/rooms`

获取全部会话列表，按最近消息时间倒序返回。

### `POST /api/v1/rooms`

创建新 room。请求体与 `POST /api/v1/im/conversations` 一致，响应：`201 Created`。

### `DELETE /api/v1/rooms/{id}`

删除指定 room。

响应：`204 No Content`

说明：

- `id` 必须是已存在会话
- 若 room 不存在，返回 `404 Not Found`

### `GET /api/v1/users`

获取全部用户列表，按名称排序返回。

### `DELETE /api/v1/users/{id}`

Kick 指定用户。

响应：`204 No Content`

说明：

- 当前语义为“全局移除用户，并从所有会话成员及历史消息中清理该用户”
- 若某个会话清理后剩余成员少于 2 人，该会话会被一并删除
- 不允许 kick 当前用户；此时返回 `409 Conflict`
- 若用户不存在，返回 `404 Not Found`

### `GET /api/v1/messages`

获取指定会话的消息历史。查询参数为 `room_id`。

### `POST /api/v1/messages`

发送消息。请求体与 `POST /api/v1/im/messages` 一致，响应：`201 Created`。

### `POST /api/v1/im/agents/join`

把 agent 加入指定会话。该接口会先确保 agent 在 IM 中拥有对应用户身份。

请求体：

```json
{
  "agent_id": "u-alice",
  "room_id": "room-1",
  "inviter_id": "u-admin",
  "locale": "zh-CN"
}
```

响应：`200 OK`，返回更新后的会话对象。

说明：

- `agent_id`、`room_id` 必填
- `inviter_id` 为空时，服务端默认使用 `u-admin`
- 若 `agent_id` 不存在，返回 `404 Not Found`

## 5. Feishu Channel 接口

### `GET /api/v1/channels/feishu/bots/{id}/events`

订阅指定 Feishu bot 的 channel message bus 事件流，返回 `text/event-stream`。例如：

```http
GET /api/v1/channels/feishu/bots/u-manager/events
```

服务端只会推送 `mentions` 包含该 bot 对应 Feishu `open_id` 的消息事件。

返回示例：

```text
: connected

data: {"type":"message.created","room_id":"oc_f778","message":{"id":"om_x100","sender_id":"ou_323c","kind":"message","content":"what skills are available?","created_at":"2026-04-13T11:15:01.848093Z","mentions":["ou_2074"]}}
```

认证要求：

- 请求头必须带 `Authorization: Bearer <token>`
- token 来自 `~/.csgclaw/config.toml` 中的 `[server].access_token`

## 6. 兼容别名接口

当前以下接口是同义路由，行为与上文一致：

- `GET /api/v1/bootstrap` 等价于 `GET /api/v1/im/bootstrap`
- `GET /api/v1/events` 等价于 `GET /api/v1/im/events`
- `POST /api/v1/rooms/invite` 等价于 `POST /api/v1/im/conversations/members`
- `POST /api/v1/im/rooms` 等价于 `POST /api/v1/im/conversations`
- `POST /api/v1/im/rooms/invite` 等价于 `POST /api/v1/im/conversations/members`

## 7. PicoClaw Bot 接口

这组接口用于 PicoClaw 与 IM 的双向通信，以及 Worker 访问服务端暴露的 OpenAI 兼容 LLM bridge。

认证要求：

- 请求头必须带 `Authorization: Bearer <token>`
- token 来自 `~/.csgclaw/config.toml` 中的 `[server].access_token`
- 若服务端配置为空，则不校验；默认初始化值为 `your_access_token`

### `GET /api/bots/{bot_id}/events`

订阅 bot 事件流，返回 `text/event-stream`。

响应示例：

```text
: connected

event: message
data: {"message_id":"msg-1","room_id":"room-1","chat_type":"direct","sender":{"id":"u-admin","username":"admin","display_name":"Admin"},"text":"hello","timestamp":"1743139200000"}
```

说明：

- 支持的 `chat_type` 为 `direct` 或 `group`
- 私聊中，bot 只要在会话里就会收到消息
- 群聊中，只有被 `@bot` 时才会收到消息
- bot 自己发出的消息不会被回推

### `POST /api/bots/{bot_id}/messages/send`

bot 向指定会话发送消息。

请求体：

```json
{
  "room_id": "room-1",
  "text": "hello from bot"
}
```

响应：`200 OK`

```json
{
  "message_id": "msg-1743139200000000003"
}
```

说明：

- 消息发送者固定为路径中的 `bot_id`
- `room_id` 必须是已存在会话
- `text` 不能为空

### `GET /api/bots/{bot_id}/llm/v1/models`

返回 bot 当前可见的模型列表，格式兼容 OpenAI `GET /v1/models`。

响应示例：

```json
{
  "object": "list",
  "data": [
    {
      "id": "gpt-5.4",
      "object": "model",
      "created": 0,
      "owned_by": "csgclaw"
    }
  ]
}
```

说明：

- 服务端会根据 `bot_id` 对应 agent 的 `profile` 解析实际模型配置，并在响应中保留已解析快照字段
- box 内看到的是统一的 OpenAI 兼容接口；不会拿到宿主机上的真实上游 `api_key`

### `POST /api/bots/{bot_id}/llm/v1/chat/completions`

OpenAI 兼容的聊天补全入口。

请求体示例：

```json
{
  "model": "ignored-by-server",
  "messages": [
    {
      "role": "user",
      "content": "Review this patch."
    }
  ]
}
```

响应示例：

```json
{
  "id": "chatcmpl-1",
  "object": "chat.completion",
  "created": 1743139200,
  "model": "gpt-5.4",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "..."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 0,
    "completion_tokens": 0,
    "total_tokens": 0
  }
}
```

说明：

- 请求会由服务端转发到对应 profile 配置中的 `base_url + /chat/completions`
- `model` 字段会被服务端强制改写为该 agent 解析出的 `model_id`
- 若 profile 配置了 `reasoning_effort`，且请求体没有显式提供 `reasoning_effort`，服务端会把该默认值注入转发请求
