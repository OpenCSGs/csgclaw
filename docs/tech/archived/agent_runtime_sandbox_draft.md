# Agent / Runtime / Sandbox Draft

## Goal

CSGClaw currently treats most agent lifecycle operations as sandbox lifecycle operations. That works for sandbox-backed agents, but it does not fit:

- agents that run directly on the user's host
- agents that run on a remote machine or remote service
- multiple agents sharing one sandbox

This draft still proposes the same target model, but the implementation should be phased. The immediate goal is not to land the full runtime-sandbox-manager design in one step. The immediate goal is to make `Agent` depend on `Runtime`, so new runtime kinds can be added without teaching agent code about sandbox details.

This is especially useful for the next set of runtimes:

- sandboxed `picoclaw`
- sandboxed `openclaw`
- non-sandboxed `codex`

The longer-term goal is still the same execution model:

- `Agent` is the logical identity and configuration object
- `Runtime` is the execution unit that owns lifecycle
- `Sandbox` is an optional isolation and workspace resource
- `SandboxManager` allocates, reuses, and cleans up sandboxes for runtimes

The target relation is:

```text
Agent -> Runtime -> Sandbox?
```

Where:

- every `Agent` binds to exactly one `Runtime`
- a `Runtime` may bind to zero or one `Sandbox`
- a `Sandbox` may be dedicated to one runtime or shared by multiple runtimes

## Design Principles

1. Do not model all agents as sandboxes.
2. Do not let `Agent` directly depend on sandbox fields such as image, mounts, or box id.
3. Keep `Runtime` small and lifecycle-focused.
4. Treat `Sandbox` as an optional resource, not as the default execution abstraction.
5. Land runtime decoupling before sandbox-sharing machinery.
6. Support shared sandbox safely through an explicit manager, not ad hoc runtime logic.
7. Represent optional abilities such as `exec`, `inspect`, and `logs` as capabilities, not mandatory runtime methods.

## Layer Responsibilities

### Agent

`Agent` is the logical entity visible to users and APIs.

It owns:

- identity
- display metadata
- model profile
- role
- runtime binding
- capability view

It should not own:

- sandbox image
- mount definitions
- sandbox ids
- process ids
- provider-specific execution details

Suggested shape:

```go
type Agent struct {
    ID          string
    Name        string
    Description string
    Role        string
    Status      string
    CreatedAt   time.Time

    Profile     AgentProfile

    RuntimeID   string
    Capabilities AgentCapabilities
}
```

### Runtime

`Runtime` is the execution abstraction. It owns lifecycle and session semantics.

Examples:

- a BoxLite-backed runtime
- a host process runtime
- a remote SSH runtime
- a remote service runtime

`Runtime` may use a sandbox, but is not defined by sandbox existence.

It owns:

- create/start/stop/delete/state
- handle persistence
- optional sandbox binding
- optional session/process metadata

It should not own:

- global agent registry
- profile resolution rules
- sandbox reuse policy for the whole system

Suggested resource shape:

```go
type RuntimeRecord struct {
    ID          string
    Kind        string
    State       RuntimeState
    Sharing     RuntimeSharing
    SandboxID   string
    HandleID    string
    CreatedAt   time.Time
    Spec        RuntimeSpec
}
```

### Sandbox

`Sandbox` is an optional resource that provides isolation and workspace hosting.

Examples:

- BoxLite box
- container
- VM
- remote workspace boundary

It owns:

- low-level sandbox lifecycle
- sandbox metadata
- sandbox-scoped exec or file operations, if supported

It should not own:

- agent identity
- model profile
- cross-agent orchestration policy

### SandboxManager

`SandboxManager` is the allocation and reuse layer.

It exists because runtime lifecycle and sandbox lifecycle are not always one-to-one.

Examples:

- one runtime creates one dedicated sandbox
- multiple runtimes attach to one shared sandbox
- deleting one runtime should not delete a shared sandbox still used by others

