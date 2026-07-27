# BoxLite CLI Sandbox Implementation Plan

## 背景

当前 CSGClaw 已经通过 `internal/sandbox` 抽象隔离 agent 运行环境，并提供了基于 vendored BoxLite Go SDK 的 `internal/sandbox/boxlitesdk` adapter。为了降低 CGO、静态库下载、平台预编译包和 SDK 版本绑定带来的集成成本，可以新增一个基于 `boxlite` 命令行的 sandbox provider。

本计划基于本地 `boxlite -h` 输出梳理。`boxlite` CLI 顶层能力包括：

```text
boxlite [OPTIONS] <COMMAND>

Global options:
  --debug
  --home <HOME>          [env: BOXLITE_HOME=]
  --registry <REGISTRY>  can be specified multiple times
  --config <CONFIG>

Commands:
  run
  exec
  create
  list | ls | ps
  rm
  start
  stop
  restart
  pull
  images
  inspect
  cp
  info
  logs
  stats
  serve
```

## 目标

- 新增一个基于 `boxlite` CLI 进程调用的 sandbox provider，建议 provider 名称为 `boxlite-cli`。
- 保持 `internal/agent` 继续只依赖 `internal/sandbox`，不直接拼接或执行 `boxlite` 命令。
- 保留现有 Go SDK provider，先通过配置显式切换到 `boxlite-cli`，验证稳定后再考虑默认值迁移。
- 所有外部命令调用都必须支持 `context.Context` 取消、超时、stdout/stderr 捕获和错误包装。
- 不编辑 `third_party/boxlite-go`。

## 非目标

- 第一阶段不重写 agent 生命周期、bot、IM 或 API 边界。
- 第一阶段不依赖 `boxlite serve` REST API；优先直接调用 CLI 子命令，避免新增长期守护进程管理问题。
- 第一阶段不把 `boxlite cp`、`logs`、`stats` 暴露到公共 API；只预留 adapter 内部扩展点。
- 第一阶段不改变现有 `box_id` 对外字段语义。

## 建议包结构

```text
internal/sandbox/boxlitecli/
  provider.go       # Provider、Runtime、Instance
  runner.go         # command runner 抽象，便于单测
  options.go        # sandbox.CreateSpec -> CLI args
  parse.go          # inspect/list JSON 解析和状态映射
  errors.go         # not found / exit code / stderr 错误归一
```

依赖方向保持为：

```text
internal/agent ─────► internal/sandbox
                         ▲
                         │
                  internal/sandbox/boxlitecli ─────► os/exec + boxlite CLI
```

## 配置设计

短期建议扩展 `[sandbox]`，让用户显式选择 CLI provider：

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
boxlite_cli_config = ""
boxlite_cli_registry = []
```

字段说明：

- `provider`: 支持现有 `boxlite-sdk` 和新增 `boxlite-cli`。
- `home_dir_name`: 继续控制 per-agent BoxLite home 目录名，保持路径兼容。
- `boxlite_cli_path`: `boxlite` 可执行文件路径，默认从 `PATH` 查找。
- `boxlite_cli_config`: 可选 `--config` 路径。
- `boxlite_cli_registry`: 可选 registry 列表，每个值映射一个 `--registry`。

如果希望第一阶段改动更小，可以先只新增 provider，并把 CLI path 固定为 `boxlite`。但最终仍建议加入 `boxlite_cli_path`，否则测试、打包和用户排障都不够明确。

配置变更需要同步更新：

- `internal/config` loader/saver/defaults/tests。
- `cli/onboard` 和 `cli/serve` 的 sandbox provider 接线。
- `README.md`、`docs/README.go.md` 或新增运维说明。

## CLI 能力映射

| sandbox 接口 | CLI 映射 | 关键参数 | 备注 |
| --- | --- | --- | --- |
| `Provider.Open(ctx, homeDir)` | 不启动进程，只构造 runtime | `--home <homeDir>` | 可执行 `boxlite info --home <homeDir>` 做可选健康检查。 |
| `Runtime.Create(ctx, spec)` | `boxlite create` | `--name`, `--detach`, `--rm`, `-e`, `-v`, `-p`, `--cpus`, `--memory`, image | 当前 `CreateSpec` 没有 port/cpu/memory 字段，第一阶段只映射已有字段。 |
| `Runtime.Get(ctx, idOrName)` | `boxlite inspect -f json <box>` | `--home`, `--format json` | 返回轻量 `Instance`，同时可缓存 inspect 结果。 |
| `Runtime.Remove(ctx, idOrName, opts)` | `boxlite rm` | `--force` | `RemoveOptions.Force` 映射 `-f/--force`。 |
| `Instance.Start(ctx)` | `boxlite start <box>` | box ID/name | 成功后可再次 inspect 更新状态。 |
| `Instance.Stop(ctx, opts)` | `boxlite stop <box>` | box ID/name | CLI help 未暴露 force/timeout，遇到 `Force` 或 `Timeout` 非零时返回 unsupported。 |
| `Instance.Info(ctx)` | `boxlite inspect -f json <box>` | `--format json` | 解析 JSON 并映射到 `sandbox.Info`。 |
| `Instance.Run(ctx, spec)` | `boxlite exec <box> -- <cmd>...` | `-e`, stdout/stderr | 当前 `CommandSpec` 只有命令、参数和输出 writer，先不支持 tty、stdin、user、workdir。 |
| `Instance.Close()` | no-op | 无 | CLI provider 没有持久 handle，Close 只释放本地状态。 |

`boxlite run` 同时具备 create/start/exec 语义，但不建议作为 `Runtime.Create` 的基础实现。CSGClaw 的 agent 生命周期需要稳定的 box ID/name、后续 start/stop/inspect/rm，因此应优先使用 `create` + `start`。

## 参数转换规则

`sandbox.CreateSpec` 到 `boxlite create`：

```text
boxlite --home <homeDir> create \
  --name <spec.Name> \
  --detach \
  --rm \
  -e KEY=VALUE \
  -v hostPath:guestPath[:ro] \
  <spec.Image>
