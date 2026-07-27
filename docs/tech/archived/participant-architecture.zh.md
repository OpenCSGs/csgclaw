# Participant 身份架构与 API 修改计划

## 快速 Review 要点

- 核心变化是把 `Participant`、`Agent`、`ChannelUser` 和产品文案里的 `Bot` 拆开：Participant 是 room/message/member/mention 使用的协作身份；Agent 是 runtime 执行实体；ChannelUser 是 channel 内部 identity/profile；Bot 不再是后端 API 和存储模型。
- UI 创建一个能出现在 CSGClaw 自带 IM 里的 Agent 时，应调用 `POST /api/v1/channels/csgclaw/participants`，使用 `type=agent` 和 `agent_binding.mode=create`，由服务端一次性创建 Agent、ChannelUser 和 Participant。
- 新建 agent-backed participant 的 Agent ID 生成关系保持旧约定：`agent_id = u-{participant_id}`。Participant ID 应来自显式 `id` 或稳定 key，不能从后续可修改的 `name` 派生。
- 跨 channel 复用不再依赖 ID 相等。同一个 Agent 可以有多个 participant，例如 `csgclaw:qa -> agent:u-qa` 和 `feishu:test -> agent:u-qa`。
- Mention 走 participant 身份层。Feishu 真人 mention 使用 `channel_user_ref` 加 `channel_app_ref` / `channel_user_kind` 显式解析，不要求真人拥有 bot app/config。
- Notification 是 `type=notification` 的 participant，表示 webhook、系统事件或 pull relay 这类通知来源；默认不绑定 Agent，也不暴露 LLM bridge。
- Message 新 API 使用 `mentions` / `mention_ids` 数组；room membership 从 `user_ids` 迁移到 `participant_ids` 或结构化 `ParticipantRef`。
- 删除 participant 默认只删除 channel 身份绑定，不删除底层 Agent；只有显式 `delete_agent=if_unreferenced` 这类语义才允许清理未被引用的 Agent。
- 这次是前后端、runtime bridge、CLI 和内置模板同步更新的 breaking API 调整：API 兼容性不是迁移重点，旧 bot 路由、公开 `/users` 路由不保留兼容别名。
- 旧 `bots.json` 等磁盘数据仍应可迁移；只要发现 runtime image/template contract 过旧，就在 UI 上提醒 recreate；当前 recreate 只承诺保留用户安装的 skills。
- Matrix 对齐只覆盖本次涉及的 identity、membership、message、mention 和 thread 形状，不实现完整 Matrix homeserver 或 Client-Server API。

## 迁移优先事项

这次前后端一起发布，API breaking 不作为迁移风险；迁移重点是本地磁盘状态和旧 runtime image。

- **`bots.json`**：旧 bot 记录仍要能读取，并迁移成 participant 记录。普通 bot 迁移为 `type=agent` participant，保留原 `agent_id` / `channel_user_ref`；notification bot 迁移为 `type=notification` participant。
- **IM state**：`im/state.json` 和 `im/sessions/*.jsonl` 里的旧 `users`、room `members`、message `sender_id`、`mentions`、thread context 等身份引用，需要从旧 user/bot ID 映射到 participant ID。
- **Feishu config**：`channels/feishu.toml` 里按旧 `bot_id` 保存的 app/config，要迁移到 participant/channel app 语义，避免飞书发送和 mention 解析丢配置。
- **Team state**：`teams/*` 下的 `lead_bot_id`、`member_bot_ids`、`bot_id`、`actor_id`、`created_by`、`assigned_to`、`requested_by`、`approver_id` 等字段，要迁移为 participant ID。
- **Agents state**：`agents/state.json` 的 Agent ID 不需要改写；新 participant 记录继续通过 `agent_id` 指向原 Agent。
- **旧镜像提醒**：只要发现 runtime image/template contract 过旧，就在 UI 上提醒用户 recreate。
- **recreate 保留 skills**：当前 recreate 只承诺保留用户安装的 skills；不在这次计划里扩展为保留 workspace/project 状态。

## 背景

CSGClaw 目前的 `bot`、`agent` 和 channel `user` 概念在最早的本地多 Agent 协作里可以工作，但面对多渠道、多真人、多机器人协作时边界不够清楚。

最直接的问题是 mention 身份。Agent 需要在 CSGClaw 自带 IM、飞书以及未来其他 IM 里 @ 真人。当前 bot 模型默认消息发送者和 mention 对象都是 bot 类身份。在飞书里，这会导致 mention 通过已配置 bot app 身份解析，Agent 无法自然地 @ 一个真人 open_id。

还有一个命名和归属问题。UI 里很多操作叫 Agent 管理，但部分客户端实际调用 channel bot API。与此同时，底层 runtime agent 本来就可以跨 channel 复用。例如，Feishu bot 可以建模为“飞书 channel 身份 + 一个原本从 CSGClaw channel 创建的底层 Agent”。现在这种复用主要依赖 `u-manager` 这类相同 ID 约定。如果 CSGClaw 里的 agent 是 `u-qa`，但飞书侧 participant 叫 `test`，这个关系就无法清晰表达。