It owns:

- create or attach decisions
- sandbox sharing policy
- ownership and reference tracking
- final cleanup policy

## Why SandboxManager Is Required

If shared sandbox support is a real requirement, `Runtime.Delete()` cannot always directly call `Sandbox.Delete()`.

For example:

- runtime A and runtime B both use sandbox S
- deleting runtime A must not remove sandbox S if runtime B still depends on it

Without a manager, this logic leaks into each runtime implementation and becomes inconsistent.

`SandboxManager` centralizes:

- whether to allocate a new sandbox
- whether to reuse an existing sandbox
- whether a sandbox is dedicated or shared
- when a sandbox becomes garbage

## Core Abstractions

### Runtime Core

The runtime core should only include operations that every runtime is expected to support.

```go
type Runtime interface {
    Kind() string

    Create(ctx context.Context, spec RuntimeSpec) (RuntimeHandle, error)
    Start(ctx context.Context, h RuntimeHandle) (RuntimeState, error)
    Stop(ctx context.Context, h RuntimeHandle) (RuntimeState, error)
    Delete(ctx context.Context, h RuntimeHandle) error
    State(ctx context.Context, h RuntimeHandle) (RuntimeState, error)
}
```

Suggested supporting types:

```go
type RuntimeHandle struct {
    RuntimeID string
    HandleID  string
}

type RuntimeState string

const (
    RuntimeStateUnknown RuntimeState = "unknown"
    RuntimeStateCreated RuntimeState = "created"
    RuntimeStateRunning RuntimeState = "running"
    RuntimeStateStopped RuntimeState = "stopped"
    RuntimeStateExited  RuntimeState = "exited"
    RuntimeStateFailed  RuntimeState = "failed"
)
```

### Optional Runtime Extensions

The runtime surface exposed to `Agent` should stay high-level.

`Agent` should not directly depend on low-level `inspect` or `exec` semantics.
Those are usually properties of the underlying sandbox, container, VM, or host
process environment, and should remain implementation details behind the
runtime boundary.

What `Agent` needs is higher-level behavior such as lifecycle, coarse state, and
possibly log streaming.

For example:

- a sandboxed PicoClaw runtime may implement log streaming by reading host log
  files and falling back to sandbox command execution internally
- a host process runtime may expose log streaming without any sandbox at all
- a remote service runtime may expose lifecycle and coarse state, but no logs

So optional runtime-facing extensions should stay at the same abstraction level
as `Agent` needs.

```go
type RuntimeLogStreamer interface {
    StreamLogs(ctx context.Context, h RuntimeHandle, opts LogOptions) error
}
```

Supporting types:

```go
type ExecSpec struct {
    Command []string
    Env     map[string]string
    WorkDir string
    Stdout  io.Writer
    Stderr  io.Writer
}

type ExecResult struct {
    ExitCode int
}

type LogOptions struct {
    Follow bool
    Tail   int
    Writer io.Writer
}
```

## Sandbox Abstractions

Sandbox abstractions should stay close to resource management and low-level execution.

```go
type Sandbox interface {
    Kind() string

    Create(ctx context.Context, spec SandboxSpec) (SandboxHandle, error)
    Start(ctx context.Context, h SandboxHandle) (SandboxState, error)
    Stop(ctx context.Context, h SandboxHandle, opts SandboxStopOptions) (SandboxState, error)
    Delete(ctx context.Context, h SandboxHandle, opts SandboxDeleteOptions) error
    State(ctx context.Context, h SandboxHandle) (SandboxState, error)
}
```

Optional capabilities:

```go
type SandboxInspector interface {
    Inspect(ctx context.Context, h SandboxHandle) (SandboxInfo, error)
}

type SandboxExecutor interface {
    Exec(ctx context.Context, h SandboxHandle, spec ExecSpec) (ExecResult, error)
}

type SandboxLogStreamer interface {
    Logs(ctx context.Context, h SandboxHandle, opts LogOptions) error
}
```

