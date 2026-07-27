# Agent Sandbox Abstraction Design

## 背景

当前 agent box 生命周期直接绑定 BoxLite。主要耦合点集中在 `internal/agent`：

- `Service.runtimes map[string]*boxlite.Runtime` 缓存 BoxLite runtime。
- `runtime.go` 直接创建、关闭、查询 `boxlite.Runtime` 和 `boxlite.Box`。
- `box.go` 直接组装 `boxlite.BoxOption`，并创建 gateway box。
- `service.go` 在创建、删除、启动 manager、查看日志等业务流程里直接判断 `boxlite.IsNotFound`、读取 `boxlite.BoxInfo`、执行 `box.Command(...)`。
- API、bot、agent 测试为了绕过真实 runtime，也直接引用了 `boxlite.Runtime`、`boxlite.Box`、`boxlite.BoxInfo`。

这导致 BoxLite 类型从底层实现泄漏到了 agent service 和测试边界。若后续要切换 Docker、Firecracker、Apple Container、远程 sandbox 服务或其他运行时，需要改动业务流程、测试 hook、配置和构建链路，替换成本较高。

## 目标

抽象目标不是隐藏“agent 需要一个可执行隔离环境”这个事实，而是把具体 sandbox 实现隔离在 adapter 内：

- agent 业务层只依赖稳定的 sandbox 接口和数据结构。
- BoxLite 作为第一个 adapter 保持现有行为不变。
- 创建、启动、删除、查询、执行命令、查看日志这些操作都通过统一接口完成。
- 不把 `boxlite` 类型暴露到 `internal/api`、`internal/bot`、`internal/agent/service.go` 的业务逻辑和测试里。
- 保持现有 agent state 兼容，短期继续使用 `BoxID` 字段对外兼容，内部逐步引入更通用的 sandbox 字段。

## 非目标

- 第一阶段不同时实现多个 sandbox 后端。
- 第一阶段不改变 API 返回字段语义，`box_id` 继续保留。
- 第一阶段不重构 agent、bot、IM 的业务边界。
- 第一阶段不改 `third_party/boxlite-go`。

## 建议包结构

新增 `internal/sandbox` 作为业务无关的 sandbox 抽象层：

```text
internal/sandbox/
  sandbox.go            # 接口、通用类型、错误定义
  spec.go               # CreateSpec、Mount、CommandSpec 等结构
  boxlite/
    adapter.go          # BoxLite Provider/Runtime/Instance 实现
    options.go          # 通用 spec 到 boxlite option 的转换
```

`internal/agent` 只引用 `internal/sandbox`。BoxLite adapter 引用 vendored SDK：

```text
internal/agent  ─────► internal/sandbox
                         ▲
                         │
                  internal/sandbox/boxlitesdk ─────► third_party/boxlite-go
```

后续新增 Docker 或远程 sandbox 时，只增加新的 adapter 包，不改 agent service 主流程。

## 核心接口

建议使用三层概念：

- `Provider`：根据 home dir 创建或打开一个 runtime。
- `Runtime`：管理某个 agent home 下的 sandbox 实例。
- `Instance`：代表一个可启动、可查询、可执行命令的 box/container。

示例接口：

```go
package sandbox

import (
	"context"
	"io"
	"time"
)

type Provider interface {
	Open(ctx context.Context, homeDir string) (Runtime, error)
	Name() string
}

type Runtime interface {
	Create(ctx context.Context, spec CreateSpec) (Instance, error)
	Get(ctx context.Context, idOrName string) (Instance, error)
	Remove(ctx context.Context, idOrName string, opts RemoveOptions) error
	Close() error
}

type Instance interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context, opts StopOptions) error
	Info(ctx context.Context) (Info, error)
	Run(ctx context.Context, spec CommandSpec) (CommandResult, error)
	Close() error
}

type Info struct {
	ID        string
	Name      string
	State     State
	CreatedAt time.Time
}

type State string

const (
	StateUnknown State = "unknown"
	StateCreated State = "created"
	StateRunning State = "running"
	StateStopped State = "stopped"
	StateExited  State = "exited"
)

type CreateSpec struct {
	Image      string
	Name       string
	Detach     bool
	AutoRemove bool
	Env        map[string]string
	Mounts     []Mount
	Entrypoint []string
	Cmd        []string
}

type Mount struct {
	HostPath  string
	GuestPath string
	ReadOnly  bool
}

type CommandSpec struct {
	Name   string
	Args   []string
	Stdout io.Writer
	Stderr io.Writer
}

type CommandResult struct {
	ExitCode int
}

type StopOptions struct {
	Timeout time.Duration
	Force   bool
}

type RemoveOptions struct {
	Force bool
}
```

