# csgclaw / csgclaw-cli bot id 参数统一改造方案

## 0. 已确认的本期边界

本方案只收敛 `csgclaw` / `csgclaw-cli` 的 CLI 身份参数语义，并在本期用第二个 commit 做本地 csgclaw channel adapter 架构整理。

已确认边界：

1. **请求字段不变，legacy 路由兼容保留，并新增 csgclaw channel 路由**
   - 不删除或重命名现有 legacy HTTP 路由。
   - 新增本地 channel-shaped 路由：`/api/v1/channels/csgclaw/rooms`、`/api/v1/channels/csgclaw/messages`、`/api/v1/channels/csgclaw/users`，以及 room members / delete user 等匹配子路径。
   - 不改 JSON request 字段：仍使用 `creator_id`、`member_ids`、`user_ids`、`sender_id`、`mention_id`。
   - CLI 参数 key 也不变：仍使用 `--creator-id`、`--member-ids`、`--user-id`、`--inviter-id`、`--sender-id`、`--mention-id`。

2. **不做 `external` 字段改造**
   - `internal/apitypes.User` 不新增 `external`。
   - Feishu member list 中无法映射的成员继续保持当前代码返回原则。
   - 本期不做 Feishu 成员/消息输出 presentation 改造。

3. **`user list` 不改成 bot id**
   - 完整 `csgclaw user ...` 命令不纳入本期。
   - Feishu `/api/v1/channels/feishu/users` 不纳入本期。
   - 本地 `/api/v1/channels/csgclaw/users` 只镜像现有本地用户逻辑；现阶段 user id 和 bot id 仍一致。
   - Feishu `ListUsers` / `ResolveBotUser` 当前可能返回 open_id，保持现状。

4. **Feishu creator/inviter 继续固定使用 manager app 覆盖**
   - 维持当前 `feishu.Service.appConfigForCreatorLocked` 固定返回 manager app 的方式。
   - 不新增“creator_id / inviter_id 必须等于 u-manager”的强校验。
   - CLI/help/文档可以表达 Feishu 下建议传 manager bot id，但实现不改变当前覆盖行为。

5. **Feishu sender fallback 维持当前代码方式**
   - 维持当前 `appConfigForSenderLocked` 找不到 sender app 时 fallback manager app 的行为。
   - 本期不改成“sender 未配置 app 就报错”。
   - 因此本期只统一 CLI 输入文案，不改变 Feishu 实际发送 app 选择逻辑。

6. **和本次无关的 docs/README 路径类清理暂不处理**
   - 本方案文档自身可以更新。
   - 不把广义文档整理、生成文档路径修正、README 全量同步作为本期必要任务。

## 1. 目标和非目标

### 1.1 目标

- 让用户在 `csgclaw` 和 `csgclaw-cli` 中看到的身份类参数说明都统一成 bot id 语义。
- 参数名称保持现状，避免兼容负担。
- CLI 继续把参数值原样填入现有 API request 字段。
- `internal/apiclient` 继续只负责路由和 JSON，不做 bot id / open_id / app_id 转换。
- 新增 `internal/channel/csgclaw` 薄 adapter，把本地 csgclaw room/member/message 能力从 `internal/api` 直接调用 `internal/im` 的结构中抽出来。
- `internal/im` 继续保持纯 IM service，不引入 bot 概念。

### 1.2 非目标

- 不删除 legacy 请求路由；新增 csgclaw channel route 是本期架构边界的一部分。
- 不改变请求 JSON 字段。
- 不新增 `--bot-id`、`--sender-bot-id`、`--member-bot-ids` 等新 CLI 参数。
- 不新增 `external` 字段。
- 不做 Feishu member/message/user 输出从 open_id 到 bot id 的 presentation 改造。
- 不改 Feishu creator/inviter 固定 manager app 的当前行为。
- 不改 Feishu sender 缺配置 fallback manager app 的当前行为。
- 不改 Feishu bot runtime SSE / MessageBus 的 open_id 语义。

## 2. 当前代码依据

### 2.1 两个 CLI 入口复用命令

