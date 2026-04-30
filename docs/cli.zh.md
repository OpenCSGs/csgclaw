# CSGClaw CLI 文档

本文档补充 [architecture.md](./architecture.md) 中的 CLI 章节。`architecture.md` 说明了 `csgclaw` 与 `csgclaw-cli` 的职责边界，而本文档记录当前代码中已经实现的命令、参数、默认值与实际行为。

## CLI 定位

`csgclaw` 是完整的本地运维 CLI，用于管理初始化、本地服务生命周期、Agent 运行时，以及共享的协作命令。

`csgclaw-cli` 是轻量级 HTTP 客户端，主要面向 Bot、Agent 和脚本。它只暴露协作相关命令，不负责初始化、配置文件管理或本地服务生命周期。

两个 CLI 都是本地 API 的薄客户端，不会直接操作 BoxLite、底层存储或渠道 SDK。

## 通用约定

### 输出格式

- `--output table` 输出适合人读的表格或纯文本。
- `--output json` 输出结构化 JSON。
- 如果不显式传入 `--output`：
  - 输出到终端时，默认是 `table`
  - 输出被管道或重定向时，默认是 `json`
- 特殊情况：
  - `csgclaw serve`、`csgclaw stop`、`csgclaw agent logs` 默认总是 `table`
  - `csgclaw-cli --version` 默认是 `table`

### 环境变量

两个 CLI 都支持：

- `CSGCLAW_BASE_URL`：默认 API 地址
- `CSGCLAW_ACCESS_TOKEN`：默认 API Token

如果同时传入 `--endpoint` 或 `--token`，命令行参数优先生效。

### 渠道

大多数协作命令都支持 `--channel`：

- `csgclaw`
- `feishu`

如果不传，默认值是 `csgclaw`。

### 配置与本地路径

`csgclaw` 额外支持 `--config`，默认读取 `~/.csgclaw/config.toml`。

常用默认路径如下：

- 配置文件：`~/.csgclaw/config.toml`
- 守护进程日志：`~/.csgclaw/server.log`
- 守护进程 PID：`~/.csgclaw/server.pid`
- agents 状态：`~/.csgclaw/agents/state.json`
- 内置 IM 状态：`~/.csgclaw/im/state.json`

## `csgclaw`

### 全局参数

用法：

```bash
csgclaw [global-flags] <command> [args]
```

全局参数：

- `--endpoint string`：HTTP 服务地址。默认来自 `CSGCLAW_BASE_URL`。
- `--token string`：API 鉴权 Token。默认来自 `CSGCLAW_ACCESS_TOKEN`。
- `--output string`：`table` 或 `json`。
- `--config string`：配置文件路径。
- `--version`、`-V`：打印版本并退出。

顶层命令：

- `onboard`
- `serve`
- `stop`
- `agent`
- `model`
- `user`
- `bot`
- `room`
- `member`
- `message`
- `completion`

### Shell 补全

两个 CLI 都可以生成 `bash`、`zsh` 和 `fish` 的 shell 补全脚本。

示例：

```bash
csgclaw completion bash
csgclaw completion zsh
csgclaw completion fish
```

### `csgclaw onboard`

初始化本地配置和引导状态。

用法：

```bash
csgclaw onboard [flags]
```

参数：

- `--provider string`：LLM 提供方预设，支持 `csghub-lite`、`custom`。
- `--base-url string`：LLM 提供方 Base URL。
- `--api-key string`：LLM 提供方 API Key。
- `--models string`：逗号分隔的模型 ID 列表。
- `--reasoning-effort string`：可选，上游 `reasoning_effort` 默认值。
- `--debian-registries string`：用于拉取 `debian:bookworm-slim` 的 OCI registry 列表，逗号分隔，并持久化到配置文件。
- `--log-level string`：初始化阶段日志级别，支持 `debug`、`info`、`warn`、`error`，默认 `info`。

行为说明：

- 如果本地还没有配置，且没有显式传入模型相关参数，`onboard` 可以进入交互式引导。
- 非交互场景下，建议显式传入模型参数。
- 命令会写入配置、确保 IM 引导状态存在，并确保引导 Manager Bot 存在。
- 如果传入 `--debian-registries`，命令会同步更新配置中的 `sandbox.debian_registries`。
- 如果模型配置不完整，后续 `serve` 会报错，并提示如何补齐配置。

示例：

```bash
csgclaw onboard
csgclaw onboard --provider csghub-lite --models Qwen/Qwen3-0.6B-GGUF
csgclaw onboard --base-url https://api.openai.com/v1 --api-key "$OPENAI_API_KEY" --models gpt-5.4-mini
csgclaw agent create --replace --force --id manager --image ghcr.io/example/manager:latest
```

### `csgclaw serve`

启动本地 HTTP 服务。

用法：

```bash
csgclaw serve [-d|--daemon] [flags]
```

参数：

- `--daemon`、`-d`：后台运行。
- `--log-level string`：日志级别，支持 `debug`、`info`、`warn`、`error`，默认 `info`。
- `--log string`：后台模式日志路径，仅 daemon 模式有效。默认 `~/.csgclaw/server.log`。
- `--pid string`：后台模式 PID 文件路径，仅 daemon 模式有效。默认 `~/.csgclaw/server.pid`。

