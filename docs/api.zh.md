# CSGClaw API 文档

本文基于当前代码中的实际 HTTP 路由整理，默认服务地址示例为 `http://127.0.0.1:18080`。

## 约定

- 除流式接口外，请求和响应均为 `application/json`
- 时间字段使用 RFC3339 / ISO8601
- 常规错误通常返回纯文本错误正文
- SSE 接口返回 `text/event-stream`
- 当前 API 主要分为 4 组：
  - 核心 API：`/api/v1/*`
  - Channel API：`/api/v1/channels/*`
  - Bot 兼容 API：`/api/bots/*`
  - 健康检查：`/healthz`

## 认证

- 默认大多数 `/api/v1/*` 接口不要求认证
- 以下接口要求 `Authorization: Bearer <token>`，其中 token 为服务端 access token
  - `GET /api/v1/channels/feishu/bots/{id}/events`
  - `GET /api/bots/{id}/events`
  - `POST /api/bots/{id}/messages/send`
  - `GET /api/bots/{id}/llm/models`
  - `GET /api/bots/{id}/llm/v1/models`
  - `POST /api/bots/{id}/llm/chat/completions`
  - `POST /api/bots/{id}/llm/v1/chat/completions`
- 若服务端开启 `no_auth`，上述鉴权会被跳过

## 健康检查

### `GET /healthz`

健康检查。

响应示例：

```text
ok
```

## 核心 API

### `GET /api/v1/version`

返回当前服务版本。

响应示例：

```json
{
  "version": "0.1.0"
}
```

### 升级

#### `GET /api/v1/upgrade/status`

返回升级状态。

响应字段：

- `current_version`
- `latest_version`
- `update_available`
- `checking`
- `upgrading`
- `last_checked_at`
- `last_error`

#### `POST /api/v1/upgrade/apply`

触发升级 helper。

成功时返回 `202 Accepted`：

```json
{
  "status": "accepted",
  "message": "upgrade helper started"
}
```

若升级管理器未配置，返回 `503 Service Unavailable`。

## Bot 管理 API

这组接口挂在 channel API 命名空间下，但底层仍由统一的 `internal/bot` 服务负责编排，当前并没有按 channel 拆成独立 bot service。`role` 仅支持 `manager` 和 `worker`，`channel` 仅支持 `csgclaw` 和 `feishu`。

### `GET /api/v1/channels/{channel}/bots`

获取指定 channel 下的 bot 列表。

路径参数：

- `channel`：`csgclaw` 或 `feishu`

可选查询参数：

- `role`

响应字段：

- `id`
- `name`
- `description`
- `role`
- `channel`
- `agent_id`
- `user_id`
- `available`
- `runtime_kind`
- `created_at`

示例：

- `GET /api/v1/channels/csgclaw/bots`
- `GET /api/v1/channels/feishu/bots?role=worker`

### `POST /api/v1/channels/{channel}/bots`

在指定 channel 下创建 bot。

路径参数：

- `channel`：`csgclaw` 或 `feishu`

请求体示例：

```json
{
  "id": "u-alice",
  "name": "alice",
  "role": "worker",
  "runtime_kind": "codex",
  "from_template": "local/review-bot"
}
```

说明：

- `name` 必填
- `role` 必填，且只能是 `manager` 或 `worker`
- 实际 channel 由路由路径决定，而不是由请求体决定
- `worker` bot 会关联后端 agent
- `manager` / `worker` 在不同 channel 上的创建行为可能不同

示例：

- `POST /api/v1/channels/csgclaw/bots`
- `POST /api/v1/channels/feishu/bots`

### `DELETE /api/v1/channels/{channel}/bots/{id}`

删除指定 channel 下的 bot。

路径参数：

- `channel`：`csgclaw` 或 `feishu`
- `id`：bot ID

成功返回 `204 No Content`。

示例：

- `DELETE /api/v1/channels/csgclaw/bots/u-alice`
- `DELETE /api/v1/channels/feishu/bots/u-alice`

## Agent API

### Agent 响应结构