- `cli/app.go` 注册完整 `csgclaw` 命令，其中包含 `bot`、`room`、`member`、`message`。
- `cli/csgclawcli/app.go` 注册 lite `csgclaw-cli` 命令，其中包含 `bot`、`room`、`member`、`message`。
- 两个入口复用：
  - `cli/bot`
  - `cli/room`
  - `cli/member`
  - `cli/message`

因此 help 文案和 CLI 参数校验一处修改，会同时影响两个入口。

### 2.2 request 字段现状

现有 API request 类型在 `internal/apitypes/types.go`：

- `CreateRoomRequest`
  - `creator_id`
  - `member_ids`
- `AddRoomMembersRequest`
  - `room_id`
  - `inviter_id`
  - `user_ids`
- `CreateMessageRequest`
  - `room_id`
  - `sender_id`
  - `mention_id`

本期不改这些字段。

### 2.3 CLI 当前只是直传参数值

- `cli/room/room.go`：`--creator-id` / `--member-ids` 直接填入 `CreateRoomRequest.CreatorID` / `MemberIDs`。
- `cli/member/member.go`：`--user-id` / `--inviter-id` 直接填入 `AddRoomMembersRequest.UserIDs` / `InviterID`。
- `cli/message/message.go`：`--sender-id` / `--mention-id` 直接填入 `CreateMessageRequest.SenderID` / `MentionID`。
- `internal/apiclient/client.go`：按 channel 选择 `/api/v1/channels/csgclaw/...`、`/api/v1/channels/feishu/...` 或 legacy `/api/v1/...` 路由，不做身份转换。

本期继续保持这个模型，只改用户可见文案和本地 csgclaw channel 架构边界。

### 2.4 本地 csgclaw 当前等价关系

当前 csgclaw channel 中，bot 创建路径会把 bot/agent id 写成本地 IM user id，因此现阶段：

```text
bot id == agent id == IM user id
```

`internal/im.Service` 仍只认识 user id，不认识 bot id。新增 `internal/channel/csgclaw` 后，转换规则仍是恒等映射：

```text
bot id -> IM user id = bot id
```

### 2.5 Feishu 当前行为保持

本期明确保持以下 Feishu 现有行为：

- `creator_id` / `inviter_id`：当前实现固定使用 manager app credential，不按任意 creator/inviter bot 动态选择 app。
- room owner：仍使用配置中的 `admin_open_id`。
- `member_ids` / `user_ids`：仍按 bot id / Feishu app config key 查 `apps[botID].AppID`，用于拉 bot 入群。
- `sender_id`：当前找到 sender app 就用 sender app；找不到时 fallback manager app。本期不改。
- `mention_id`：仍按 bot id / Feishu app config key 解析 mention open_id。
- Feishu user/member/message 返回值：保持当前代码原则，不在本期做统一 bot id presentation。

## 3. 命令级改造说明

### 3.1 bot 命令

`bot` 命令本身已经使用 bot id 语义，不需要核心改造。

保留语义：

- `bot list`：
  - `id` / `ID` 是 bot id。
  - `user_id` / `USER` 仍是 channel user id；Feishu 下可以是 open_id。
- `bot create --id`：输入 bot id。
- `bot delete <id>`：输入 bot id。
- `bot config --bot-id`：输入 bot id；`app_id`、`app_secret`、`admin_open_id` 仍是 Feishu/channel 配置概念，不是 bot id。

本期不把 `Bot.UserID` 统一改成 bot id。

### 3.2 room create

用户输入示例：

```bash
csgclaw-cli room create \
  --title ops \
  --creator-id u-manager \
  --member-ids u-alice,u-bob

csgclaw-cli room create \
  --channel feishu \
  --title ops \
  --creator-id u-manager \
  --member-ids u-alice,u-bob
```

本期要求：

- 参数 key 不变。
- `--creator-id` help 改成 creator bot id 语义。
- `--member-ids` help 改成 comma-separated member bot ids 语义。
- request 仍填 `creator_id` / `member_ids`。

Feishu 注意：

- CLI 侧建议传 manager bot id，例如 `u-manager`。
- 代码继续固定使用 manager app credential。
- 不新增强校验。

### 3.3 member create

用户输入示例：