行为说明：

- 从 `--config` 或 `~/.csgclaw/config.toml` 加载配置。
- 启动前会校验最终模型配置是否完整。
- 对 `csghub-lite` 会做连通性预检查。
- 前台模式下会打印生效配置和 IM 访问地址。
- 后台模式会拉起隐藏的 `_serve` 内部入口，并等待 `/healthz` 健康检查成功。

示例：

```bash
csgclaw serve
csgclaw serve --daemon
csgclaw serve --config /path/to/config.toml
csgclaw --endpoint http://127.0.0.1:18080 serve
```

### `csgclaw stop`

停止后台运行的本地服务。

用法：

```bash
csgclaw stop [flags]
```

参数：

- `--pid string`：PID 文件路径。默认 `~/.csgclaw/server.pid`。

行为说明：

- 从 PID 文件读取进程号并发送 `SIGTERM`。
- 如果进程已经不存在，会删除失效的 PID 文件并返回对应状态。

### `csgclaw model auth`

管理模型 Provider 通过嵌入式 CLIProxyAPI 使用的本地 Codex 和 Claude Code 鉴权。

用法：

```bash
csgclaw model auth login <provider> [flags]
```

Provider：

- `codex`
- `claude-code`

参数：

- `--no-browser`：打印 OAuth URL，而不是自动打开浏览器。

行为说明：

- `codex` 会优先复用 `~/.codex/auth.json`，没有可用 token 时再启动 OAuth。
- `claude-code` 会在 macOS 上优先探测 Keychain，没有可用 token 时再启动 OAuth。
- 鉴权文件会写入 CLIProxy auth 目录，默认是 `~/.cli-proxy-api`。
- 模型 Provider 鉴权放在 `csgclaw model auth` 下，不和服务端自身 API 鉴权混在一起。

示例：

```bash
csgclaw model auth login codex
csgclaw model auth login claude-code --no-browser
```

### `csgclaw agent`

管理运行时 Agent。

用法：

```bash
csgclaw agent <subcommand> [flags]
```

子命令：

- `list`
- `create`
- `start`
- `stop`
- `delete`
- `logs`

#### `csgclaw agent list`

用法：

```bash
csgclaw agent list [flags]
```

参数：

- `--filter string`：按 Agent 状态过滤列表结果。

#### `csgclaw agent create`

用法：

```bash
csgclaw agent create [flags]
csgclaw agent create [-r|--replace] --id <id> [flags]
```

参数：

- `--replace`、`-r`：原地替换一个已存在的 Agent。
- `--force`、`-f`：原地替换时跳过确认。
- `--id string`：Agent ID。
- `--name string`：Agent 名称。
- `--description string`：Agent 描述。
- `--image string`：Agent 镜像。
- `--profile string`：Agent 使用的 LLM profile。

行为说明：

- 不带 `--replace` 时，命令会创建一个新 Agent。
- 带 `--replace` 时，必须传入 `--id`。
- 带 `--replace` 时，CLI 会发送一次带有 `replace: true` 的 create 请求，并在 `field_mask` 中列出本次显式传入的字段。
- API/service 会读取现有 Agent，保留未被 mask 选中的字段，应用 mask 中的字段，然后执行重建。
- 原地替换默认会要求确认，除非传入 `--force`。
- `image` 字段也遵循同样的 field-mask 行为：未显式传入时保留原值，显式传入 `--image` 时覆盖原值。

#### `csgclaw agent delete`

用法：

```bash
csgclaw agent delete <id>
csgclaw agent delete --all [-f|--force]
```

参数：

- `--all`、`-a`：删除全部 Agent。
- `--force`、`-f`：删除全部 Agent 时跳过确认。

行为说明：

- 不带 `--all` 时，必须且只能传入一个 Agent ID。
- 带 `--all` 时，不能再传位置参数 ID。
- 批量删除默认会要求确认，只有传入 `--force` 才会跳过。

#### `csgclaw agent start`

用法：

```bash
csgclaw agent start <id>
```

#### `csgclaw agent stop`

用法：

```bash
csgclaw agent stop <id>
```

#### `csgclaw agent logs`

用法：

```bash
csgclaw agent logs <id> [-f|--follow] [-n lines]
```

参数：

- `-f`、`--follow`：持续跟随日志输出。
- `-n int`：拉取的日志行数，默认 `20`。

行为说明：

- `-n` 必须大于 `0`。
- 只有非 follow 模式支持 `--output json`。
- `--output json --follow` 会直接报错。

示例：

```bash
csgclaw agent list
csgclaw agent list --filter running
csgclaw agent create --name alice --description "frontend worker" --profile openai.gpt-5.4-mini
csgclaw agent create -r --id agent-alice
csgclaw agent create --replace --id agent-alice --name alice-v2 --profile openai.gpt-5.4-mini --force
csgclaw agent start agent-alice
csgclaw agent stop agent-alice
csgclaw agent logs agent-alice -n 50
csgclaw agent logs agent-alice --follow
csgclaw agent delete agent-alice
csgclaw agent delete --all --force
```