目标设计是拆开这些概念：

- `Participant` 是协作身份，被 room、message、member、mention 引用。
- `Agent` 是 runtime 执行实体，拥有 model、profile、生命周期、日志和 sandbox 状态。
- `Bot` 从 API 和存储模型中移除。只有当产品文案明确需要面向用户说“Bot”时，UI 才可以保留这个词。

## 目标

- Agent 可以在 CSGClaw IM、飞书和未来 IM 中 @ 真人。
- Room 和 Message 可以统一包含 human、agent-backed participant、notification participant。
- 一个底层 Agent 可以被多个 channel participant 复用，不依赖 ID 相等。
- `GET /api/v1/agents` 可以展示所有 runtime agent，以及这些 agent 在任意支持 IM 中对应的 participant，包括飞书。
- UI 可以同时支持创建新的 agent-backed participant，以及把已有 agent 添加到另一个 channel，而不要求用户先理解内部模型。
- 前后端、runtime bridge 和内置模板同步更新，现有 bot 路由不保留旧别名，直接删除。
- 新模型在这次涉及 room、member、message、mention、thread 的范围内尽量贴近 Matrix 语义。

## 非目标

- 本次不合并同一个真人在不同 channel 的身份。如果后续产品需要跨 channel 审计或权限，可以再增加 `Identity` 层。
- 不要求所有 participant 都有 agent。human 和 notification participant 默认不需要 runtime 执行能力。
- 本次不实现完整 Matrix homeserver 或完整 Client-Server API。只在这次会碰到的身份、成员、消息、mention 和 thread 形状上对齐 Matrix。

## 目标模型

### Agent

Agent 是 CSGClaw 全局 runtime 实体，不属于任何单一 channel。

```text
Agent
  id
  name
  role
  runtime_id
  runtime_kind
  image
  runtime_options
  status
  agent_profile
  created_at
```

规则：

- Agent ID 全局唯一。
- 新建 agent-backed participant 时，如果请求未显式指定 Agent ID，服务端按 `u-{participant_id}` 生成 Agent ID。这个关系保持旧 worker/bot ID 的习惯，例如 participant `qa` 对应 agent `u-qa`。
- Bootstrap manager 是保留例外：默认 CSGClaw participant ID 为 `manager`，Agent ID 仍为 `u-manager`。
- 如果调用方显式指定 Agent ID，仍必须满足全局唯一，并且不要求和 participant ID 相同；跨 channel 复用时通常显式传已有 `agent_id`。
- Agent 生命周期操作继续放在 `/api/v1/agents`。
- Agent profile、model、runtime、日志、start、stop、restart、recreate 仍由 agent service 管理。

### Participant

Participant 是某个 channel 内可见的协作身份。

```text
Participant
  id                  # 在所在 channel 内稳定
  channel             # csgclaw | feishu | matrix | ...
  type                # human | agent | notification
  name
  avatar
  channel_user_ref    # csgclaw user id, feishu open_id, matrix user_id, ...
  channel_user_kind   # local_user_id | open_id | matrix_user_id | ...
  channel_app_ref     # 可选，用于 Feishu app_id 这类 bot app/config 身份
  agent_id            # 可选 FK -> Agent.id，仅 type=agent 时有意义
  lifecycle_status    # provisioning | active | disabled | failed
  presence            # 可选，channel presence 或 room member view
  mentionable
  metadata
  created_at
  updated_at
```

规则：

- Participant 的规范 key 是 `(channel, id)`。
- 只要 participant 需要在 channel 里发送、接收或被 mention，`channel_user_ref` 就是必填。
- 活跃 participant 应满足该 channel 的 channel-user identity 唯一。对简单 channel 是 `(channel, channel_user_ref)`；对 Feishu 这类 app-scoped identity，需要把 `channel_app_ref` 和 `channel_user_kind` 纳入唯一键。
- `lifecycle_status` 描述 participant 记录自身是否可用；`presence` 描述 channel 或 room 里的在线/离线视图；runtime 是否 running 应来自绑定的 Agent，不应写入 participant 的持久状态。
- `mentionable=false` 表示该 participant 可以存在于列表或历史中，但不应被新消息 mention。
- `type=human` 不要求也不应该要求 `agent_id`。
- `type=notification` 不要求也不应该要求 `agent_id`。
- `type=agent` 可以绑定已有 agent、创建新 agent，或者先注册 participant 后续再绑定 runtime。
- 一个 Agent 可以跨 channel 拥有多个 participant：

```text
csgclaw:qa     -> agent:u-qa
feishu:test    -> agent:u-qa
matrix:qa-bot  -> agent:u-qa
```

### Participant ID 生成规则