```bash
csgclaw-cli member create \
  --room-id room-123 \
  --user-id u-alice \
  --inviter-id u-manager

csgclaw-cli member create \
  --channel feishu \
  --room-id oc_alpha \
  --user-id u-alice \
  --inviter-id u-manager
```

本期要求：

- 参数 key 不变。
- `--user-id` help 改成 bot id to add 语义。
- `--inviter-id` help 改成 inviter bot id 语义。
- 缺参错误从纯字段名错误改成 bot id 语义，例如 `--user-id bot id is required`。
- request 仍填 `user_ids` / `inviter_id`。

Feishu 注意：

- CLI 侧建议 `--inviter-id` 传 manager bot id。
- 代码继续固定使用 manager app credential。
- `--user-id` 继续作为 bot id / Feishu app config key，用于查 app_id 入群。

### 3.4 message create

用户输入示例：

```bash
csgclaw-cli message create \
  --room-id room-123 \
  --sender-id u-manager \
  --content "hello alice" \
  --mention-id u-alice

csgclaw-cli message create \
  --channel feishu \
  --room-id oc_alpha \
  --sender-id u-manager \
  --content "hello alice" \
  --mention-id u-alice
```

本期要求：

- 参数 key 不变。
- `--sender-id` help 改成 sender bot id 语义。
- `--mention-id` help 改成 mentioned bot id 语义。
- 缺参错误从纯字段名错误改成 bot id 语义，例如 `--sender-id bot id is required` 或 `sender bot id is required`。
- request 仍填 `sender_id` / `mention_id`。

Feishu 注意：

- 维持当前 sender app 选择逻辑：找到 sender app 用 sender app；找不到时 fallback manager app。
- 不在本期改成“sender 未配置时报错”。
- 返回值和内部缓存继续按当前代码原则处理；不做 output presentation 改造。

### 3.5 list/delete 类命令

以下命令没有 bot 身份输入，或输入的是 room id，因此不做参数语义改造：

- `room list`
- `room delete <id>`
- `member list --room-id`
- `message list --room-id`

本期不改这些命令的返回值语义。

## 4. 本地 csgclaw channel adapter 设计

### 4.1 新增包

新增：

```text
internal/channel/csgclaw/service.go
```

建议 Go package 名称：

```go
package csgclaw
```

在 `internal/api` 中导入时建议使用别名，避免和模块名/命令名混淆：

```go
csgclawchannel "csgclaw/internal/channel/csgclaw"
```

### 4.2 Service 结构

```go
type Service struct {
    im *im.Service
}

func NewService(imSvc *im.Service) *Service
```

如果 `imSvc == nil`，可以返回 `nil`，让 handler 继续返回 service unavailable。

### 4.3 方法集合

方法只做薄转发，当前转换为恒等映射。

```go
func (s *Service) ListRooms() []im.Room
func (s *Service) CreateRoom(req apitypes.CreateRoomRequest) (im.Room, error)
func (s *Service) DeleteRoom(roomID string) error

func (s *Service) ListRoomMembers(roomID string) ([]im.User, error)
func (s *Service) AddRoomMembers(req apitypes.AddRoomMembersRequest) (im.Room, error)

func (s *Service) ListMessages(roomID string) ([]im.Message, error)
func (s *Service) SendMessage(req apitypes.CreateMessageRequest) (im.Message, error)
```

内部可以预留私有转换函数，但当前保持恒等：

```go
func botIDToUserID(id string) string {
    return strings.TrimSpace(id)
}
```

需要转换的位置：

- `CreateRoom.CreatorID`
- `CreateRoom.MemberIDs`
- `AddRoomMembers.InviterID`
- `AddRoomMembers.UserIDs`
- `CreateMessage.SenderID`
- `CreateMessage.MentionID`

注意：当前转换虽然是恒等，也建议在 adapter 中集中处理，后续如果 bot id 与 IM user id 分离，只改 adapter。

### 4.4 Handler 接入

修改：

```text
internal/api/handler.go
```

建议新增字段：

```go
csgclaw *csgclawchannel.Service
```

构造函数保持现有签名，在 `NewHandlerWithBotAndAuth` 内部由 `imSvc` 自动构造：