Suggested types:

```go
type SandboxHandle struct {
    SandboxID string
    HandleID  string
}

type SandboxState string

const (
    SandboxStateUnknown SandboxState = "unknown"
    SandboxStateCreated SandboxState = "created"
    SandboxStateRunning SandboxState = "running"
    SandboxStateStopped SandboxState = "stopped"
    SandboxStateExited  SandboxState = "exited"
    SandboxStateFailed  SandboxState = "failed"
)

type SandboxSpec struct {
    Name    string
    Image   string
    Env     map[string]string
    Mounts  []SandboxMount
    Shared  bool
    Labels  map[string]string
}

type SandboxMount struct {
    HostPath  string
    GuestPath string
    ReadOnly  bool
}

type SandboxInfo struct {
    SandboxID string
    HandleID  string
    State     SandboxState
    CreatedAt time.Time
    Metadata  map[string]string
}

type SandboxStopOptions struct {
    Force bool
}

type SandboxDeleteOptions struct {
    Force bool
}
```

## SandboxManager Abstraction

`SandboxManager` should be the only component deciding sandbox allocation and cleanup for runtimes.

```go
type SandboxManager interface {
    Bind(ctx context.Context, req SandboxBindingRequest) (SandboxBinding, error)
    Release(ctx context.Context, binding SandboxBinding) error
}
```

Suggested types:

```go
type SandboxBindingRequest struct {
    RuntimeID string
    RuntimeKind string
    Policy    SandboxPolicy
    Spec      SandboxSpec
}

type SandboxBinding struct {
    RuntimeID  string
    SandboxID  string
    Dedicated  bool
    Reused     bool
}

type SandboxPolicy struct {
    Mode      SandboxBindingMode
    ShareKey  string
    OwnerID   string
}

type SandboxBindingMode string

const (
    SandboxBindingNone      SandboxBindingMode = "none"
    SandboxBindingDedicated SandboxBindingMode = "dedicated"
    SandboxBindingShared    SandboxBindingMode = "shared"
)
```

Rules:

- `none`: runtime does not need a sandbox
- `dedicated`: runtime gets an isolated sandbox
- `shared`: runtime attaches to an existing sandbox selected by `ShareKey`, or creates one if missing

`Release()` may:

- do nothing for a still-referenced shared sandbox
- delete the sandbox if this runtime was the last reference
- only detach the runtime from the sandbox mapping

## Recommended Runtime Kinds

Short-term kinds:

- `picoclaw_sandbox`
- `openclaw_sandbox`
- `codex`

Medium-term or long-term kinds:

- `host_process`
- `remote_service`
- `remote_ssh`
- `sandbox_vm`
- `embedded_session`

For the near-term rollout, treating `codex` as a runtime kind is acceptable if it keeps the refactor small. If the model later becomes too overloaded, `codex` can be split into profile/provider concerns plus a lower-level execution runtime.

## Shared Sandbox Semantics

This draft recommends supporting only one shared model first:

- shared filesystem and environment boundary
- independent runtime handles and agent identities

That means:

- multiple runtimes may point to one sandbox
- each runtime still has its own runtime handle and lifecycle state
- sandbox-level `exec/logs` exist only if meaningful for the underlying sandbox implementation

This draft does not recommend supporting the more complex model first:

- multiple logical agents sharing one primary process tree with no clear runtime boundary

That model complicates:

- state reporting
- log ownership
- exec target selection
- cleanup semantics

## Data Model Changes

Current sandbox-shaped fields should be replaced with runtime-shaped fields.

Recommended changes:

- replace `agent.box_id` with `agent.runtime_id`
- remove top-level `agent.image`
- store runtime-specific config in runtime records
- move sandbox ids to runtime records
- expose runtime and sandbox capabilities separately if needed

