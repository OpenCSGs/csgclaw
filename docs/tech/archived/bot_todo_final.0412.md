# Bot 改造 TODO List 2026-04-12

目标：引入统一的 `bot` 概念，把“创建 agent + 创建 channel user”的流程收敛到 `internal/bot`，并通过统一的 CLI 和 API 支持 `csgclaw` 与 `feishu` 两种 channel。

## 设计目标

- bot 是用户直接管理的业务对象。
- bot 可以是 `manager` 或 `worker`。
- 一个 bot 底层对应一个 agent。
- 一个 bot 同时对应一个 channel 里的 user。
- channel 默认是 `csgclaw`，也支持 `feishu`。
- 新增 API：`GET /api/v1/bots`、`POST /api/v1/bots`。
- 新增 CLI：`csgclaw bot list -channel <csgclaw|feishu>`、`csgclaw bot create -channel <csgclaw|feishu>`。
- bot 创建逻辑放在 `internal/bot`，不要继续堆在 `internal/api` handler 里。

## 当前可复用代码

- `internal/api.handleAgents`
  - 当前 `/api/v1/agents` 的入口。
  - `POST` 会创建 worker agent，并调用 `ensureWorkerIMState` 创建 csgclaw IM user。
  - 只能覆盖 csgclaw IM，不能覆盖 Feishu，也没有 bot 记录。

- `internal/agent.Service.CreateWorker`
  - 创建 worker agent 和对应 BoxLite box。
  - 当前会把 agent role 固定为 `worker`。

- `internal/agent.EnsureBootstrapState`
  - 当前 manager box 的主要创建入口。
  - manager 是特殊保留身份，不能直接复用 `CreateWorker`。

- `internal/im.Service.EnsureAgentUser`
  - 可用于在内置 `csgclaw` IM 中创建或确保 user。
  - 会自动创建 Admin & Agent 私聊。

- `internal/channel.FeishuService.CreateUser`
  - 可用于 Feishu channel 的 user 创建。

- `cli/http_client.go`
  - 已有 channel-aware 的 room/user API helper，可参考 `channelPath` 的风格。

## 增量步骤

### Step 1：增加 `internal/bot` 的模型和纯内存/文件 store

新增 `internal/bot` 包，只放数据模型、请求模型、校验和 store，不接 API、不接 CLI、不创建 agent。

建议文件：

```text
internal/bot/model.go
internal/bot/store.go
internal/bot/store_test.go
```

建议模型：