生命周期语义必须保持清晰：

- `Runtime.Close`：释放当前进程持有的 runtime/client/SDK 资源，不停止、不删除任何实例。
- `Instance.Close`：释放当前进程持有的 instance/box handle，不停止、不删除实例。
- `Instance.Stop`：停止运行中的实例，保留实例元数据、文件系统和可再次启动的状态。
- `Runtime.Remove`：删除实例；`RemoveOptions.Force=true` 表示必要时强制停止并删除。

也就是说，`Close` 是资源释放操作，`Stop` 和 `Remove` 才是 sandbox 生命周期操作。

错误语义必须统一，避免业务层继续依赖某个 SDK 的错误类型：

```go
var ErrNotFound = errors.New("sandbox not found")

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}
```

BoxLite adapter 将 `boxlite.IsNotFound(err)` 包装为 `sandbox.ErrNotFound`。

## 新旧接口对应关系

后续改代码时应按下表迁移，避免把 `Close`、`Stop`、`Remove` 的语义混用。

| 新抽象 | 语义 | 当前 `internal/agent` 入口 | 当前 BoxLite SDK 对应 |
|---|---|---|---|
| `Provider.Name()` | 返回 sandbox 后端名称，例如 `boxlite-sdk` | 新增，无现有对应 | adapter 固定返回 `"boxlite-sdk"` |
| `Provider.Open(ctx, homeDir)` | 打开或初始化 runtime 管理上下文，不创建 box | `ensureRuntimeAtHome(homeDir)` | `boxlite.NewRuntime(boxlite.WithHomeDir(homeDir))` |
| `Runtime.Create(ctx, spec)` | 创建实例，但是否启动由 adapter/spec 语义决定；建议业务层仍显式调用 `Start` | `createBox(ctx, rt, image, opts...)`、`createGatewayBox` 内的 `rt.Create` | `rt.Create(ctx, image, opts...)` |
| `Runtime.Get(ctx, idOrName)` | 按 ID 或名称获取已有实例 handle | `getBox(ctx, rt, idOrName)`、`resolveAgentBox` | `rt.Get(ctx, idOrName)` |
| `Runtime.Remove(ctx, idOrName, opts)` | 删除实例；`Force=true` 表示强制删除 | `forceRemoveBox(ctx, rt, idOrName)` | `rt.ForceRemove(ctx, idOrName)` |
| `Runtime.Close()` | 释放 runtime/client 资源，不停止、不删除实例 | `closeRuntime(homeDir, rt)`、`Service.Close` | `rt.Close()` |
| `Instance.Start(ctx)` | 启动已创建或已存在的实例 | `startBox(ctx, box)` | `box.Start(ctx)` |
| `Instance.Stop(ctx, opts)` | 停止实例但保留实例；不能映射为 handle close | 新增，当前业务尚未显式使用 | 若 BoxLite SDK 有 stop 则映射；否则 adapter 返回 unsupported 或实现等价停止流程 |
| `Instance.Info(ctx)` | 读取实例 ID、状态、创建时间等通用信息 | `boxInfo(ctx, box)`、`refreshAgentBoxID` | `box.Info(ctx)` |
| `Instance.Run(ctx, spec)` | 在实例内执行命令并返回 exit code | `runBoxCommand(ctx, box, name, args, w)` | `box.Command(name, args...).Run(ctx)` + `ExitCode()` |
| `Instance.Close()` | 释放 instance/box handle，不停止、不删除实例 | `closeBox(box)` | `box.Close()` |
| `sandbox.IsNotFound(err)` | 统一 not found 判断 | `boxlite.IsNotFound(err)` | adapter 将 BoxLite not found 包装为 `sandbox.ErrNotFound` |

当前 `createGatewayBox` 同时做了 create、start、info 三步：

```go
box, err := rt.Create(ctx, image, boxOpts...)
if err := box.Start(ctx); err != nil { ... }
info, err := box.Info(ctx)
```