```

转换要求：

- 所有参数用 `exec.CommandContext` 的 arg slice 传递，不通过 shell 拼接。
- 环境变量 key 为空时报错，沿用现有 Go SDK adapter 行为。
- mount 的 host/guest path 为空时报错。
- `Mount.ReadOnly=true` 映射为 `host:guest:ro`。
- `Entrypoint` 暂不支持，因为 `boxlite create -h` 未暴露 `--entrypoint`。如果 `CreateSpec.Entrypoint` 非空，应返回 `unsupported sandbox option: entrypoint`，不能静默忽略。
- `Cmd` 暂不支持 `create`，因为 `boxlite create -h` 只接收 image。当前 agent gateway 若依赖 `Cmd`，需要改成 `create` 后 `start` 的默认镜像命令，或确认 CLI 是否存在隐藏参数；确认前必须显式报错。

`sandbox.CommandSpec` 到 `boxlite exec`：

```text
boxlite --home <homeDir> exec <boxIDOrName> -- <spec.Name> <spec.Args...>
```

要求：

- `spec.Name` 为空时报错。
- stdout/stderr 分别连接到 `spec.Stdout` 和 `spec.Stderr`。
- 返回真实 exit code；命令非零退出不应被包装成“执行命令失败但 exit code 丢失”。
- `context` 取消时返回包含超时/取消语义的错误。

## inspect JSON 解析

`boxlite inspect --format json <box>` 是 CLI provider 的核心数据源。实现前需要用真实 CLI 确认 JSON schema，并用 fixture 固化。目标映射为：

```go
type Info struct {
    ID        string
    Name      string
    State     sandbox.State
    CreatedAt time.Time
}
```

状态映射建议：

| BoxLite CLI 状态 | sandbox.State |
| --- | --- |
| `configured`, `created` | `StateCreated` |
| `running` | `StateRunning` |
| `stopping` | `StateUnknown` |
| `stopped` | `StateStopped` |
| `exited` | `StateExited` |
| unknown/empty | `StateUnknown` |

若 `inspect` 在找不到 box 时返回非零退出码和 stderr，应包装为 `sandbox.ErrNotFound`，让上层继续使用 `sandbox.IsNotFound(err)`。

## command runner 抽象

为了让测试不依赖真实 `boxlite` binary，建议引入小接口：

```go
type Runner interface {
    Run(ctx context.Context, req CommandRequest) (CommandResult, error)
}

type CommandRequest struct {
    Path   string
    Args   []string
    Env    []string
    Stdout io.Writer
    Stderr io.Writer
}