### `csgclaw user`

管理渠道用户。

用法：

```bash
csgclaw user <subcommand> [flags]
```

子命令：

- `list`
- `create`
- `delete`

#### `csgclaw user list`

用法：

```bash
csgclaw user list [flags]
```

参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。

#### `csgclaw user create`

用法：

```bash
csgclaw user create [flags]
```

参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--id string`：用户 ID。
- `--name string`：用户名。
- `--handle string`：用户 handle。
- `--role string`：用户角色。
- `--avatar string`：头像缩写，仅 `feishu` 使用。

行为说明：

- `--name` 为必填。
- `csgclaw` 与 `feishu` 会走不同的后端路由和请求体结构。

#### `csgclaw user delete`

用法：

```bash
csgclaw user delete <id> [flags]
```

参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。

示例：

```bash
csgclaw user list
csgclaw user list --channel feishu
csgclaw user create --name Alice --handle alice --role worker
csgclaw user create --channel feishu --name Alice --handle alice --role manager --avatar AL
csgclaw user delete u-alice
```

### `csgclaw` 中共享的协作命令组

以下命令组与 `csgclaw-cli` 共享同一套实现，因此参数和行为完全一致。

#### `bot`

用法：

```bash
csgclaw bot <subcommand> [flags]
```

子命令：

- `list`
- `create`
- `delete`

`bot list` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--role string`：按 `manager` 或 `worker` 过滤。

`bot create` 参数：

- `--id string`：Bot ID。
- `--name string`：必填。
- `--description string`：Bot 描述。
- `--role string`：必填，取值为 `manager` 或 `worker`。
- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--model-id string`：Agent model ID。

`bot delete` 用法与参数：

```bash
csgclaw bot delete <id> [flags]
```

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。

#### `room`

用法：

```bash
csgclaw room <subcommand> [flags]
```

子命令：

- `list`
- `create`
- `delete`

`room list` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。

`room create` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--title string`：房间标题。
- `--description string`：房间描述。
- `--creator-id string`：创建者用户 ID。
- `--member-ids string`：逗号分隔的成员 ID 列表。
- `--locale string`：房间 locale。

`room delete` 用法与参数：

```bash
csgclaw room delete <id> [flags]
```

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。

#### `member`

用法：

```bash
csgclaw member <subcommand> [flags]
```

子命令：

- `list`
- `create`

`member list` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--room-id string`：目标房间 ID。

`member create` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--room-id string`：目标房间 ID。
- `--user-id string`：必填，要加入房间的用户。
- `--inviter-id string`：邀请人用户 ID。
- `--locale string`：房间 locale。

`member create` 行为说明：

- `--user-id` 为必填。

#### `message`

用法：

```bash
csgclaw message <subcommand> [flags]
```

子命令：

- `list`
- `create`

`message list` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--room-id string`：必填。

`message create` 参数：

- `--channel string`：`csgclaw` 或 `feishu`，默认 `csgclaw`。
- `--room-id string`：必填。
- `--sender-id string`：必填。
- `--content string`：必填。
- `--mention-id string`：可选，被提及用户 ID。

`message list` 行为说明：

- `--room-id` 为必填。

示例：

```bash
csgclaw bot list
csgclaw bot create --name alice --role worker --model-id gpt-5.4-mini
csgclaw room create --title "release-room" --creator-id admin --member-ids admin,manager
csgclaw member create --room-id room-1 --user-id u-alice --inviter-id admin
csgclaw message list --room-id room-1
csgclaw message create --channel csgclaw --room-id room-1 --sender-id admin --content hello
```

## `csgclaw-cli`

### 全局参数

用法：

```bash
csgclaw-cli [global-flags] <command> [args]
```

全局参数：

- `--endpoint string`：HTTP 服务地址。默认来自 `CSGCLAW_BASE_URL`。
- `--token string`：API 鉴权 Token。默认来自 `CSGCLAW_ACCESS_TOKEN`。
- `--output string`：`table` 或 `json`。
- `--version`、`-V`：打印版本并退出。

顶层命令：

- `bot`
- `room`
- `member`
- `message`
- `completion`

### Shell 补全

示例：

```bash
csgclaw-cli completion bash
csgclaw-cli completion zsh
csgclaw-cli completion fish
```

### 命令组

`csgclaw-cli` 与 `csgclaw` 复用完全相同的实现，包含：

- `bot list`
- `bot create`
- `bot delete`
- `room list`
- `room create`
- `room delete`
- `member list`
- `member create`
- `message list`
- `message create`

因此，上述命令在两个 CLI 中的参数、默认值、校验逻辑和 JSON 输出结构完全一致。

示例：

```bash
csgclaw-cli bot list --channel feishu
csgclaw-cli bot create --name manager --role manager --channel feishu
csgclaw-cli room create --channel feishu --title "ops-room"
csgclaw-cli member list --channel feishu --room-id oc_x
csgclaw-cli message create --channel feishu --room-id oc_x --sender-id u-manager --content hello
```