Participant ID 是用户和 CLI 经常看到的 channel 身份 ID，应优先可读且稳定，而不是默认使用裸 UUID。UUID 可以作为内部随机源或兜底值，但直接暴露给用户会降低可读性，也不利于 room membership、mention 和 CLI 操作。

Participant ID 不能从 `name` 生成。`name` 是显示名，后续可能支持修改；ID 一旦进入 room member、message、mention、agent binding 和 CLI，就必须稳定。业界更常见的做法是“稳定 slug + 短随机冲突后缀”，例如 Kubernetes object name 或很多 SaaS 的 workspace slug。只有完全不面向用户操作的内部对象，才更适合直接暴露 `usr_...`、`agt_...` 这类带类型前缀的 opaque ID。

推荐生成算法：

1. 如果请求显式传入 `id`，先 normalize 并校验唯一性。
2. 如果没有显式 `id`，只能从稳定来源生成 slug，例如创建请求里的独立 `slug` / `handle` 字段、内置模板 key、角色 key、外部 channel 的不可变 handle，或迁移时的旧 bot/user ID。不要使用可修改的显示名 `name`。
3. Slug 规则：小写；去掉首尾空白；把连续非 `[a-z0-9]` 字符替换成 `-`；折叠连续 `-`；去掉首尾 `-`；建议长度限制为 3 到 48 个字符。
4. 如果 slug 为空，按类型生成可读前缀加短随机后缀，例如 `agent-8f3k2m`、`human-8f3k2m`、`notification-8f3k2m`。
5. 如果 slug 已存在，在 slug 后追加短随机后缀，例如 `qa-8f3k2m`。短随机后缀可以来自 UUID/ULID/nanoid 的 base32/base36 截断值。
6. 服务端返回最终 participant ID；同一个 `request_id` 或 `client_transaction_id` 重试时必须返回同一个 ID。

Agent-backed participant 的默认 Agent ID 生成规则保持：

```text
agent_id = "u-" + participant_id
```

因此新建 CSGClaw IM Agent 时，participant `qa` 默认生成 agent `u-qa`，同时 CSGClaw channel user ref 也可以继续使用 `u-qa`，保持旧 runtime、workspace 和 mention 习惯。

### Channel User / Channel Identity

引入 Participant 之后，现有对外 `User` 模型不应该继续作为顶层产品 API 保留。对外创建、查询、更新和删除“人”的入口应统一替换成 participant API。

但底层仍然需要一个类似 user 的记录，只是它的语义变成 channel-scoped identity/profile，由 channel adapter 拥有，不再是主要协作身份。

```text
ChannelUser
  channel             # csgclaw | feishu | matrix | ...
  ref                 # CSGClaw local user id, Feishu open_id/union_id, Matrix user_id
  kind                # local_user_id | open_id | matrix_user_id | ...
  app_ref             # 可选，Feishu app_id/tenant scoped identity 时使用
  display_name
  handle
  avatar_url
  presence
  raw_profile
  updated_at
```

规则：

- `Participant.channel_user_ref` 指向 channel user，或该 channel 原生的等价身份。
- 对内置 CSGClaw channel，这个内部记录替代当前 `User` 存储形状，用来承载 profile、avatar、handle 和 presence。
- 对飞书，这个记录由 adapter 管理。`kind=open_id` 时 `ref` 是 app-scoped open_id，必须和 `app_ref` 一起理解；如果后续使用 `union_id` 或 `user_id`，也必须在 `kind` 中显式标出。
- 对 Matrix，这个记录可以自然映射到 Matrix user ID，以及 `displayname`、`avatar_url`、membership state 等 member profile 字段。
- 公共客户端应通过 participant 创建、列出、更新和删除真人，不再调用独立 `/users` API。

### Feishu 身份 Scope

Feishu 的 user identity 不能只看一个裸 `open_id` 字符串。不同 app、tenant 或 identity type 下，同一个真人可能有不同 ID，同一个 ID 字符串也不能跨 scope 复用。

规则：

- `channel_app_ref` 表示这个 participant 对应的 Feishu app/config，例如 `cli_xxx`。
- `channel_user_kind=open_id` 时，`channel_user_ref` 是该 app scope 下的 open_id；唯一键应至少包含 `(channel, channel_app_ref, channel_user_kind, channel_user_ref)`。
- `type=agent` participant 需要 `channel_app_ref` 来确定发送消息时使用哪个 Feishu app 凭证。
- `type=human` participant 用 `channel_user_ref` 作为 mention 目标，不要求这个真人有 bot app/config。
- 如果 adapter 从 Feishu event 里只拿到 `user_id`、`union_id` 或其他身份，应先按 `channel_user_kind` 记录原始身份，再在需要发送或 mention 时做显式解析，不能隐式当作 bot ID。

### Notification Participant

Notification 也是一种 channel participant，但它默认不是 runtime agent。