Suggested records:

```go
type AgentRecord struct {
    ID          string
    Name        string
    Description string
    Role        string
    Status      string
    CreatedAt   time.Time
    Profile     AgentProfile
    RuntimeID   string
}

type RuntimeRecord struct {
    ID          string
    Kind        string
    State       RuntimeState
    AgentIDs    []string
    SandboxID   string
    Sharing     RuntimeSharing
    Spec        RuntimeSpec
    CreatedAt   time.Time
}
```

Suggested runtime sharing values:

```go
type RuntimeSharing string

const (
    RuntimeSharingDedicated RuntimeSharing = "dedicated"
    RuntimeSharingShared    RuntimeSharing = "shared"
    RuntimeSharingNone      RuntimeSharing = "none"
)
```

Suggested runtime spec:

```go
type RuntimeSpec struct {
    Kind          string
    SandboxPolicy SandboxPolicy

    HostProcess *HostProcessSpec
    BoxLite     *BoxLiteRuntimeSpec
    Remote      *RemoteRuntimeSpec
}

type HostProcessSpec struct {
    Command []string
    WorkDir string
    Env     map[string]string
}

type BoxLiteRuntimeSpec struct {
    Image  string
    Env    map[string]string
    Mounts []SandboxMount
}

type RemoteRuntimeSpec struct {
    Endpoint string
    Headers  map[string]string
}
```

## API and Service Implications

The current `agent.Service` mixes:

- registry
- profile defaulting
- runtime orchestration
- sandbox orchestration

That should be split.

Recommended service split:

- `agent.Service`
  - owns agent CRUD
  - owns profile resolution and persistence
- `agentruntime.Service`
  - owns runtime CRUD and lifecycle
  - dispatches to runtime implementations
- `sandbox.Service`
  - owns sandbox provider interaction
- `sandbox.Manager`
  - owns runtime-to-sandbox binding policy

Then high-level agent operations become:

1. resolve or create the target runtime
2. bind sandbox if required
3. start or stop runtime
4. update agent status from runtime state

## Example Flows

### Host Agent Without Sandbox

```text
Create Agent
  -> create Runtime(kind=host_process, sandbox mode=none)
  -> runtime stores local process spec
  -> no sandbox created
```

### Sandbox Worker With Dedicated Sandbox

```text
Create Agent
  -> create Runtime(kind=sandbox_boxlite, sandbox mode=dedicated)
  -> runtime asks SandboxManager for dedicated sandbox
  -> SandboxManager creates sandbox
  -> runtime stores sandbox binding
```

### Two Agents Sharing One Sandbox

```text
Create Agent A
  -> create Runtime A(kind=host_process or sandbox_boxlite, sandbox mode=shared, share key=X)
  -> SandboxManager creates sandbox S for X

Create Agent B
  -> create Runtime B(kind=host_process or sandbox_boxlite, sandbox mode=shared, share key=X)
  -> SandboxManager reuses sandbox S for X

Delete Agent A
  -> delete Runtime A
  -> SandboxManager releases binding A -> S
  -> sandbox S remains because Runtime B still references it
```

## Compatibility With Existing Code

Current `internal/sandbox` can become the first concrete sandbox implementation.

That means:

- current `sandbox.Provider` and `sandbox.Runtime` should be repositioned as sandbox-layer abstractions
- current `box` or `instance` ids become `SandboxID` or sandbox handles
- current `agent.Service` should stop caching `sandbox.Runtime` directly

Practical interpretation:

- existing BoxLite provider becomes a `Sandbox`
- a new BoxLite-based `Runtime` implementation depends on that sandbox implementation
- host and remote runtimes can be added without pretending they are sandboxes

## Phased Implementation Plan

### Short-Term

Goal: insert a `Runtime` layer first and stop letting agent code depend directly on sandbox concepts.

This phase should be treated as a refactor guide, not just a target architecture note.

