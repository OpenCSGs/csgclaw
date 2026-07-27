# csgclaw / csgcli 拆分分析

## 背景

当前 `csgclaw` 命令行工具功能较多，二进制体积也比较大。目标是保留原有 `csgclaw` 的完整本地功能，同时新增一个更小的 `csgcli`，只暴露 `bot / room / member` 三类命令，供外部使用。

目标命令分组：

- `csgclaw`: `onboard / serve / stop / agent / user / bot / room / member`
- `csgcli`: `bot / room / member`

## 结论

这个拆分方向是合理的，也可以做到；但如果只是新增一个 `cmd/csgcli/main.go` 然后继续 import 现有 `csgclaw/cli` 包，`csgcli` 的 size 基本不会变小。

要让 `csgcli` 真正变小，需要先拆 Go package 的依赖边界，尤其是把轻量 CLI/API DTO 从当前 `cli`、`internal/bot` 这类重包里拆出来。

## 1. 拆分是否合理

合理。

当前命令天然分成两类：

- 本地管理类：`onboard / serve / stop / agent / user`
- 外部操作类：`bot / room / member`

从当前代码看，`bot / room / member` 基本都是通过 HTTP API 调 server，不直接启动本地服务：

- `bot list/create` 在 `cli/bot.go` 中通过 `NewAPIClient` 调接口。
- `room list/create/delete` 在 `cli/room.go` 中通过 `NewAPIClient` 调接口。
- `member list/create` 在 `cli/member.go` 中通过 `NewAPIClient` 调接口。

所以产品边界可以定义为：

- `csgclaw`: 本地完整工具，保留全部命令。
- `csgcli`: 更轻量的 lite CLI，只暴露 `bot / room / member`。

需要注意两个已有行为：

- `member` 当前默认只支持 `--channel feishu`。`csgclaw` channel 会报 `member ... currently supports --channel feishu`，逻辑在 `cli/http_client.go` 的 `roomMembersPath`。
- `room delete` 当前走 `/api/v1/rooms/<id>`，没有 channel-aware delete。如果外部 `csgcli` 主要面向 feishu，需要确认这是不是预期 API。

## 2. 是否可以做到

可以，但不能直接复用当前 `cli.New()`。

当前 `cmd/csgclaw/main.go` 直接 import 整个 `csgclaw/cli` 包：

```go
import "csgclaw/cli"
```

`cli.App.Execute` 的 switch 包含所有命令：

```go
switch rest[0] {
case "onboard":
case "serve":
case "stop":
case "agent":
case "bot":
case "room":
case "member":
case "user":
case "_serve":
}
```

更关键的是，`cli` 当前是单个 Go package。Go 会以 package 为单位编译依赖，而不是只按最终入口实际调用的函数做源码级拆包。

当前 `cli/serve.go` import 了这些重依赖：

```go
"csgclaw/internal/agent"
"csgclaw/internal/bot"
"csgclaw/internal/channel"
"csgclaw/internal/config"
"csgclaw/internal/im"
"csgclaw/internal/server"
```

`cli/http_client.go` 也同时 import 了：

```go
"csgclaw/internal/agent"
"csgclaw/internal/bot"
"csgclaw/internal/channel"
"csgclaw/internal/config"
"csgclaw/internal/im"
```

因此，如果 `cmd/csgcli` 继续 import `csgclaw/cli`，仍会把本地服务、agent、channel、server 等依赖带进来。

建议拆分为类似结构：

```text
cmd/csgclaw/main.go       -> import 全功能 app
cmd/csgcli/main.go        -> import 轻量 app

cli/common                -> GlobalOptions、flag parsing、output、HTTP base client
cli/csgclaw               -> onboard/serve/stop/agent/user + bot/room/member
cli/csgcli                -> bot/room/member only

internal/apitypes         -> Bot / CreateBotRequest / Room / User / AddRoomMembersRequest 等 DTO
internal/bot              -> 服务实现，继续依赖 agent/channel/im
internal/im               -> IM 服务实现和模型，必要时也拆模型与服务
```

