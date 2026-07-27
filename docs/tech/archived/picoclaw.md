# PicoClaw 对接说明

本文说明 PicoClaw 如何对接当前这版 CSGClaw IM。

当前推荐模式是 SSE：

- PicoClaw 主动连接 IM 的 SSE 接口收消息
- PicoClaw 主动调用 IM 的发送接口回消息

对应关系：

- 入站：`GET /api/bots/{bot_id}/events`
- 出站：`POST /api/bots/{bot_id}/messages/send`

## 1. 总体接法

PicoClaw 侧建议新增一个自定义 channel，例如 `myim`。

这个 channel 的职责：

- `Start()`
  - 主动连接 IM 的 SSE 接口
  - 持续读取消息事件
  - 把事件转成 PicoClaw 入站消息并调用 `HandleMessage(...)`
- `Send()`
  - 调用 IM 的发送消息接口
  - 把 PicoClaw 生成的回复发回 IM

这版不需要 PicoClaw 暴露 webhook 给 IM。

## 2. PicoClaw 侧需要配置的信息

PicoClaw 的 `myim channel` 至少需要这几个配置：

- `base_url`
  - CSGClaw IM 服务地址
  - 例如 `http://127.0.0.1:18080`
- `bot_id`
  - PicoClaw 在 IM 中对应的 bot 用户 ID
  - 例如 `u-manager`
- `access_token`
  - 访问 IM SSE 和发送接口时使用的 Bearer Token

这个 `access_token` 来自 IM 侧配置文件 `~/.csgclaw/config.toml` 的 `[server].access_token`。

拼接后的两个关键地址：

- SSE: `GET {base_url}/api/bots/{bot_id}/events`
- Send: `POST {base_url}/api/bots/{bot_id}/messages/send`

## 3. 入站接口

### 3.1 SSE 请求

请求示例：

```http
GET /api/bots/u-manager/events
Authorization: Bearer your-shared-token
Accept: text/event-stream
```

服务端返回 `text/event-stream`。

连接建立后，IM 会持续推送：

```text
event: message
data: {"message_id":"msg-1","room_id":"room-1","chat_type":"direct","sender":{"id":"u-admin","username":"admin","display_name":"Admin"},"text":"hello","timestamp":"1710000000000"}
```

### 3.2 事件字段

当前消息事件 JSON 结构：

```json
{
  "message_id": "msg-1",
  "room_id": "room-1",
  "chat_type": "direct",
  "sender": {
    "id": "u-admin",
    "username": "admin",
    "display_name": "Admin"
  },
  "text": "hello",
  "timestamp": "1710000000000",
  "mentions": ["u-manager"]
}
```

关键字段：

- `message_id`
- `room_id`
- `chat_type`
- `sender.id`
- `text`

说明：

- `chat_type` 当前取值是 `direct` 或 `group`
- 群聊只有在消息 `@bot` 时才会推送给该 bot
- 私聊里只要 bot 在会话里，就会收到消息
- bot 自己发送的消息不会被再次推回

## 4. PicoClaw 侧如何转成入站消息

PicoClaw 收到 SSE 事件后，建议按下面方式转换：

```go
peerKind := "direct"
if evt.ChatType == "group" {
    peerKind = "group"
}

c.HandleMessage(
    ctx,
    bus.Peer{Kind: peerKind, ID: evt.RoomID},
    evt.MessageID,
    evt.Sender.ID,
    evt.RoomID,
    evt.Text,
    nil,
    map[string]string{
        "timestamp": evt.Timestamp,
    },
)
```

这里的重点是：

- `Peer.ID` 使用 `room_id`
- `sender_id` 使用 `sender.id`
- `content` 使用 `text`

## 5. 出站接口

PicoClaw `Send(...)` 时调用：

```http
POST /api/bots/u-manager/messages/send
Authorization: Bearer your-shared-token
Content-Type: application/json

{
  "room_id": "room-1",
  "text": "hello from picoclaw"
}
```

响应：

```json
{
  "message_id": "msg-1742970000000000000"
}
```

建议：

- `msg.RoomID` 直接映射到请求里的 `room_id`
- `msg.Content` 直接映射到请求里的 `text`

## 6. 当前实现边界

当前这版 IM 对接层是最小闭环，不包含这些高级能力：

- 自动重连
- 断线补发
- 消息去重
- 附件消息
- 编辑消息
- typing

所以 PicoClaw 侧当前最好也按“前期简化模式”实现：

- `Start()` 里建立一条 SSE 长连接
- 断线先直接报错或退出
- `Stop()` 时关闭连接
- `Send()` 保持简单 HTTP POST

## 7. 建议的联调步骤

1. 在 IM 侧准备好 `bot_id`、`access_token` 和服务地址
2. PicoClaw `myim channel.Start()` 连上 SSE
3. 在 IM WebUI 中给 bot 发一条私聊消息，确认 PicoClaw 能收到
4. 在群聊中 `@bot` 再发一条消息，确认只在提及时触发
5. 让 PicoClaw 回一条消息，确认 `Send()` 能正常写回 IM

## 8. 典型排查点

如果 PicoClaw 收不到事件，优先检查：

- `Authorization` 是否带了 `Bearer <access_token>`
- `bot_id` 是否和 IM 中的真实用户 ID 一致
- bot 是否已经在目标会话里
- 群聊消息是否真的提及了 bot

如果 PicoClaw 发不回消息，优先检查：

- `room_id` 是否真实存在
- `bot_id` 是否存在于 IM 用户列表
- `text` 是否为空