Desired end state for this phase:

- `Agent` depends on `Runtime`, not on sandbox details
- runtime lifecycle is orchestrated through a runtime-oriented service
- current PicoClaw behavior remains intact, but its implementation moves behind a runtime boundary
- new runtime kinds can be introduced without reworking `agent.Service` again

In-scope changes:

- add `Runtime` interface and `RuntimeRecord`
- make `Agent` store `runtime_id` instead of directly depending on `box_id`
- move agent lifecycle orchestration to a runtime-oriented service
- move PicoClaw-specific bootstrap and startup logic behind a `picoclaw_sandbox` runtime implementation
- define runtime-facing capability views for API and IM integration points
- keep one-agent-to-one-runtime semantics

Non-goals for this phase:

- no shared sandbox support yet
- no standalone `SandboxManager` requirement in production code yet
- no full unification of sandbox lifecycle logic across runtime kinds yet
- no change to `/api/bots/*` compatibility behavior or IM bridge behavior

Near-term runtime targets after the extraction lands:

- `picoclaw_sandbox`
- `openclaw_sandbox`
- `codex`

#### Why the first extraction must be wider than `box_id -> runtime_id`

The short-term plan cannot treat current PicoClaw coupling as only a `box_id` or sandbox-lifecycle problem. The existing behavior is spread across both runtime-specific boot contracts and higher-level bot integration assumptions, and the first runtime extraction has to pull the runtime concerns under a clear boundary as well.

At minimum, the runtime-facing refactor must account for:

- `internal/agent/service.go`: `agent.Service` currently caches `sandbox.Runtime`, stores sandbox-oriented state, and owns manager/worker lifecycle decisions that assume a BoxLite-backed PicoClaw gateway.
- `internal/agent/box.go`: create spec generation is PicoClaw-specific today, including `picoclaw` gateway startup commands, `/home/picoclaw` paths, workspace/project mounts, and `PICOCLAW_*` environment variables.
- `internal/agent/manager_config.go`: runtime bootstrap currently depends on rendering PicoClaw config files and security files into a PicoClaw-specific workspace layout.
- `internal/api/bot_compat.go` and `internal/im/bot_bridge.go`: these layers are not runtime implementations, but they currently depend on assumptions shaped by the existing PicoClaw-backed flow. The runtime abstraction should not absorb them. Instead it should expose the runtime or bot state, capabilities, and reconnect semantics those layers need.
- tests and embedded runtime assets under `internal/agent/embed/runtimes/picoclaw/...`: they encode the current PicoClaw boot contract and should be treated as part of the existing runtime behavior, not as incidental implementation detail.

That means the first runtime layer should own more than create/start/stop/delete wrappers. It should become the boundary for:

- runtime bootstrap and config materialization
- runtime-specific environment and mount preparation
- runtime command and startup contract
- runtime-agnostic state and log views consumed by compatibility APIs and IM bridges

If this boundary is too thin, `agent.Service` will keep PicoClaw-specific branches and the refactor will not actually unlock later runtime introduction.

#### Recommended execution order

##### Step 1: Introduce the runtime model without changing behavior

Deliverables:

- add `RuntimeRecord`
- make `Agent` reference `runtime_id`
- introduce a runtime-oriented service boundary that `agent.Service` can call

Exit criteria:

- agent persistence and in-memory state can identify a runtime independently from a sandbox handle
- existing agent flows still work with the current PicoClaw-backed behavior

##### Step 2: Move existing PicoClaw boot logic behind `picoclaw_sandbox`

Deliverables:

- move current PicoClaw create-spec generation behind the runtime implementation
- move config rendering and security file materialization behind the runtime implementation
- move runtime-specific env vars, mounts, and startup command assembly behind the runtime implementation

Exit criteria:

- `agent.Service` no longer needs to know `/home/picoclaw`, `PICOCLAW_*`, or PicoClaw-specific startup command details
- the runtime implementation fully owns the current PicoClaw boot contract