`/api/v1/agents*` 返回的 agent 主要字段如下：

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "runtime_id": "codex",
  "runtime_kind": "codex",
  "image": "example/image:latest",
  "box_id": "codex-session-alice",
  "role": "worker",
  "status": "running",
  "created_at": "2026-05-16T08:00:00Z",
  "profile": "api.gpt-5.4",
  "runtime_options": {},
  "agent_profile": {
    "provider": "api",
    "base_url": "https://api.example.com/v1",
    "api_key_set": true,
    "api_key_preview": "sk-1...",
    "model_id": "gpt-5.4",
    "reasoning_effort": "medium",
    "profile_complete": true
  },
  "profile_complete": true,
  "detection_results": []
}
```

说明：

- `agent_profile` 中不会返回真实 `api_key`
- `runtime_options` 会经过 API 侧脱敏处理
- `profile` 是服务端归一化后的选择器，例如 `api.gpt-5.4`
- `detection_results` 用于展示默认 profile 探测结果

### `GET /api/v1/agents`

列出全部 agent。

服务端会先执行 reload，再返回最新状态。

### `POST /api/v1/agents`

创建 agent。

请求体字段：

- `id`
- `name`
- `description`
- `image`
- `runtime_kind`
- `from_template`
- `replace`
- `field_mask`
- `role`
- `status`
- `created_at`
- `profile`
- `runtime_options`
- `agent_profile`

请求体示例：

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "runtime_kind": "codex",
  "profile": "api.gpt-5.4",
  "agent_profile": {
    "provider": "api",
    "base_url": "https://api.example.com/v1",
    "api_key": "sk-xxx",
    "model_id": "gpt-5.4",
    "reasoning_effort": "medium"
  }
}
```

补充说明：

- `name` 必填
- `replace=true` 时会走替换逻辑
- `field_mask` 用于替换时只覆盖指定字段
- `agent_profile.api_key` 只在写入时使用，读取时会被脱敏

### `GET /api/v1/agents/{id}`

获取单个 agent。

不存在时返回 `404`。

### `PATCH /api/v1/agents/{id}`

更新 agent 基本信息。

可更新字段：

- `name`
- `description`
- `image`
- `runtime_options`
- `agent_profile`

请求体示例：

```json
{
  "description": "updated description",
  "runtime_options": {
    "sandbox": "default"
  }
}
```

说明：

- 省略的字段不会修改
- `agent_profile.api_key` 如果传空，服务端会保留原有密钥
- 如果 `agent_profile.env` 发生变化，响应中的 `env_restart_required` 可能为 `true`

### `DELETE /api/v1/agents/{id}`

删除 agent。

成功返回 `204 No Content`。

### `POST /api/v1/agents/{id}/start`

启动 agent，返回更新后的 agent 对象。

### `POST /api/v1/agents/{id}/stop`

停止 agent，返回更新后的 agent 对象。

### `GET /api/v1/agents/{id}/logs`

获取 agent 日志。

查询参数：

- `lines`：默认 `20`
- `follow`：`1/true/yes/on` 表示持续跟随输出

返回类型：`text/plain; charset=utf-8`

说明：

- `follow=false` 时，错误会直接以 HTTP 错误返回
- `follow=true` 时，若流式过程中出错，错误文本会被写入响应体

### `GET /api/v1/agents/{id}/profile`

获取单个 agent 的脱敏 profile。

### `PUT /api/v1/agents/{id}/profile`

整体更新单个 agent 的 profile。

请求体为 `agent_profile` 结构，例如：

```json
{
  "provider": "api",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx",
  "model_id": "gpt-5.4",
  "reasoning_effort": "medium",
  "headers": {
    "x-org": "demo"
  },
  "env": {
    "FOO": "bar"
  }
}
```

说明：

- 与 `PATCH /api/v1/agents/{id}` 不同，这里语义上是“用新的 profile 覆盖当前 profile”
- 若 `api_key` 为空，服务端会保留现有密钥

### `POST /api/v1/agents/{id}/recreate`

按当前配置重建 agent，返回新的 agent 状态。

常见失败：

- `404`：agent 不存在
- `400`：profile 不完整或运行时不允许重建

## Agent Profile 辅助 API

### `POST /api/v1/agent-profiles/models`

根据给定 provider 配置获取可选模型列表。

请求体字段：

- `agent_id`
- `provider`
- `base_url`
- `api_key`
- `headers`

请求体示例：