```text
Participant(type=notification)
  channel_user_ref       # notification sender identity 或本地 webhook identity
  channel_app_ref        # 可选，外部 channel 发送所需 app/config
  metadata.notification  # webhook、remote_pull、subscription、delivery config
```

规则：

- Notification participant 可以作为 room/message sender，用于展示系统通知、第三方 webhook 或 pull relay 的来源。
- Notification participant 默认没有 `agent_id`，也不暴露 LLM bridge。
- Notification webhook/pull 配置挂在 participant metadata 或专门的 notification profile 上，不再挂在 bot API 形状上。
- 原 `POST /api/v1/channels/csgclaw/bots/{id}/notifications` 应替换为 participant-scoped notification endpoint：

```text
POST /api/v1/channels/{channel}/participants/{id}/notifications
```

### Room、Message、Mention

Room 和 Message 应引用 participant，而不是引用 agent。

```text
Room
  channel
  members: ParticipantRef[]

Message
  event_id
  room_id
  sender: ParticipantRef
  mentions: ParticipantRef[]
  content
  relates_to

ParticipantRef
  channel
  id
```

新 API 应避免 bot 形状命名。`sender_id`、`member_ids`、`user_ids` 这类字段只有在含义明确是 participant 或 Matrix user identity 时才保留，不能继续表示 legacy bot identity。新消息 API 应优先使用 `mentions: ParticipantRef[]` 或 `mention_ids: []`，而不是单个 `mention_id`。存在歧义时，优先使用 `participant_id`、`participant_ids` 或结构化 `ParticipantRef`。

### 未来聊天记录同步兼容性

聊天记录同步不是这次实施计划的一部分，它只是 participant 模型必须兼容的后续方向。

如果用户在飞书里聊天，CSGClaw 后续同步这个 room，导入消息应能走和本地消息相同的 participant 解析路径。因此 participant 模型不能假设所有消息都是本地产生，也不能假设所有 sender 都是 agent-backed participant。

未来 sync 工作可以增加 `MessageEvent` 或相邻 sync 记录，字段包括：

- `external_room_ref`，例如 Feishu `chat_id` 或 Matrix room ID；
- `external_event_id`，例如 Feishu `message_id` 或 Matrix event ID；
- source IM 提供的 `origin_server_ts`；
- 本地摄入时间 `received_at`；
- 用于可恢复同步的 `sync_batch` 或 cursor metadata；
- 脱敏或限长后的 `raw_event`，用于排查问题。

这次计划不实现 sync storage、sync API 或 backfill job。这里只保证 sender、mention、room 和 event identity 不阻碍后续同步。

### 可选的未来 Identity 层

如果未来需要知道同一个真人同时出现在 CSGClaw local user、Feishu open_id 和 Matrix user_id 中，可以增加身份聚合层：

```text
Identity 1 -> N Participant(type=human)
```

这个层不是解决 Agent @ 真人或跨 channel 复用 Agent 的前置条件。

## Matrix 对齐

后续 CSGClaw IM 会往 Matrix 协议方向实现，因此这次 participant 调整应该在涉及范围内尽量贴近 Matrix。目标不是现在实现完整 Matrix Client-Server API，而是避免产生第二套不可兼容的 IM 模型。

对齐点如下：

- Matrix user ID 映射到 `Participant.channel_user_ref`，`channel_user_kind=matrix_user_id`，例如 `@qa:example.org`。
- Matrix room ID 映射到 channel room reference，例如 `!roomid:example.org`；room alias 可以放在 channel metadata。
- Room membership 应能表示成 `m.room.member` state：`membership`、`displayname`、`avatar_url` 属于 room membership 或 participant view，不属于 runtime agent。
- 文本消息应能表示成 `m.room.message` event，content 至少包含 `msgtype=m.text`、`body`，可选 `format` / `formatted_body`。
- Mention 应能表示成 Matrix `m.mentions`，尤其是用户 mention 的 `m.mentions.user_ids` 和 room mention 的 `m.mentions.room`。
- 已有 thread metadata 应继续保持 Matrix 形状：`m.relates_to.rel_type=m.thread` 加 root event ID。
- 后续 CSGClaw sync API 应保持与 Matrix `/sync` 类似的高层形状：客户端拿到 joined room timelines，以及类似 `next_batch` / `since` 的可恢复 batch token。

这样 CSGClaw 自有 IM 后续迁到 Matrix 会更顺，Feishu 和其他 channel adapter 仍然可以保留各自原生传输细节。

## API 计划

### Participant API

在 channel 命名空间下新增 participant API。这些 API 替换旧的 channel bot CRUD 路由。

```text
GET    /api/v1/channels/{channel}/participants
POST   /api/v1/channels/{channel}/participants
GET    /api/v1/channels/{channel}/participants/{id}
PATCH  /api/v1/channels/{channel}/participants/{id}
DELETE /api/v1/channels/{channel}/participants/{id}
```

以下路由直接删除：