```go
localChannel := csgclawchannel.NewService(imSvc)

h := &Handler{
    im:      imSvc,
    csgclaw: localChannel,
    ...
}
```

这样不需要一次性改所有 handler 构造调用点和测试 helper。

### 4.5 Handler 路由调用替换与 csgclaw channel route

保留 legacy route 和 request/response schema，同时新增本地 csgclaw channel route。两套路由暂时使用相同本地逻辑；未来 user id 与 bot id 分离时，只需要收敛在 csgclaw channel handler / adapter。

应替换的直接调用：

- `handleRooms` GET：`h.im.ListRooms()` -> `h.csgclaw.ListRooms()`
- `handleCreateRoom`：`h.im.CreateRoom(req)` -> `h.csgclaw.CreateRoom(req)`
- `handleRoomByID` DELETE：`h.im.DeleteRoom(id)` -> `h.csgclaw.DeleteRoom(id)`
- `handleRoomMembersByID` GET：`h.im.ListMembers(roomID)` -> `h.csgclaw.ListRoomMembers(roomID)`
- `handleAddRoomMembers`：`h.im.AddRoomMembers(serviceReq)` -> `h.csgclaw.AddRoomMembers(serviceReq)`
- `handleMessages` GET：`h.im.ListMessages(roomID)` -> `h.csgclaw.ListMessages(roomID)`
- `handleCreateMessage`：`h.im.CreateMessage(serviceReq)` -> `h.csgclaw.SendMessage(serviceReq)`

保留现有事件发布逻辑：

- `publishRoomEvent`
- `publishMessageCreated`

保留现有 reload 行为：

- GET list 前仍调用 `h.reloadIM()`。
- adapter 持有同一个 `im.Service` 指针，因此 reload 后继续读取同一 service 的最新状态。

### 4.6 不要改 internal/im

`internal/im.Service` 保持现有 user id 语义。

不要把 bot id 概念下沉到：

- `internal/im/service.go` 的 `CreateRoom`
- `internal/im/service.go` 的 `AddRoomMembers`
- `internal/im/service.go` 的 `CreateMessage`

## 5. Feishu 本期明确不改的点

### 5.1 不改 user list

不修改：

- `internal/channel/feishu/service.go` 的 `ListUsers`
- `ResolveBotUser`
- `EnsureUser`
- `/api/v1/channels/feishu/users`
- `csgclaw user list --channel feishu`

### 5.2 不新增 external

不修改：

- `internal/apitypes/types.go` 的 `User`
- `internal/im/service.go` 的 `User`
- CLI user/member table renderer

### 5.3 Feishu 输出 presentation 边界

根据 PR review，Feishu read path 对已配置 bot 新增 open_id -> bot id 反向映射 helper；无法映射的外部/human open_id 暂时保留。

不修改：

- `SendMessage` 返回值 presentation
- Feishu MessageBus
- Feishu bot runtime SSE

### 5.4 不改 manager 覆盖和 sender fallback

不修改：

- `appConfigForCreatorLocked` 固定 manager app 行为。
- `appConfigForSenderLocked` fallback manager app 行为。

如果后续要改变这些行为，必须作为单独需求重新设计和测试。

## 6. Commit 拆分

### Commit 1: CLI 参数文案和校验错误收敛

目标：只改用户可见 CLI 语义，不改 API、service、Feishu 行为。

修改文件：

- `cli/room/room.go`
- `cli/member/member.go`
- `cli/message/message.go`
- 相关 CLI 测试：
  - `cli/app_test.go`
  - `cli/csgclawcli/app_test.go`
  - 如有必要，补充更小的 command/help 测试

具体修改：

- `room create --creator-id` help：改为 creator bot id。
- `room create --member-ids` help：改为 comma-separated member bot ids。
- `member create --user-id` help：改为 bot id to add。
- `member create --inviter-id` help：改为 inviter bot id。
- `member create` 缺 `--user-id` 错误：改为 bot id 语义。
- `message create --sender-id` help：改为 sender bot id。
- `message create --mention-id` help：改为 mentioned bot id。
- `message create` 缺 `--sender-id` 错误：改为 bot id 语义。

不得修改：