```json
{
  "provider": "api",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx"
}
```

响应示例：

```json
{
  "provider": "api",
  "models": ["gpt-5.4", "gpt-5.4-mini"]
}
```

说明：

- `provider=codex` 或 `claude_code` 时会通过 CLIProxy 获取模型选项
- `provider=api` 时会调用目标 OpenAI-compatible `/models`
- 若提供了 `agent_id` 且当前请求未显式传 `api_key`，服务端可能复用该 agent 已保存的密钥

### `GET /api/v1/agent-profile-defaults`

获取服务当前默认 agent profile 的脱敏视图。

常用于前端初始化默认 provider / model 展示。

## Hub Template API

### `GET /api/v1/hub/templates`

列出可读 registry 中的全部模板。

响应字段：

- `id`
- `name`
- `description`
- `runtime_kind`
- `image`
- `updated_at`
- `source.name`
- `source.kind`
- `workspace.kind`

### `POST /api/v1/hub/templates`

将现有 agent 的 workspace 发布到 hub。

请求体：

```json
{
  "agent_id": "u-alice",
  "registry": "local"
}
```

说明：

- `agent_id` 必填
- `registry` 省略时使用默认 publish registry
- 发布成功返回 `201 Created`

### `GET /api/v1/hub/templates/{id}`

获取模板详情。

在列表接口的基础上，还会返回：

- `workspace.entries`

`workspace.entries` 字段示例：

```json
{
  "workspace": {
    "kind": "dir",
    "entries": [
      {"path":"SKILL.md","name":"SKILL.md","type":"file","depth":0,"size":128},
      {"path":"assets","name":"assets","type":"dir","depth":0,"size":0}
    ]
  }
}
```

### `GET /api/v1/hub/templates/{id}/workspace/file?path=...`

读取模板 workspace 中的单个文件预览。

查询参数：

- `path`：必填，相对路径

响应字段：

- `path`
- `content`
- `size`
- `truncated`
- `binary`

说明：

- 非 UTF-8 文件会返回 `binary=true`
- 超过 `256 KiB` 的文本内容会被截断，并返回 `truncated=true`
- 不允许绝对路径或 `..` 越界路径

## CLIProxy Auth API

### `GET /api/v1/cliproxy/auth/status?provider=...`

查询 provider 的本地鉴权状态。

`provider` 必填。

响应内容由 CLIProxy 返回，通常包含：

- `provider`
- `authenticated`
- `login_required`
- `message`
- `supports_login`

### `POST /api/v1/cliproxy/auth/login`

触发 provider 登录。

请求体：

```json
{
  "provider": "codex",
  "no_browser": true
}
```

成功返回 provider 当前鉴权状态。

说明：

- 缺少 `provider` 返回 `400`
- 登录失败返回 `502 Bad Gateway`

## Bootstrap Config API

### `GET /api/v1/config/bootstrap`

获取 bootstrap 配置视图。

响应字段：

- `default_manager_template`
- `default_worker_template`
- `runtime_kind`
- `effective_manager_image`
- `supported_runtime_kinds`
- `runtime_default_images`

### `PUT /api/v1/config/bootstrap`

更新 bootstrap 默认模板。

请求体：

```json
{
  "default_manager_template": "builtin/manager",
  "default_worker_template": "local/review-bot"
}
```

说明：

- 两个字段都可选
- 更新后会做 bootstrap 配置校验
- 如果默认模板变化且 agent service 已挂载，会同步更新 gateway runtime

## 本地 IM API

这组接口对应 CSGClaw 本地 IM 数据。

### `GET /api/v1/bootstrap`

获取 IM bootstrap 数据。

响应字段：

- `current_user_id`
- `users`
- `rooms`
- `invite_draft_user_ids`

### `GET /api/v1/events`

订阅本地 IM 事件流。

返回 `text/event-stream`，建立连接后先写入：

```text
: connected
```

随后按 SSE `data:` 帧推送 JSON 事件；心跳为：

```text
: ping
```

当前实际可能出现的事件类型包括：

- `message.created`
- `room.created`
- `room.members_added`
- `upgrade.status_changed`

事件 JSON 结构：

- `type`
- `room_id`
- `room`
- `user`
- `message`
- `sender`
- `upgrade`