迁移后应保持这个流程，只是类型替换为通用接口：

```go
instance, err := rt.Create(ctx, spec)
if err := instance.Start(ctx); err != nil { ... }
info, err := instance.Info(ctx)
```

当前普通 `Create` 创建 agent 时只调用了 `createBox`，没有显式 `Start` 和 `Info`。迁移时需要单独确认这是现有语义还是遗漏：如果 BoxLite 在 `WithDetach(true)` 下创建即启动，则 adapter 必须明确保持该行为；如果目标语义是“创建后运行”，建议改成和 gateway 一样显式 `Start` 再 `Info`，并用返回的 `Info` 写入 `BoxID` 和 `Status`。

## Agent 层职责调整

`internal/agent` 应保留这些职责：

- 解析 agent 名称、ID、角色、模型 profile。
- 计算 agent home、workspace、projects 目录。
- 组装 PicoClaw gateway 运行规格。
- 保存 agent 元数据。
- 决定 manager/worker 创建、删除、日志读取的业务流程。

`internal/agent` 不应关心：

- `boxlite.Runtime` 是否有效。
- `boxlite.BoxOption` 如何构造。
- SDK 的 not found 错误类型。
- SDK command API 如何绑定 stdout/stderr。

## Gateway Box 规格

把当前 `gatewayBoxOptions` 拆成“业务规格”和“adapter 转换”两层。

`internal/agent` 生成通用 `sandbox.CreateSpec`：

```go
spec := sandbox.CreateSpec{
	Image:      image,
	Name:       name,
	Detach:     true,
	AutoRemove: false,
	Env:        envVars,
	Cmd: []string{
		"/bin/sh",
		"-c",
		"/usr/local/bin/picoclaw gateway -d 1>~/.picoclaw/gateway.log 2>/dev/null",
	},
	Mounts: []sandbox.Mount{
		{HostPath: hostWorkspaceRoot, GuestPath: boxWorkspaceDir},
		{HostPath: projectsRoot, GuestPath: boxProjectsDir},
	},
}
```

BoxLite adapter 负责把它转换为：

- `boxlite.WithName(spec.Name)`
- `boxlite.WithDetach(spec.Detach)`
- `boxlite.WithAutoRemove(spec.AutoRemove)`
- `boxlite.WithEnv(k, v)`
- `boxlite.WithVolume(host, guest)`
- `boxlite.WithCmd(spec.Cmd...)`

如果其他后端不支持某个字段，adapter 应返回明确错误，例如 `unsupported sandbox option: read-only mount`，而不是静默忽略。

## 日志抽象

当前 `StreamLogs` 通过在 box 内执行：

```text
tail -n <lines> [-f] /home/picoclaw/.picoclaw/gateway.log
```

短期建议保留这一实现，但通过 `Instance.Run` 完成，业务层不直接碰 SDK command API。

中期可在 `sandbox` 增加可选接口：

```go
type LogStreamer interface {
	StreamLogs(ctx context.Context, idOrName string, spec LogSpec, w io.Writer) error
}
```

`agent.Service.StreamLogs` 的策略：

1. 如果 runtime 实现 `LogStreamer`，调用原生日志能力。
2. 否则 fallback 到 `tail` 命令。

这样 Docker 之类的后端可以直接映射到 `docker logs`，BoxLite 仍可沿用当前文件日志方式。

## Agent 元数据兼容

现有 `Agent` 字段：

```go
BoxID string `json:"box_id,omitempty"`
```

建议第一阶段不删除，避免 API 和 state 破坏性变更。内部可以新增字段：

```go
SandboxProvider string `json:"sandbox_provider,omitempty"`
SandboxID       string `json:"sandbox_id,omitempty"`
```

兼容策略：

- 创建新 agent 时同时写入 `BoxID` 和 `SandboxID`，两者初始相同。
- 对外 API 继续返回 `box_id`。
- 读取旧 state 时，如果 `SandboxID` 为空且 `BoxID` 不为空，则使用 `BoxID`。
- 删除、查询、日志读取时优先使用 `SandboxID`，fallback 到 `BoxID` 和 agent name。
- 后续大版本再考虑废弃 `BoxID`。

如果希望第一阶段更小，也可以先只保留 `BoxID`，等 adapter 切换稳定后再做字段泛化。