```text
GET    /api/v1/channels/{channel}/bots
POST   /api/v1/channels/{channel}/bots
GET    /api/v1/channels/{channel}/bots/{id}
PATCH  /api/v1/channels/{channel}/bots/{id}
DELETE /api/v1/channels/{channel}/bots/{id}
GET    /api/v1/channels/feishu/bots/{id}/events
POST   /api/v1/channels/csgclaw/bots/{id}/notifications
GET    /api/bots/{id}/events
POST   /api/bots/{id}/messages/send
GET    /api/bots/{id}/llm/models
GET    /api/bots/{id}/llm/v1/models
POST   /api/bots/{id}/llm/chat/completions
POST   /api/bots/{id}/llm/v1/chat/completions
POST   /api/bots/{id}/llm/responses
POST   /api/bots/{id}/llm/v1/responses
```

当前公开 user 路由也应从产品 API 面删除：

```text
GET    /api/v1/users
POST   /api/v1/users
DELETE /api/v1/users/{id}
GET    /api/v1/channels/csgclaw/users
POST   /api/v1/channels/csgclaw/users
DELETE /api/v1/channels/csgclaw/users/{id}
```

使用 participant API 替代：

```text
GET  /api/v1/channels/{channel}/participants?type=human
POST /api/v1/channels/{channel}/participants
```

列表查询参数：

- `type=human|agent|notification`
- `agent_id=<agent_id>`
- `include_agent=true`
- `include_channel_user=true`

创建一个同时新建 Agent 的 agent-backed participant：

```json
{
  "id": "qa",
  "type": "agent",
  "name": "qa",
  "channel_user": {
    "ref": "u-qa",
    "kind": "local_user_id"
  },
  "agent_binding": {
    "mode": "create",
    "agent": {
      "id": "u-qa",
      "name": "qa",
      "role": "worker",
      "runtime_kind": "codex",
      "agent_profile": {
        "provider": "api",
        "model_id": "gpt-5.4"
      }
    }
  }
}
```

这个 endpoint 是旧 UI “create bot” 行为的替代：旧行为会同时创建 agent 和 channel user。新方案必须由服务端提供一个单次 provisioning 操作，不能让 UI 先调用 `POST /api/v1/agents` 再单独创建 participant。

对于内置 CSGClaw channel，`agent_binding.mode=create` 必须一次性创建：

```text
1. Agent
2. Channel user / Matrix-shaped member identity
3. Participant(type=agent, agent_id=<created agent>)
```

只有三个资源都有效时才算提交成功。如果 agent 已创建但 user 或 participant 创建失败，服务端必须回滚已创建 agent，或者把该操作标记为失败且可通过 idempotency key 安全重试。

对于外部 channel，不存在真正的分布式事务。UI 仍然只调用一次 API，但服务端必须通过幂等和补偿保证一致性：

- 接受可选 `request_id` 或 `client_transaction_id`；
- 同一个 key 的重复请求返回同一个最终资源；
- 记录已完成的部分步骤；
- 在 participant 暴露为 active 前，重试或补偿失败的 channel-user provisioning。

创建一个复用已有 Agent 的 channel participant：

```json
{
  "id": "test",
  "type": "agent",
  "name": "QA",
  "channel_user": {
    "ref": "ou_xxx",
    "kind": "open_id"
  },
  "channel_app_ref": "cli_xxx",
  "agent_binding": {
    "mode": "reuse",
    "agent_id": "u-qa"
  }
}
```

创建一个真人 participant：

```json
{
  "id": "alice",
  "type": "human",
  "name": "Alice",
  "channel_user": {
    "ref": "ou_alice",
    "kind": "open_id"
  }
}
```

支持的 `agent_binding.mode`：

- `create`：创建新的 Agent 并绑定到这个 participant。
- `reuse`：绑定到已有 Agent。
- `none`：只创建 participant，不绑定 runtime。适用于 human、notification 和草稿状态的 agent participant。

校验规则：

- `type=human` 拒绝 `agent_binding.mode=create`。
- `type=notification` 拒绝 `agent_binding.mode=create`，除非未来 notification 明确需要 runtime。
- `type=agent` 且 `mode=reuse` 时必须提供 `agent_id`。
- `type=agent` 且 `mode=create` 时必须提供足够创建合法 worker 或 manager 的 Agent 字段。
- Participant ID 和 Agent ID 不要求相同。

删除规则：

- `DELETE /api/v1/channels/{channel}/participants/{id}` 默认只删除 participant 以及它的 channel 绑定，不删除底层 Agent。
- 如果 `type=agent` participant 绑定了 Agent，删除 participant 只解除该 channel identity 与 Agent 的关联；同一个 Agent 的其他 participant 不受影响。
- 如果调用方希望同时清理不再被引用的 Agent，应使用显式参数，例如 `delete_agent=if_unreferenced`。当仍有其他 participant 引用该 Agent 时，服务端必须拒绝删除 Agent。
- Channel user / channel identity 只有在由 CSGClaw 管理且没有其他 active participant 引用时才可以被清理；外部 channel 通常只删除本地映射或标记 inactive，不删除远端真实用户。
- Notification participant 删除时应同时移除本地 notification profile、webhook token 和 remote_pull subscription metadata。