最重要的拆分点是 `internal/bot`。

现在 `cli/bot.go` 为了使用 `bot.Bot` 和 `bot.CreateRequest` import 了 `internal/bot`。但 `internal/bot/service.go` 又 import：

```go
"csgclaw/internal/agent"
"csgclaw/internal/channel"
"csgclaw/internal/im"
```

这会把 agent runtime、boxlite、飞书 SDK 等重依赖带进轻量 CLI。应该把 API 请求/响应结构从服务包里拆到轻量 DTO 包，或者让 `csgcli` 自己定义只用于 JSON 的 DTO。

## 3. 拆分后 csgcli 的 size 是否真的会变小

会，但前提是包依赖真的拆开。

本地实测环境：

```text
GOOS=darwin
GOARCH=arm64
CGO_ENABLED=1
```

实测结果：

```text
当前 csgclaw:                         89M
新增入口但继续 import 现有 csgclaw/cli: 89M
轻量 HTTP-only probe:                 8.1M
```

这个结果说明：

- 直接新增 `cmd/csgcli` 但复用当前 `cli` 包，体积不会明显下降。
- 如果 `csgcli` 只依赖 `net/http`、`encoding/json`、少量 DTO 和 flag/output 代码，体积可以大幅下降。

当前大头很可能主要来自两块：

- `boxlite` / agent 侧 cgo 依赖。
- 飞书 SDK。

仓库里的 `third_party/boxlite-go/libboxlite.a` 本身约 `107M`。当前 full build 链接时也出现了大量 `libboxlite.a` 相关 warning，说明本地服务/agent 依赖已经进入构建链路。

## 推荐实施路径

建议按依赖边界拆，而不是只按命令入口拆。

第一步：抽出轻量 API DTO。

- 新增 `internal/apitypes` 或类似包。
- 放置 `Bot`、`CreateBotRequest`、`Room`、`User`、`CreateRoomRequest`、`AddRoomMembersRequest` 等 HTTP API 请求/响应结构。
- `internal/bot`、`internal/im`、`internal/api` 和 CLI 都引用这个轻量 DTO 包。

第二步：拆轻量 HTTP client。

- 从当前 `cli/http_client.go` 中拆出只用于 `bot / room / member` 的 client。
- 这个 client 不能 import `internal/agent`、`internal/channel`、`internal/server`。
- 默认 endpoint 可以保留当前行为，但要避免为了默认值 import 过重的 config 包；可以把 `DefaultAPIBaseURL` 放到更轻的 common 包。

第三步：拆 CLI app。

- `csgclaw` 使用全功能 app。
- `csgcli` 使用轻量 app，只注册 `bot / room / member`。
- `bot / room / member` 的命令实现尽量共享，但共享代码必须只依赖轻量 client 和 DTO。

第四步：新增构建目标。

- 新增 `cmd/csgcli/main.go`。
- 更新 `Makefile` 和 release/package 脚本。
- 分别构建 `csgclaw` 和 `csgcli`，并在 CI 中记录体积。

第五步：验证体积。

建议至少验证：

```sh
go build -o /tmp/csgclaw ./cmd/csgclaw
go build -o /tmp/csgcli ./cmd/csgcli
ls -lh /tmp/csgclaw /tmp/csgcli
go list -deps ./cmd/csgcli
```

`go list -deps ./cmd/csgcli` 中不应出现：

- `github.com/RussellLuo/boxlite/sdks/go`
- `csgclaw/internal/agent`
- `csgclaw/internal/server`
- 非必要的 `github.com/larksuite/oapi-sdk-go/v3/...`

## 总体建议

拆分值得做，但核心工作不是“新增一个二进制入口”，而是“把轻量客户端依赖从本地 server/agent 实现中剥离出来”。

推荐目标是：

- `csgclaw` 保持完整功能和本地管理能力。
- `csgcli` 只作为远端 HTTP API client。
- `csgcli` 的依赖图中不包含 boxlite、本地 server、agent runtime 和完整飞书 SDK。

这样功能边界和二进制体积都会比较干净。