## 配置设计

新增 sandbox 配置，默认保持 BoxLite：

```go
type SandboxConfig struct {
	Provider string
	HomeDirName string
}
```

配置文件示例：

```toml
[sandbox]
provider = "boxlite-sdk"
home_dir_name = "boxlite"
```

短期也可以先不暴露用户配置，在 `agent.NewService...` 内注入默认 provider。建议最终落到配置层，因为切换底层 sandbox 是用户可感知能力。

注意：现有 `config.RuntimeHomeDirName = "boxlite"` 已经把 runtime home 命名绑定到 BoxLite。引入抽象后应迁移为：

- `config.DefaultSandboxProvider = "boxlite-sdk"`
- `config.DefaultSandboxHomeDirName = "boxlite"`
- `boxRuntimeHome` 改名为 `sandboxRuntimeHome`。

## Service 注入方式

`agent.Service` 从缓存具体 BoxLite runtime 改为缓存通用 runtime：

```go
type Service struct {
	sandboxProvider sandbox.Provider
	runtimes map[string]sandbox.Runtime
}
```

构造函数默认注入 BoxLite provider：

```go
provider := boxlitesdk.NewProvider()
```

为了降低改动面，可以增加内部构造选项：

```go
type ServiceOption func(*Service)

func WithSandboxProvider(provider sandbox.Provider) ServiceOption
```

测试里使用 fake provider/fake runtime/fake instance，不再使用 `boxlite.Runtime` 和 test hook。

## 迁移步骤拆解

### 1. 建立抽象包

- 新增 `internal/sandbox`。
- 定义 `Provider`、`Runtime`、`Instance`、`Info`、`CreateSpec`、`CommandSpec`、`ErrNotFound`。
- 不改现有业务逻辑，只让新包先编译通过。

验证：

```bash
go test ./internal/sandbox
```

### 2. 实现 BoxLite adapter

- 新增 `internal/sandbox/boxlitesdk`。
- 将 `boxlite.Runtime` 包装为 `Runtime`。
- 将 `boxlite.Box` 包装为 `Instance`。
- 将通用 `CreateSpec` 转换为 `boxlite.BoxOption`。
- 将 `Runtime.Remove(..., RemoveOptions{Force: true})` 映射到 `boxlite.Runtime.ForceRemove`。
- 将 `Instance.Stop` 映射到 BoxLite 可用的停止能力；如果 SDK 暂无显式 stop，应返回明确的 unsupported 错误或在 adapter 内实现等价流程，不能把 `Close` 当作 `Stop`。
- 将 `boxlite.IsNotFound` 转换为 `sandbox.ErrNotFound`。
- 映射 BoxLite state 到 `sandbox.State`。

验证：

```bash
go test ./internal/sandbox/boxlitesdk
```

### 3. 改造 agent runtime 边界

- `Service.runtimes` 改为 `map[string]sandbox.Runtime`。
- `ensureRuntime`、`ensureRuntimeAtHome` 返回 `sandbox.Runtime`。
- `getBox`、`startBox`、`boxInfo`、`createBox`、`runBoxCommand`、`closeBox` 使用 sandbox 接口。
- `forceRemoveBox` 改为调用 `Runtime.Remove(ctx, idOrName, sandbox.RemoveOptions{Force: true})`。
- `runtimeValid` 删除或仅留在 BoxLite adapter 内部。
- `boxRuntimeHome` 改名为 `sandboxRuntimeHome`，先保持路径不变。

验证：

```bash
go test ./internal/agent
```

### 4. 改造 gateway 创建

- `gatewayBoxOptions` 改为 `gatewayCreateSpec`。
- `createGatewayBox` 调用 `rt.Create(ctx, spec)`，再 `Start`、`Info`。
- manager 和 worker 创建流程只读取 `sandbox.Info`。
- 错误文案从 `boxlite` 调整为 `sandbox`，但用户可见语义保持一致。

验证：

```bash
go test ./internal/agent ./internal/bot ./internal/api
```

### 5. 替换测试 hook

- 删除或弱化 `SetTestHooks` 中的 BoxLite 类型签名。
- 用 fake sandbox provider 替代 test hook。
- API 和 bot 测试不再 import `github.com/RussellLuo/boxlite/sdks/go`。
- 保留少量 adapter 单测覆盖 BoxLite 转换逻辑。