### Agent API

保留 `/api/v1/agents` 作为较底层的 runtime agent API，用于编辑 model/profile、start、stop、restart、recreate、delete、查看日志，以及支持内部 provisioning 流程。

CSGClaw 自己的产品 UI 如果创建结果预期会出现在 CSGClaw IM 中，不应把 `POST /api/v1/agents` 作为主“创建 Agent”入口。它应和飞书等第三方 IM 使用同一套 participant provisioning API，只是 `channel=csgclaw`。

扩展 list/detail 响应，展示 participant 绑定关系。

```text
GET /api/v1/agents?include_participants=true
GET /api/v1/agents/{id}?include_participants=true
```

响应片段示例：

```json
{
  "id": "u-qa",
  "name": "qa",
  "role": "worker",
  "runtime_kind": "codex",
  "status": "running",
  "participants": [
    {
      "id": "qa",
      "channel": "csgclaw",
      "type": "agent",
      "channel_user_ref": "u-qa"
    },
    {
      "id": "test",
      "channel": "feishu",
      "type": "agent",
      "channel_user_ref": "ou_xxx"
    }
  ]
}
```

这满足“agent list 要包含任意 IM 中的 agent，包括当前支持的第三方 IM 飞书 agent”的需求。

### Runtime Bridge API 替换

删除 `/api/bots/*` 后，需要把每个旧 bridge surface 替换成明确的 participant scoped 或 agent scoped 路由。

Participant event stream 属于 channel 身份：

```text
GET /api/v1/channels/{channel}/participants/{id}/events
```

替换：

```text
GET /api/v1/channels/feishu/bots/{id}/events
GET /api/bots/{id}/events
```

Participant 作为发送者发消息也属于 channel 身份：

```text
POST /api/v1/channels/{channel}/participants/{id}/messages
```

发送者来自 path participant。body 包含 `room_id`、`content`、可选 `mentions` 和可选 thread relation 字段。替换：

```text
POST /api/bots/{id}/messages/send
```

Notification 投递也属于 channel 身份：

```text
POST /api/v1/channels/{channel}/participants/{id}/notifications
```

替换：

```text
POST /api/v1/channels/csgclaw/bots/{id}/notifications
```

LLM bridge 属于 runtime agent，不属于 channel 身份：

```text
GET  /api/v1/agents/{agent_id}/llm/models
POST /api/v1/agents/{agent_id}/llm/chat/completions
POST /api/v1/agents/{agent_id}/llm/responses
```

替换：

```text
GET  /api/bots/{id}/llm/models
GET  /api/bots/{id}/llm/v1/models
POST /api/bots/{id}/llm/chat/completions
POST /api/bots/{id}/llm/v1/chat/completions
POST /api/bots/{id}/llm/responses
POST /api/bots/{id}/llm/v1/responses
```

如果 runtime 只知道自己的 channel participant ID，需要先解析 participant，再用 `agent_id` 调 LLM API。这样消息身份和 model/runtime 身份保持分离。

### UI API 替换

当前 UI 问题是“创建 Agent”的流程调用了将被删除的 channel bot API。替代方案不是让 UI 串联多个 API。CSGClaw 自己的 UI 也要和飞书及未来第三方 IM 使用同一个模型：在目标 channel 创建 participant，并绑定或创建底层 agent。

当 CSGClaw UI 行为是“创建一个可以在 CSGClaw IM 里聊天的 Agent”时，调用：

```text
POST /api/v1/channels/csgclaw/participants
```

使用 `type=agent` 和 `agent_binding.mode=create` 一次性创建 runtime agent 和 CSGClaw participant。这就是之前 bot API “同时创建 agent 和 user”的直接替代。

当 UI 要把已有 agent 添加到另一个 channel 时，仍使用目标 channel 的同一个 endpoint，并传 `agent_binding.mode=reuse`。

这个流程里 UI 不能先调用 `POST /api/v1/agents`，再单独创建 user 或 participant。拆成多次调用会产生半失败：agent 创建成功但没有 channel identity，或者 channel identity 存在但没有有效 runtime binding。

推荐 UI 到 API 映射：

```text
CSGClaw Agent UI
  创建 CSGClaw IM Agent     -> POST /api/v1/channels/csgclaw/participants
                                type=agent, agent_binding.mode=create
  编辑 runtime/model/profile -> PATCH/PUT /api/v1/agents/{id}...
  start/stop/logs           -> /api/v1/agents/{id}/...

Channel 或 Room 页面
  在当前 channel 创建新 Agent identity -> POST /api/v1/channels/{channel}/participants
                                type=agent, agent_binding.mode=create
  从已有 Agent 添加到 channel identity -> POST /api/v1/channels/{channel}/participants
                                type=agent, agent_binding.mode=reuse
  添加真人                  -> POST /api/v1/channels/{channel}/participants
                                type=human, agent_binding.mode=none
```