```go
type Role string

const (
    RoleManager Role = "manager"
    RoleWorker  Role = "worker"
)

type Channel string

const (
    ChannelCSGClaw Channel = "csgclaw"
    ChannelFeishu  Channel = "feishu"
)

type Bot struct {
    ID        string    `json:"id"`
    Name      string    `json:"name"`
    Role      string    `json:"role"`
    Channel   string    `json:"channel"`
    AgentID   string    `json:"agent_id"`
    UserID    string    `json:"user_id"`
    CreatedAt time.Time `json:"created_at"`
}

type CreateRequest struct {
    ID          string `json:"id,omitempty"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    Role        string `json:"role"`
    Channel     string `json:"channel,omitempty"`
    ModelID     string `json:"model_id,omitempty"`
}
```

约束：

- `role` 只允许 `manager` 或 `worker`。
- `channel` 为空时归一化为 `csgclaw`。
- `channel` 只允许 `csgclaw` 或 `feishu`。
- `name` 必填。
- store 先只负责保存/读取 bot 记录，不做 agent/user 创建。

建议验证：

```bash
go test ./internal/bot
```

完成后项目仍应能运行，因为还没有接入现有路径。

### Step 2：实现 `internal/bot.Service` 的 list 能力

新增 `internal/bot/service.go`，先只实现 list，不实现 create。

服务依赖先保持简单：

```go
type Service struct {
    store *Store
}
```

能力：

- `List(channel string) ([]Bot, error)`
- 支持按 channel 过滤。
- `channel` 为空时建议返回全部，或者按 API 约定返回默认 `csgclaw`。需要和 CLI 行为保持一致。

建议决策：

- API `GET /api/v1/bots` 不带 `channel` 时返回全部 bot。
- CLI `csgclaw bot list` 默认带 `channel=csgclaw`，所以 CLI 默认只看内置 channel。

建议验证：

```bash
go test ./internal/bot
go test ./...
```

### Step 3：把 bot service 挂到 server 初始化链路，但不暴露新路由

在 server 初始化处创建 `bot.Service`，并注入 `api.Handler`。

需要查看并修改：

```text
internal/server/http.go
internal/api/handler.go
```

注意：

- `api.NewHandler` 当前参数是 agent、im、imBus、picoclaw、feishu。
- 可以先新增一个 `botSvc *bot.Service` 字段，但暂时不注册 `/api/v1/bots`。
- bot store 的文件路径需要放到 config/state 体系里；如果现有 config 没有独立字段，第一步可以把路径派生为 agent/im state 同目录下的 `bots.json`，避免扩大 config 改动。

建议验证：

```bash
go test ./internal/api ./internal/server ./internal/bot
go test ./...
```

这一步完成后行为不变，只是依赖注入准备好了。

### Step 4：新增 `GET /api/v1/bots`

在 `internal/api/router.go` 注册：

```go
mux.HandleFunc("/api/v1/bots", h.handleBots)
```

新增 handler：

```text
GET /api/v1/bots
GET /api/v1/bots?channel=csgclaw
GET /api/v1/bots?channel=feishu
```

行为：

- 只允许 `GET`，其他方法先返回 `405`。
- 如果 `bot.Service` 未配置，返回 `503`。
- channel 不支持时返回 `400`。
- 返回 `[]bot.Bot`。

建议测试：

- `GET /api/v1/bots` 返回全部 bot。
- `GET /api/v1/bots?channel=csgclaw` 只返回 csgclaw bot。
- `GET /api/v1/bots?channel=unknown` 返回 `400`。
- 未配置 bot service 返回 `503`。

建议验证：

```bash
go test ./internal/api ./internal/bot
go test ./...
```

此时新 API 可读，但还不能创建 bot。

### Step 5：在 CLI 增加 `csgclaw bot list`

先只实现 list，不实现 create。

需要修改：

```text
cli/app.go
cli/http_client.go
cli/bot.go
cli/app_test.go
```

CLI 行为：

```bash
csgclaw bot list
csgclaw bot list -channel csgclaw
csgclaw bot list -channel feishu
csgclaw --output json bot list -channel feishu
```

约定：

- `-channel` 默认 `csgclaw`。
- `bot list` 调用 `GET /api/v1/bots?channel=<channel>`。
- 输出列建议：`ID NAME ROLE CHANNEL AGENT USER`。
- JSON 输出直接输出 `[]bot.Bot`。

建议验证：

```bash
go test ./cli ./internal/bot
go test ./...
```

这一步完成后，用户可以通过 CLI 查看 bot 列表。

### Step 6：实现 worker bot 的 csgclaw 创建

先只做最小闭环：

```bash
csgclaw bot create -channel csgclaw --role worker --name alice
```

`internal/bot.Service.Create` 对 `role=worker, channel=csgclaw` 执行：

1. 调用 `agent.Service.CreateWorker` 创建 worker agent 和 BoxLite box。
2. 调用 `im.Service.EnsureAgentUser` 创建或确保 csgclaw IM user。
3. 保存 bot 记录。
4. 返回 bot 记录。

注意事项：

- 这一步可以直接复用 `/api/v1/agents` 的 `ensureWorkerIMState` 思路，但逻辑应下沉到 `internal/bot`。
- 后续统一保留 `/api/v1/agents`。
- `POST /api/v1/bots` 创建成功返回 `201 Created`。
- 如果 agent 创建成功但 IM user 创建失败，需要明确处理策略。

建议错误处理策略：

- 当前阶段先返回错误，并在 TODO 中标记后续补偿删除 agent。
- 不要静默吞掉 user 创建失败。

建议测试：

- `POST /api/v1/bots` 创建 csgclaw worker bot。
- 创建后 `GET /api/v1/bots?channel=csgclaw` 能看到记录。
- 创建后 `GET /api/v1/agents` 能看到对应 worker agent。
- 创建后 `GET /api/v1/users` 能看到对应 IM user。

建议验证：

```bash
go test ./internal/bot ./internal/api ./internal/agent ./internal/im
go test ./...
```

此时 csgclaw worker bot 的 API 闭环完成。

### Step 7：实现 worker bot 的 Feishu 创建

扩展 `internal/bot.Service.Create`：

```bash
csgclaw bot create -channel feishu --role worker --name alice
```

对 `role=worker, channel=feishu` 执行：

1. 调用 `agent.Service.CreateWorker` 创建 worker agent 和 BoxLite box。
2. 调用 `channel.FeishuService.CreateUser` 创建 Feishu user。
3. 保存 bot 记录。
4. 返回 bot 记录。

注意事项：

- Feishu user 的 `ID` 可以先使用 agent ID 或 bot 请求里的 ID，但必须在 TODO/代码注释里保持一致。
- 如果未来需要真实 open_id/app_id 映射，不要把这些字段塞进 agent；应放在 bot/channel 状态里。
- Feishu 当前 `CreateUser` 仍有 mock 成分，但 bot service 不应关心具体实现。

建议测试：

- `POST /api/v1/bots {"channel":"feishu","role":"worker","name":"alice"}` 返回 bot。
- 创建后 `GET /api/v1/channels/feishu/users` 能看到对应 user。
- 创建后 `GET /api/v1/bots?channel=feishu` 能看到 bot。

建议验证：

```bash
go test ./internal/bot ./internal/api ./internal/channel
go test ./...
```

### Step 8：在 CLI 增加 `csgclaw bot create`

实现统一 CLI：

```bash
csgclaw bot create --name alice --role worker
csgclaw bot create --name alice --role worker -channel csgclaw
csgclaw bot create --name alice --role worker -channel feishu
```

建议 flags：

```text
--id string
--name string
--description string
--role string        manager or worker
-channel string      csgclaw or feishu, default csgclaw
--model-id string
```

行为：

- CLI 不区分 csgclaw/feishu 的创建路径，统一调用 `POST /api/v1/bots`。
- channel 放进 JSON payload。
- 输出 bot 表格或 JSON。

建议测试：

- 默认 channel payload 是 `csgclaw`。
- `-channel feishu` payload 是 `feishu`。
- `--role worker` 被正确传入。
- 缺少 `--name` 或 `--role` 时有清晰错误。

建议验证：

```bash
go test ./cli ./internal/bot
go test ./...
```

### Step 9：处理 manager bot 创建策略

manager 和 worker 的 agent 创建路径不同，需要单独收敛，避免强行塞进 `CreateWorker`。

建议分两阶段做：

#### Step 9.1：API 接受 manager，但先绑定已有 manager agent

对：

```bash
csgclaw bot create --role manager --name manager -channel csgclaw
```

先实现为：

1. 查找已有 `u-manager` agent。
2. 如果不存在，返回明确错误：`manager agent is not bootstrapped`。
3. 为 channel 创建或确保 manager user。
4. 保存 bot 记录。

这一步不创建新的 manager box，因此风险小。

建议验证：

```bash
go test ./internal/bot ./internal/api
go test ./...
```

#### Step 9.2：如确实需要，通过 bootstrap 创建 manager agent

如果产品要求 `POST /api/v1/bots` 创建 manager 时也能创建 manager box，再把 `agent.EnsureBootstrapState` 或新的 agent service 方法接入 `internal/bot`。

注意：

- 不要让 `agent.Service.CreateWorker` 支持 manager。
- manager 是保留身份，ID/name/box name 应保持稳定。
- 需要覆盖 onboard/config/state 的测试。

建议验证：

```bash
go test ./internal/agent ./internal/bot ./internal/config ./cli
go test ./...
make
```

### Step 10：让 `/api/v1/agents` 统一复用 bot service / agent service

在 bot API 完成后，整理旧 worker API。

建议策略：

- `GET /api/v1/agents` 返回统一 agent 列表。
- `POST /api/v1/agents` 内部创建 `role=worker` agent，并传入：
  - `role=worker`
  - `channel=csgclaw`
- 响应是否保持 agent 对象需要谨慎：
  - 统一通过 `/api/v1/agents` 返回 agent。
  - `/api/v1/bots` 返回 bot。

这一步能减少重复逻辑，同时不破坏旧接口。

建议验证：

```bash
go test ./internal/api ./internal/bot
go test ./...
```

### Step 11：补齐文档和端到端手工验证

需要同步更新：

```text
docs/tech/api.md
docs/tech/architecture.md
README.md 或 README.zh.md 中涉及 worker/bot 的说明
```

建议手工验证：

```bash
make
go test ./...