- `internal/apitypes`
- `internal/apiclient`
- `internal/api` 路由
- `internal/channel/feishu`
- `internal/im`

验证：

```bash
go test ./cli ./internal/apiclient
```

### Commit 2: 新增 csgclaw channel adapter 并接入 API handler

目标：做本期架构优化，让本地 csgclaw room/member/message/user 通过 channel-shaped API route 进入本地 channel adapter，再委托现有 IM service。

新增文件：

- `internal/channel/csgclaw/service.go`
- `internal/channel/csgclaw/service_test.go`
- `internal/api/csgclaw.go`

修改文件：

- `internal/api/handler.go`
- `internal/api/handler_test.go` 中受影响的本地 room/member/message 测试，保持行为期望不变。
- `internal/api/router.go`
- `internal/apiclient/client.go`
- 相关 CLI/apiclient 测试。

具体修改：

- 新增 `csgclaw.Service` 薄 adapter。
- `Handler` 新增 `csgclaw` 字段。
- `NewHandlerWithBotAndAuth` 由现有 `imSvc` 自动构造 adapter，保持构造函数签名不变。
- 本地 `/api/v1/rooms`、`/api/v1/messages`、`/api/v1/rooms/{id}/members` 相关 handler 改为调用 adapter。
- 新增 `/api/v1/channels/csgclaw/rooms`、`/api/v1/channels/csgclaw/messages`、`/api/v1/channels/csgclaw/users`，以及 `/rooms/{id}`、`/rooms/{id}/members`、`/users/{id}` 子路径。
- `internal/apiclient` 在 channel=`csgclaw` 时显式使用 `/api/v1/channels/csgclaw/...`；空 channel 仍保留 legacy `/api/v1/...`。
- 保持所有 response schema、状态码、错误语义尽量不变。
- 保持 `h.reloadIM()` 调用点不变。
- 保持 `publishRoomEvent` / `publishMessageCreated` 不变。

不得修改：

- Feishu service 行为。
- 请求字段。
- `User` schema。
- Feishu write path、MessageBus、SSE 行为。

验证：

```bash
go test ./internal/channel/csgclaw ./internal/api
go test ./cli ./internal/apiclient ./internal/api
```

### 合并前建议验证

```bash
go test ./cli ./internal/apiclient ./internal/api ./internal/bot ./internal/channel/feishu ./internal/channel/csgclaw
```

如触及共享 handler 或类型较多，再运行：

```bash
go test ./...
```

## 7. 风险和验收标准

### 7.1 风险

- CLI 文案统一后，Feishu sender fallback 仍可能导致 `--sender-id u-alice` 在未配置 `u-alice` app 时实际用 manager 发送；这是已确认保留的现有行为，需在文档中避免承诺“严格 sender app”。
- Feishu read path 已对配置内 bot 做 bot id presentation；外部/human open_id、Feishu user list、SendMessage response、MessageBus/SSE 仍可能显示 open_id。
- 新增 csgclaw adapter 后，如果 handler nil 检查不一致，可能把原来的 `im service is not configured` 错误变成新错误。建议尽量保持现有错误文本和状态码。
- `handleIMRooms` / `handleIMMessages` / `handleIMRoomMembers` 复用本地 handler，接入 adapter 时要确认兼容路径也被覆盖。
- 新增 csgclaw channel route 后，需要同步 CLI/apiclient 测试，避免默认 channel=csgclaw 仍走 legacy route。

### 7.2 验收标准

- `csgclaw` 和 `csgclaw-cli` 的相关 help 文案都表达 bot id 语义。
- CLI 参数 key 没有变化。
- legacy API request 路由兼容保留，请求字段没有变化。
- 新增 `/api/v1/channels/csgclaw/{rooms,messages,users}` 并由 apiclient 在 channel=`csgclaw` 时使用。
- Feishu user list / external / SendMessage response / MessageBus/SSE 没有变化。
- Feishu creator/inviter manager 覆盖和 sender fallback 没有变化。
- 本地 csgclaw room/member/message 行为在 API 层与改造前一致。
- 新增 `internal/channel/csgclaw` 后，`internal/im` 仍不出现 bot 概念。
- 目标测试通过。