### Message 和 Mention API

对 participant-scoped 发送 API，发送者来自 path participant，body 中不再传 `sender_id`。Mention 使用数组，支持一次消息 mention 多个 participant。

```json
{
  "room_id": "oc_xxx",
  "mentions": [
    {
      "id": "alice"
    }
  ],
  "content": "please take a look"
}
```

Channel adapter 解析链路：

```text
path id -> Participant(channel=feishu, id=test)
        -> sender 所需的 channel_user_ref/channel_app_ref

mentions[].id -> Participant(channel=feishu, id=alice)
              -> channel_user_ref=open_id
```

在 CSGClaw IM 中，adapter 渲染本地 mention。在飞书中，adapter 渲染 `<at user_id="open_id">name</at>`。未来 IM 使用各自 channel 的 mention 语法。

如果仍保留 channel-level message endpoint，`sender_id` 必须明确解释为该 channel 下的 participant ID，不能再解释为 bot ID。单个 `mention_id` 不进入新 API，只能作为旧请求结构在调用方同步替换前的临时实现细节。

Room member API 也应从 user 形状迁移到 participant 形状。例如，`user_ids` 应改为 `participant_ids` 或 `participants: ParticipantRef[]`。Matrix 协议边界的 adapter 仍可发送或接收 Matrix 原生 `user_id`，但进入 CSGClaw 领域模型前，应先解析成 participant，再进入 room membership 或 mention 逻辑。

## UI 计划

UI 不应该把 participant、agent、channel user 这些内部模型词作为用户第一层选择。

使用基于意图的入口：

```text
添加 Bot 到当前 Channel

  创建全新的 Bot
    - 创建 Participant(type=agent)
    - 创建并绑定新的 Agent
    - 配置 runtime、model、template

  从已有 Bot 添加
    - 列出当前 channel 中还没有出现的 agent
    - 创建 Participant(type=agent)
    - 绑定到选中的 Agent
    - 确认 channel 相关身份设置
```

真人 participant 使用 channel 语义：

```text
添加真人
  - 选择或输入 channel user identity
  - 创建 Participant(type=human)
  - 让真人可被 mention，并可选加入 room
```

UI 可以继续使用 Bot、Person 这类产品友好的名称。后端保持 participant 和 agent 的分层清晰。

## CLI 变化

CLI 的规范资源名应跟随后端模型，使用 `participant` 作为协作身份入口，并提供更短的 `pt` 子命令别名。`participant` 用于文档、脚本和长期稳定引用；`pt` 用于交互式日常操作。`bot` 可以作为面向使用者的轻量别名保留给 `type=agent` 场景，但输出 JSON、API payload 和错误信息应使用 participant 语义，避免继续暴露 Bot 存储模型。

推荐命令形状：

```text
csgclaw participant list --channel csgclaw --type agent
csgclaw participant create --channel csgclaw --type agent --id qa --name QA --bind create
csgclaw participant create --channel feishu --type agent --id test --bind reuse --agent-id u-qa --channel-user-ref ou_xxx --channel-user-kind open_id --channel-app-ref cli_xxx
csgclaw participant create --channel feishu --type human --id alice --name Alice --channel-user-ref ou_alice --channel-user-kind open_id --channel-app-ref cli_xxx
csgclaw participant delete --channel feishu test
csgclaw participant delete --channel feishu test --delete-agent if-unreferenced
csgclaw pt list --channel csgclaw --type agent
csgclaw pt create --channel csgclaw --type agent --id qa --name QA --bind create
```

CLI 字段改名应和 API 一致：

- `pt` 是 `participant` 的等价短别名，所有 `participant` 子命令、flag、输出和错误语义都必须一致。
- `bot list/create/delete` 不再是规范命令；如果保留，应只是 `participant --type agent` / `pt --type agent` 的产品别名。
- `agent create` 只负责创建 runtime-only Agent，不应作为“创建可聊天 CSGClaw IM Agent”的主入口。
- `user list/create/delete` 迁移为 `participant list/create/delete --type human`。
- room member 命令中的 `--user-id`、`--user-ids`、`--member-ids` 应改为 `--participant-id`、`--participant-ids` 或结构化 participant ref。
- message 命令中的 `--sender-id` 应改为 path 或显式 `--participant-id`；`--mention-id` 应支持重复传入或改为 `--mention-participant-id`，并发送为 `mentions` / `mention_ids` 数组。
- Feishu 配置命令中的 `--bot-id` 应改为 `--participant-id` 或 `--channel-app-ref`，取决于命令是在配置 participant 绑定，还是在管理 Feishu app/config。
- team/task 命令里的 `--lead-bot-id`、`--member-bot-ids`、`--bot-id`、`--actor-id` 应改为 `--lead-participant-id`、`--member-participant-ids`、`--participant-id`、`--actor-participant-id`；只有明确操作 runtime 时才使用 `--agent-id`。
- `csgclaw-cli` 这类 runtime 内置命令也要同步更新；内置技能和模板不能继续依赖旧 `bot_id`、`sender_id`、`mention_id` 和 `user_ids` 语义。