type CommandResult struct {
    Stdout   []byte
    Stderr   []byte
    ExitCode int
}
```

生产实现使用 `os/exec`。单测使用 fake runner 断言参数顺序、stdout/stderr 连接、exit code 和错误映射。

注意不要把 secret 直接写入日志。调试日志里可以打印命令名和非敏感参数，但 env 值、token、model key 等必须 redacted。

## `boxlite serve` 的取舍

`boxlite serve` 可以启动 REST API server：

```text
boxlite serve --host 0.0.0.0 --port 8100 --home <homeDir>
```

第一阶段不建议依赖它，原因：

- CSGClaw 已经有自己的 server 生命周期，不宜隐式拉起另一个长期进程。
- 需要处理端口分配、健康检查、崩溃恢复和关闭顺序。
- REST API 契约需要额外确认，不如 CLI help 稳定直观。

后续如果发现频繁 fork CLI 性能不足，可以新增 `boxlitecli/server` 或独立 provider，通过显式配置启用。

## 分阶段落地

### 1. CLI spike

- 记录真实 `boxlite create/start/inspect/exec/rm` 的 stdout、stderr、exit code。
- 确认 `create` 成功后输出的是 ID、name 还是结构化数据。
- 确认 `inspect --format json` 的 schema。
- 确认 not found、command exit code、context kill 时的表现。
- 将样例输出固化为 `internal/sandbox/boxlitecli/testdata` fixture。

验证：

```bash
boxlite --home /tmp/csgclaw-boxlite-cli-spike create --name csgclaw-spike alpine
boxlite --home /tmp/csgclaw-boxlite-cli-spike start csgclaw-spike
boxlite --home /tmp/csgclaw-boxlite-cli-spike inspect --format json csgclaw-spike
boxlite --home /tmp/csgclaw-boxlite-cli-spike exec csgclaw-spike -- sh -lc 'echo ok'
boxlite --home /tmp/csgclaw-boxlite-cli-spike rm -f csgclaw-spike
```

### 2. 新增 `boxlitecli` adapter

- 实现 provider/runtime/instance。
- 实现 `runner` 和 `execRunner`。
- 实现 `CreateSpec` 参数转换。
- 实现 inspect JSON parser 和 state mapper。
- 实现 stderr/not found/exit code 错误归一。

测试重点：

```bash
go test ./internal/sandbox/boxlitecli
```

### 3. 接入配置和服务启动


- 增加一个 `boxlite_cli_path`，专门用于boxlite-cli类型的provider
- `internal/config` 增加 CLI provider 配置字段。
- `cli/serve` 的 `sandboxServiceOptions` 支持 `boxlite-cli`。
- `cli/onboard` 支持生成或保留 `[sandbox] provider = "boxlite-cli"`。
- 保持 `boxlite-sdk` Go SDK provider 可用，避免一次性切换默认行为。

测试重点：

```bash
go test ./internal/config ./cli/serve ./cli/onboard
```

### 4. Agent 生命周期回归

- 用 fake runner 覆盖 manager/worker 创建、启动、删除、命令执行流程。
- 真实 CLI 集成测试默认跳过，使用环境变量启用，例如 `CSGCLAW_BOXLITE_CLI_INTEGRATION=1`。
- 集成测试必须使用临时 `--home`，避免污染用户 BoxLite runtime。

建议命令：

```bash
go test ./internal/agent ./internal/bot ./internal/api
CSGCLAW_BOXLITE_CLI_INTEGRATION=1 go test ./internal/sandbox/boxlitecli
```

### 5. 文档和发布检查

- 更新 README 中 sandbox provider 说明。
- 说明 `boxlite-cli` 需要用户预先安装 `boxlite` binary。
- 说明 `boxlite_cli_path`、`BOXLITE_HOME` 和 CSGClaw sandbox home 的关系。
- 明确 `boxlite-cli` provider 不需要 vendored Go SDK 运行时，但当前仓库默认 provider 仍可能让 `make test` 触发 BoxLite SDK 构建链路。

最终验证：

```bash
make fmt
go test ./...
```

## 风险和待确认点

- `boxlite create` 是否支持 command/entrypoint：当前 help 未显示，若不支持，需要调整 agent 镜像启动方式或保持 Go SDK provider 作为需要 `Cmd` 的路径。
- `boxlite create` 成功输出格式未知：必须通过 spike 确认，不能依赖脆弱字符串解析。
- `boxlite inspect --format json` schema 未固化：必须用 fixture 覆盖。
- 非零 exit code 和 CLI 自身失败要区分：agent 内命令失败应返回 exit code，CLI 调用失败才是 adapter 错误。
- CLI fork 成本可能高：短期可接受，长期可评估 `boxlite serve`。
- 日志能力差异：当前 `sandbox.Instance` 没有 logs 接口，`boxlite logs` 可先不接入。
- 安全边界取决于外部 `boxlite` binary 版本：启动时应输出 provider 和 binary version，但不能打印 secrets。

## 验收标准

- `provider = "boxlite-cli"` 时，manager/worker agent 可以完成创建、启动、执行命令、删除。
- `internal/agent`、`internal/api`、`internal/bot` 不直接 import `os/exec` 或拼接 `boxlite` 命令。
- `rg -n "boxlite-cli|boxlitecli" internal cli docs` 能看到配置、adapter 和文档入口。
- fake runner 单测覆盖参数转换、状态解析、not found、exit code、unsupported option。
- 真实 CLI 集成测试可通过环境变量启用，默认不污染开发者机器。