验证：

```bash
go test ./internal/agent ./internal/bot ./internal/api
```

### 6. 引入配置

- 在 `internal/config.Config` 增加 `Sandbox SandboxConfig`。
- loader/saver 支持 `[sandbox]`。
- 默认 provider 为 `boxlite-sdk`。
- onboard 和 serve 创建 agent service 时传入 provider。
- 文档更新 `README.md`、`docs/tech/architecture.md`、`docs/README.go.md`。

验证：

```bash
go test ./internal/config ./cli ./internal/api
```

### 7. 收敛 BoxLite 构建依赖

当 agent service 不再直接引用 BoxLite 后，BoxLite 依赖应只出现在：

- `internal/sandbox/boxlitesdk`
- `third_party/boxlite-go`
- build/setup 相关 Makefile 目标

可用以下命令检查：

```bash
rg -n "boxlite|BoxLite" internal cmd cli
```

预期业务层只出现文案或 adapter import，不出现 `boxlite.Runtime`、`boxlite.Box`。

### 8. 增加第二个 sandbox adapter

选择一个最小后端验证抽象，例如：

- `noop` adapter：只用于测试，不执行真实 sandbox。
- `process` adapter：本机进程隔离很弱，但可验证接口。
- Docker adapter：功能更接近真实 sandbox，但引入外部 daemon 依赖。

建议先做 `noop` 或 `process`，确认 agent service 已不依赖 BoxLite 特性，再做 Docker。

## 推荐实施顺序

最稳妥的实际 PR 拆分：

1. PR 1：新增 `internal/sandbox` 接口和 fake 实现，仅加测试。
2. PR 2：新增 BoxLite adapter，保持业务层不变。
3. PR 3：`internal/agent/runtime.go` 切换到 `sandbox.Runtime`。
4. PR 4：`internal/agent/box.go` 切换到 `sandbox.CreateSpec`。
5. PR 5：替换 `internal/api`、`internal/bot`、`internal/agent` 测试里的 BoxLite hook。
6. PR 6：新增 `[sandbox]` 配置、文档和 onboard/serve 接线。
7. PR 7：增加第二个 adapter 或 noop adapter 作为抽象验收。

每个 PR 都应能独立通过 targeted tests，避免一次性重构导致 manager/worker 创建和日志流同时回归。

## 风险与注意事项

- **日志能力差异**：不是所有 sandbox 都支持在实例内执行 `tail -f`，需要 optional `LogStreamer` 扩展点。
- **挂载语义差异**：BoxLite 的 volume、Docker bind mount、远程 sandbox 文件同步语义可能不同，adapter 必须显式报错不支持的选项。
- **状态命名差异**：不同后端状态值不一致，业务层只应依赖通用 `sandbox.State`。
- **实例 ID 兼容**：旧 state 只有 `BoxID`，迁移时必须 fallback，避免已有 manager/worker 无法删除或查看日志。
- **runtime home 语义**：BoxLite 使用 per-agent home，其他后端可能不需要。抽象层仍保留 `homeDir`，adapter 可选择忽略或用于本地元数据。
- **构建依赖**：只要默认 provider 仍是 BoxLite，`make test`/`make build` 仍可能需要 `boxlite-setup`。如果未来希望无 BoxLite 构建，需要 build tags 或 provider 插件化。
- **安全边界**：不同 sandbox 的隔离级别不同。配置文档必须说明 provider 的安全假设，不能把弱隔离后端宣传成等价替代。

## 验收标准

完成抽象后应满足：

- `internal/agent/service.go` 不 import BoxLite SDK。
- `internal/api` 和 `internal/bot` 测试不 import BoxLite SDK。
- `rg -n "boxlite\\.Runtime|boxlite\\.Box|boxlite\\.BoxInfo|boxlite\\.BoxOption" internal/agent internal/api internal/bot` 只在 BoxLite adapter 或迁移兼容测试中命中。
- manager bootstrap、worker create、agent delete、agent logs 行为保持不变。
- 旧 `~/.csgclaw/agents/state.json` 中只有 `box_id` 的 agent 仍可被查找、删除和查看日志。
- 新增 fake/noop provider 测试能在不加载 BoxLite native library 的情况下覆盖 agent service 主流程。