##### Step 3: Remove bot-compat recovery assumptions from runtime semantics

Deliverables:

- keep `/api/bots/*` compatibility behavior unchanged
- keep IM bridge behavior unchanged
- replace implicit current-runtime assumptions in those layers with runtime-agnostic state and optional log interfaces
- move bot delivery recovery decisions into compatibility or service policy instead of treating them as runtime behavior
- keep `Recreate` as a service-level orchestration built from runtime primitives rather than adding it to the base `Runtime` interface

Exit criteria:

- compatibility APIs and IM bridges depend only on declared runtime state views, optional runtime log streaming, and explicit upper-layer recovery policy
- `bot reconnect` is treated as a bot compatibility or IM delivery concern, not as an agent or runtime semantic
- non-PicoClaw runtimes do not inherit PicoClaw-style reconnect or recreate assumptions accidentally

##### Step 4: Add new runtime kinds on the extracted abstraction

Candidates:

- `openclaw_sandbox`
- `codex`

Entry condition:

- Steps 1 through 3 are complete first

Reason:

- otherwise new runtime introduction will force another round of agent-service refactoring

#### Practical implementation rule

For this phase, sandboxed runtimes may continue calling current sandbox logic internally. The important constraint is that agent-facing code must only know `Runtime`, while sandbox binding remains an internal concern of concrete runtime implementations.

### Medium-Term

Goal: separate runtime concerns from sandbox concerns without changing the agent-facing model again.

Scope:

- introduce a dedicated sandbox abstraction under runtime
- move sandbox create/start/stop/delete logic out of concrete runtime implementations where appropriate
- add persistence migration rules for `box_id` to `runtime_id`
- standardize capability reporting such as `exec`, `logs`, and `inspect`

Expected outcome:

- agents still see only runtime
- runtimes may depend on sandbox through a cleaner internal contract
- sandboxed `picoclaw` and `openclaw` share more infrastructure

### Long-Term

Goal: support sandbox reuse, shared lifecycle policy, and more advanced execution models.

Scope:

- introduce `SandboxManager`
- implement shared sandbox allocation and reference tracking
- define share-key rules and cleanup semantics
- evaluate whether `codex` should remain a first-class runtime kind or split into provider plus execution layers
- add more runtime kinds such as `host_process`, `remote_service`, or `remote_ssh` as needed

Expected outcome:

- runtime-to-sandbox becomes many-to-one when needed
- sandbox cleanup is policy-driven instead of embedded in runtime deletion
- the system can support dedicated sandbox, shared sandbox, and no-sandbox runtimes consistently

## Open Questions

1. Is one agent always bound to one runtime, or do we need future support for one agent switching runtimes across sessions?
2. Should shared sandbox selection be explicit in config, or inferred from project/workspace identity?
3. Does a shared sandbox imply shared filesystem only, or shared process space too?
4. Do we need a separate `Session` abstraction above runtime for conversational tools such as Codex?
5. Which runtime capabilities must be exposed in API responses so the UI can hide unsupported actions?

## Recommendation

Adopt the architecture in stages, not all at once.

Short-term recommendation:

- make `Agent` depend only on `Runtime`
- add runtime kinds for sandboxed `picoclaw`, sandboxed `openclaw`, and non-sandboxed `codex`
- keep sandbox interaction behind runtime implementations for now

Medium-term recommendation:

- extract a dedicated sandbox abstraction once at least two sandboxed runtimes need to share behavior
- migrate persistence and capability reporting onto the runtime model

Long-term recommendation:

- add `SandboxManager` only when shared sandbox and lifecycle reuse become real product requirements

The main architectural rule should remain:

`Agent` depends on `Runtime`; `Runtime` may depend on `Sandbox`; sandbox sharing and cleanup move to `SandboxManager` only when the system is ready for that complexity.