## 一步到位实施范围

- 新增 participant request/response types。
- 新增 participant storage，规范 key 为 `(channel, id)`。
- 新增 Participant ID 生成器：从显式 `id` 或稳定 key 生成可读 slug，冲突时追加短随机后缀；不要从可修改的 `name` 派生 ID；新建 agent-backed participant 的默认 Agent ID 保持 `u-{participant_id}`，但 bootstrap manager 例外，participant 为 `manager`、Agent 为 `u-manager`。
- 用 participant API 替换公开 `User` API。只有 CSGClaw 和外部 channel adapter 需要时，才保留内部 channel identity/profile store。
- 新增 participant service，支持 list、get、create、patch、delete 和 agent binding。
- 注册 `/api/v1/channels/{channel}/participants`。
- 注册 participant event/message 路由：
  `/api/v1/channels/{channel}/participants/{id}/events` 和
  `/api/v1/channels/{channel}/participants/{id}/messages`。
- 注册 participant notification 路由：
  `/api/v1/channels/{channel}/participants/{id}/notifications`。
- 在 `/api/v1/agents/{agent_id}/llm/*` 下注册 agent LLM 路由。
- 实现 `create`、`reuse`、`none` 三种创建模式。
- 支持 `include_agent` 和 `include_channel_user` 响应展开。
- 增加测试：创建 Feishu participant `test` 并绑定到 agent `u-qa`。
- sender 和 mention ID 统一通过 participant service 解析。
- 更新 CSGClaw IM mention 渲染，支持 human 和 agent participant。
- 更新 Feishu 发送链路，让 mention 解析到 participant 的 `channel_user_ref`，不再要求每个 mention 对象都有配置好的 bot app。
- 增加测试：Agent 在 CSGClaw IM 和飞书中 @ 真人。
- 增加测试：Feishu human participant 使用 `channel_app_ref + open_id` 作为 mention 目标，不需要自己的 bot app/config。
- 将 message send 请求从单个 `mention_id` 调整为 `mentions` / `mention_ids` 数组。
- 将 room membership 请求字段从 `user_ids` 改为 `participant_ids` 或结构化 participant ref。
- 为 `GET /api/v1/agents` 增加 `participants` 展开。
- 包含飞书和未来 channel store 中的 participant。
- 增加测试：agent `u-qa` 同时展示 `csgclaw:qa` 和 `feishu:test` 两个绑定。
- 增加测试：删除 `feishu:test` participant 不删除仍被 `csgclaw:qa` 使用的 agent `u-qa`。
- 用基于意图的 channel action 替代当前 create-agent/create-bot 混淆。
- CSGClaw UI 创建可聊天 Agent 时，使用
  `POST /api/v1/channels/csgclaw/participants`，不要直接创建 agent 后再单独创建 user。
- 增加“创建全新的 Bot”和“从已有 Bot 添加”流程。
- 增加“添加真人”流程。
- 用 participant-scoped notification endpoint 替换当前 notification bot webhook/pull 路由。
- Agent 页面聚焦 runtime 配置和生命周期。
- 更新 CLI 和 `csgclaw-cli`：规范命令使用 participant，并注册 `pt` 短别名；旧 bot/user/member/message 参数同步改为 participant 语义。
- 迁移旧 `bots.json`、IM state、Feishu config 和 Team state 里的身份引用；旧 runtime image/template contract 过期时在 UI 提醒 recreate，当前 recreate 只保留用户 skills。
- 本次不实现聊天记录同步、sync storage 或 sync API；只保证身份模型兼容未来同步。
- 在同一个变更中删除 channel bot CRUD、Feishu bot event、`/api/bots/*` 和公开 `/users` 路由。
- 删除旧 handler 前，先把 runtime bridge 调用方替换成 participant/agent scoped 路由。

## 为什么这是最佳方案

这个模型符合真实领域边界。真人和 bot 是 channel 身份，Agent 是可复用的 runtime 能力。Mention 属于 channel 身份层，不属于 runtime 层。

它也去掉了 ID 相等假设。Feishu participant 可以叫 `test`，同时显式绑定到底层 agent `u-qa`。这个关系是持久化外键，不再是命名约定。

最终结果是一个目标 API，而不是两个并存的 API 面。UI 创建 Agent 时不再调用 bot API；channel 流程显式创建 participant。UI 仍然可以用用户意图组织流程，而不是暴露内部模型术语。