### `GET /api/v1/users`

列出本地 IM 用户。

### `POST /api/v1/users`

创建本地 IM 用户。

请求体：

```json
{
  "id": "u-alice",
  "name": "Alice",
  "handle": "alice",
  "role": "worker"
}
```

说明：

- `id` 必填
- `name` 必填
- `handle` 省略时默认等于 `name`
- 对于 `worker/agent` 角色，如果 bot service 与 agent service 已启用，服务端可能转而创建一个 worker bot 及其 backing agent

### `DELETE /api/v1/users/{id}`

删除本地 IM 用户。

常见返回：

- `204`：删除成功
- `404`：用户不存在
- `409`：尝试删除当前用户

### `GET /api/v1/rooms`

列出本地 IM 房间。

### `POST /api/v1/rooms`

创建房间。

请求体：

```json
{
  "title": "Launch",
  "description": "coordination",
  "creator_id": "u-admin",
  "member_ids": ["u-alice", "u-bob"],
  "locale": "en"
}
```

兼容字段：

- 旧请求中的 `participant_ids` 仍可被识别并映射到 `member_ids`

### `DELETE /api/v1/rooms/{id}`

删除房间，成功返回 `204`。

### `GET /api/v1/rooms/{id}/members`

列出房间成员。

### `POST /api/v1/rooms/{id}/members`

向指定房间加人。

请求体：

```json
{
  "inviter_id": "u-admin",
  "user_ids": ["u-bob"],
  "locale": "en"
}
```

说明：

- 路径中的 `{id}` 会作为 `room_id`
- 若 body 中也传了 `room_id`，必须与路径一致

### `POST /api/v1/rooms/invite`

按 room 维度添加成员，语义与 `POST /api/v1/rooms/{id}/members` 基本一致。

请求体：

```json
{
  "room_id": "room-1",
  "inviter_id": "u-admin",
  "user_ids": ["u-bob"],
  "locale": "en"
}
```

### `GET /api/v1/messages?room_id=...`

获取指定房间消息列表。

`room_id` 必填。

### `POST /api/v1/messages`

发送消息。

请求体：

```json
{
  "room_id": "room-1",
  "sender_id": "u-admin",
  "content": "hello @alice",
  "mention_id": "u-alice"
}
```

说明：

- `room_id` 必填
- 成功返回 `201 Created`
- 发送成功后会向 `/api/v1/events` 发布 `message.created`

## Channel API

## `csgclaw` channel

`/api/v1/channels/csgclaw/*` 基本是本地 IM 的镜像接口。

### 用户

- `GET /api/v1/channels/csgclaw/users`
- `POST /api/v1/channels/csgclaw/users`
- `DELETE /api/v1/channels/csgclaw/users/{id}`

说明：

- `GET` / `POST` 复用本地 IM 用户逻辑
- `DELETE` 走 channel 专用删除逻辑，但语义仍是删除本地用户

### 房间

- `GET /api/v1/channels/csgclaw/rooms`
- `POST /api/v1/channels/csgclaw/rooms`
- `DELETE /api/v1/channels/csgclaw/rooms/{id}`
- `GET /api/v1/channels/csgclaw/rooms/{id}/members`
- `POST /api/v1/channels/csgclaw/rooms/{id}/members`

### 消息

- `GET /api/v1/channels/csgclaw/messages?room_id=...`
- `POST /api/v1/channels/csgclaw/messages`

## `feishu` channel

### 配置

#### `GET /api/v1/channels/feishu/config`

获取飞书 channel 配置视图。

可选查询参数：

- `bot_id`

响应示例：

```json
{
  "bot_id": "u-manager",
  "configured": true,
  "app_id": "cli_xxx",
  "app_secret": "present",
  "admin_open_id": "ou_xxx"
}
```

说明：

- `app_secret` 返回的是状态值，不是真实 secret

#### `PUT /api/v1/channels/feishu/config`

更新飞书 channel 配置。

请求体：

```json
{
  "bot_id": "u-manager",
  "app_id": "cli_xxx",
  "app_secret": "secret",
  "admin_open_id": "ou_xxx",
  "reload": true
}
```

说明：