csgclaw serve
csgclaw bot list
csgclaw bot create --name alice --role worker
csgclaw bot list
csgclaw bot create --name bob --role worker -channel feishu
csgclaw bot list -channel feishu
```

如果 Feishu 需要真实 app 配置，手工验证时要说明当前环境是否使用 mock service 或真实 Feishu API。

## 推荐实现顺序总结

- [x] 1. `internal/bot` 模型和 store。
- [x] 2. `internal/bot` list service。
- [x] 3. 注入 bot service。
- [x] 4. `GET /api/v1/bots`。
- [x] 5. `csgclaw bot list`。
- [x] 6. `POST /api/v1/bots` 支持 `worker + csgclaw`。
- [x] 7. `POST /api/v1/bots` 支持 `worker + feishu`。
- [x] 8. `csgclaw bot create`。
- [x] 9. manager bot 先绑定现有 manager，再评估是否支持创建 manager box。
- [x] 10. 统一通过 `/api/v1/agents` 提供 agent 创建与查询。
- [ ] 11. 更新 API/README 文档和手工验证。

## 每步完成标准

每一步都应满足：

- 能通过该步涉及包的 targeted tests。
- 能通过 `go test ./...`，除非明确记录跳过原因。
- 不破坏现有 `agent`、`room`、`user`、`member` CLI。
- 不把 token/api key 打到日志或响应里。
- 不编辑 `third_party/boxlite-go`。
