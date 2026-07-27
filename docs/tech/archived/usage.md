# CSGClaw IM 使用说明

本文说明当前这版 CSGClaw IM 如何和 PicoClaw 配合使用。

安装 CLI：

- macOS / Linux：`curl -fsSL https://raw.githubusercontent.com/OpenCSGs/csgclaw/main/scripts/install.sh | bash`
- 当前预编译安装包仅支持 macOS arm64 和 Linux amd64

从源码构建：

- 执行 `make build` 或 `make test` 时，会自动下载 BoxLite 预编译静态库 `third_party/boxlite-go/libboxlite.a`
- 当前源码构建同样仅支持 macOS arm64 和 Linux amd64

当前采用的是 SSE 方案：

- 入站：`PicoClaw -> IM` 建立 SSE 长连接，IM 在流上推送新消息事件
- 出站：`PicoClaw -> IM` 调用 IM 发送消息接口

不再使用 IM 主动调用 PicoClaw webhook 的模式。

## 1. 启动 CSGClaw

首次初始化：

```bash
csgclaw onboard \
  --base-url http://127.0.0.1:4000 \
  --api-key sk-please-change-me \
  --models gpt-4o-mini
```

参数含义：

- `onboard` 只负责写入基础配置并初始化本地状态
- `listen_addr` 使用内置默认值
- `advertise_base_url` 是可选扩展配置，本地使用通常不需要设置
- `manager_image_override` 默认为空；如有需要，可在 `~/.csgclaw/config.toml` 中手动设置以覆盖内置默认镜像
- 访问 server 用的共享 `access_token` 需要在初始化后手动修改 `~/.csgclaw/config.toml` 的 `[server]` 版块

初始化完成后启动服务：

```bash
csgclaw serve
```

默认地址：

- WebUI: `http://127.0.0.1:18080/`
- 健康检查: `http://127.0.0.1:18080/healthz`

## 2. 关键接口

### 2.1 创建 Worker

```http
POST /api/v1/agents
Content-Type: application/json

{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "status": "requested",
  "created_at": "2026-03-28T12:00:00Z",
  "model_id": "gpt-4o-mini",
  "manager": {
    "id": "u-manager",
    "name": "manager",
    "description": "bootstrap manager",
    "status": "running",
    "created_at": "2026-03-28T11:59:00Z",
    "model_id": "gpt-4o-mini"
  }
}
```

示例：

```bash
curl -X POST \
  -H 'Content-Type: application/json' \
  http://127.0.0.1:18080/api/v1/agents \
  -d '{
    "id": "u-alice",
    "name": "alice",
    "description": "frontend dev",
    "status": "requested",
    "created_at": "2026-03-28T12:00:00Z",
    "model_id": "gpt-4o-mini",
    "manager": {
      "id": "u-manager",
      "name": "manager",
      "description": "bootstrap manager",
      "status": "running",
      "created_at": "2026-03-28T11:59:00Z",
      "model_id": "gpt-4o-mini"
    }
  }'
```

说明：

- 创建时会真正拉起一个 box，镜像和启动方式与 manager 相同，非调试模式下会运行 `picoclaw gateway -d`
- 会为 worker 自动生成独立的 PicoClaw 配置目录，`bot_id` 使用请求里的 `id`
- 会自动在 IM 中创建对应 user / bot 身份，并创建一个 `Admin & <Worker>` bootstrap 私聊
- `name` 必须唯一，大小写不敏感；且不能为 `manager`
- 返回体里的 `status` 和 `created_at` 以实际 box 启动结果为准，不使用请求体里的占位值

### 2.2 PicoClaw 订阅事件流

```http
GET /api/bots/{bot_id}/events
Authorization: Bearer <token>
Accept: text/event-stream
```

示例：

```bash
curl -N \
  -H 'Accept: text/event-stream' \
  -H 'Authorization: Bearer your-shared-token' \
  http://127.0.0.1:18080/api/bots/u-manager/events
```

返回是 SSE：

```text
event: message
data: {"message_id":"msg-1","room_id":"room-1","chat_type":"direct","sender":{"id":"u-admin","username":"admin","display_name":"Admin"},"text":"hello","timestamp":"1710000000000"}
```

说明：

- `bot_id` 必须是 IM 中真实存在并参与了会话的用户 ID
- 私聊中，只要 bot 在会话里，就会收到消息事件
- 群聊中，只有消息里 `@bot` 时才会收到消息事件
- bot 自己发出的消息不会再次推回给 bot

### 2.3 PicoClaw 发送回复消息

```http
POST /api/bots/{bot_id}/messages/send
Authorization: Bearer <token>
Content-Type: application/json

{
  "room_id": "room-1",
  "text": "hello from picoclaw"
}
```

示例：

```bash
curl -X POST \
  -H 'Authorization: Bearer your-shared-token' \
  -H 'Content-Type: application/json' \
  http://127.0.0.1:18080/api/bots/u-manager/messages/send \
  -d '{"room_id":"room-1","text":"hello from picoclaw"}'
```

响应：

```json
{
  "message_id": "msg-1742970000000000000"
}
```

说明：

- 该接口会把消息写入指定会话
- 消息发送者固定为路径里的 `bot_id`
- `room_id` 必须是当前 IM 中已存在的会话 ID

## 3. WebUI 的配合方式

当前 WebUI 仍然通过原有 IM 接口工作：

- `GET /api/v1/im/bootstrap`
- `POST /api/v1/im/messages`
- `POST /api/v1/im/conversations`
- `POST /api/v1/im/conversations/members`

当用户从 WebUI 发送消息时：

1. 消息先写入 IM 会话
2. IM 判断哪些 bot 需要收到这条消息
3. 如果该 bot 正在订阅 SSE，就把消息推到对应事件流上

## 4. 当前约束

这版实现是面向局域网和早期验证的最小闭环，当前有这些约束：

- 没有断线重连
- 没有消息补发
- 没有事件去重
- 没有历史回放
- 没有附件收发

所以当前更适合：

- 同网段内部调试
- 少量 bot
- 先验证 PicoClaw channel 对接是否跑通

## 5. 配置文件示例

`~/.csgclaw/config.toml` 当前会生成类似内容：

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = ""
access_token = "your-shared-token"

[model]
base_url = "http://127.0.0.1:4000"
api_key = "sk-please-change-me"
model_id = "gpt-4o-mini"

[bootstrap]
manager_image_override = ""
```

## 6. 联调建议

最简单的联调顺序：

1. 启动 CSGClaw IM
2. 调用 `POST /api/v1/agents` 创建一个 worker
3. 先用 `curl -N` 连上 `/api/bots/{bot_id}/events`
4. 在 WebUI 中给 bot 所在私聊发消息，或在群聊里 `@bot`
5. 确认 SSE 能收到 `event: message`
6. 再用 `POST /api/bots/{bot_id}/messages/send` 验证 PicoClaw 回消息链路

如果 SSE 收不到消息，优先检查：

- `bot_id` 是否真实存在
- bot 是否在该会话里
- 群聊消息是否真的 `@bot`
- Authorization Token 是否一致