- `app_id` 和 `app_secret` 必填
- `bot_id` 可从 query 或 body 中传入
- `reload` 省略时默认 `true`

#### `POST /api/v1/channels/feishu/config`

重新加载飞书配置。

响应示例：

```json
{
  "status": "reloaded",
  "feishu_bots": ["u-manager"]
}
```

### Bot 事件

#### `GET /api/v1/channels/feishu/bots/{id}/events`

订阅指定 bot 在飞书中的被提及消息事件。

特点：

- 需要 Bearer Token
- 返回 `text/event-stream`
- 只转发“消息里 mention 到该 bot open_id”的事件
- 建立连接后先输出 `: connected`

### 用户

- `GET /api/v1/channels/feishu/users`
- `POST /api/v1/channels/feishu/users`
- `DELETE /api/v1/channels/feishu/users/{id}`

`POST` 请求体示例：

```json
{
  "id": "ou_xxx",
  "name": "Alice",
  "handle": "alice",
  "role": "member",
  "avatar": "AL"
}
```

### 房间

- `GET /api/v1/channels/feishu/rooms`
- `POST /api/v1/channels/feishu/rooms`
- `DELETE /api/v1/channels/feishu/rooms/{id}`
- `GET /api/v1/channels/feishu/rooms/{id}/members`
- `POST /api/v1/channels/feishu/rooms/{id}/members`

创建房间和加人时，请求体与本地 IM 基本一致，仍使用：

- `title`
- `description`
- `creator_id`
- `member_ids`
- `locale`

加人接口请求体：

```json
{
  "inviter_id": "u-manager",
  "user_ids": ["ou_member"],
  "locale": "zh-CN"
}
```

### 消息

- `GET /api/v1/channels/feishu/messages?room_id=...`
- `POST /api/v1/channels/feishu/messages`

发送消息请求体：

```json
{
  "room_id": "oc_xxx",
  "sender_id": "u-manager",
  "content": "hello",
  "mention_id": "u-worker"
}
```

## Bot 兼容 API

这组接口位于 `/api/bots/{id}`，用于兼容旧的 PicoClaw Bot 接入方式。

### `GET /api/bots/{id}/events`

订阅 bot 事件流。

特点：

- 需要 Bearer Token
- 返回 `text/event-stream`
- 建立连接后先输出 `: connected`
- 心跳注释为 `: heartbeat`
- 事件名为 `message`
- 若客户端带 `Last-Event-ID`，服务端会按 replay 规则尝试补发最近消息

单条事件示例：

```text
id: msg-1
event: message
data: {"message_id":"msg-1","room_id":"room-1","sender_id":"u-admin","text":"hello"}
```

### `POST /api/bots/{id}/messages/send`

向 bot 兼容通道发送消息。

请求体示例：

```json
{
  "room_id": "room-1",
  "text": "hello"
}
```

具体响应由兼容桥实现决定。

### `GET /api/bots/{id}/llm/models`

### `GET /api/bots/{id}/llm/v1/models`

转发模型列表请求到 LLM bridge。

说明：

- 需要 Bearer Token
- 返回内容类型和响应体由上游 bridge 决定

### `POST /api/bots/{id}/llm/chat/completions`

### `POST /api/bots/{id}/llm/v1/chat/completions`

转发聊天补全请求到 LLM bridge。

说明：

- 需要 Bearer Token
- 请求体会原样读取并转发
- 单次读取上限为 `10 MiB`
- 失败时可能返回普通文本错误，也可能返回：

```json
{
  "error": {
    "code": "unauthorized",
    "message": "upstream auth failed",
    "provider": "openai"
  }
}
```

## 兼容性说明

- `CreateRoomRequest.participant_ids` 仍兼容旧字段，会映射到 `member_ids`
- `Message.mentions` 兼容旧格式：
  - 新格式：`[{ "id": "u-alice", "name": "Alice" }]`
  - 旧格式：`["u-alice"]`
- 本地 `csgclaw` channel 路由本质上是 `/api/v1/users|rooms|messages` 的镜像入口

## 当前未暴露的旧接口

以下旧文档中常见路径，当前路由里已不存在，不应再作为对外 API 使用：

- `/api/v1/notify/{agent_id}`
- 任何未在 `internal/api/router.go` 中注册的旧路径
