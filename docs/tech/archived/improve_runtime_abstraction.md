# Improve Runtime Abstraction

## Goal

Reduce concrete runtime knowledge inside `internal/agent`, especially PicoClaw/OpenClaw sandbox details, so `agent.Service` coordinates lifecycle through stable runtime capabilities instead of:

- runtime-specific helper branches
- runtime-family checks like `isGatewayRuntimeKind(...)`
- ad hoc type assertions
- duplicated host/config/workspace logic in `internal/agent`

This document focuses on current leakage points, then recommends a refactor that moves those details behind runtime-owned interfaces.

## Current Leakage Points In `internal/agent`

### 1. Runtime family classification is embedded in agent domain logic

Files:

- `internal/agent/runtime_record.go`
- `internal/agent/service.go`
- `internal/agent/service_profiles.go`
- `internal/agent/store.go`

Symptoms:

- `runtimeKindForAgent`, `isGatewayRuntimeKind`, `runtimeKindForGatewayRuntime`, `managerImageForRuntimeKind` encode runtime taxonomy in `internal/agent`.
- `runtimeForAgent` has special routing for manager / worker / gateway runtime combinations.
- `syncRuntimeRecordLocked`, `startupAgentCandidates`, `ensureGatewayConfigForAgent`, `profileForCreateRequest`, `CreateWorker`, `createNew`, `replace` all branch on gateway-vs-non-gateway semantics.
- `normalizeLoadedAgent` hardcodes manager runtime fallback to `RuntimeKindPicoClawSandbox`.

Problem:

`internal/agent` is currently deciding runtime *family semantics*, not just selecting a runtime by kind. That makes every new runtime kind more invasive, because behavior is spread across many call sites.

### 2. Workspace layout and template rules are runtime-specific inside agent package

Files:

- `internal/agent/workspace.go`
- `internal/agent/runtime_state.go`
- `internal/agent/service.go`

Symptoms:

- `workspaceTemplateForAgent` defaults directly to `RuntimeKindPicoClawSandbox`.
- `OpenClawRuntimeHost` overrides `WorkspaceTemplate`, `EnsureWorkspace`, `WorkspaceLayout`.
- `workspaceLayoutForOverlay` switches on `RuntimeKindOpenClawSandbox` vs PicoClaw vs fallback.
- `managerGatewayMatch` is really a runtime bootstrap rule, but lives in generic workspace logic.

Problem:

Workspace shape is a runtime concern. `internal/agent` should ask the runtime how to seed and overlay workspace, not know where OpenClaw mounts `.openclaw/workspace` while PicoClaw mounts `.picoclaw/workspace`.

### 3. Gateway config generation is split between agent and concrete runtimes

Files:

- `internal/agent/manager_config.go`
- `internal/agent/runtime_state.go`
- `internal/runtime/openclawsandbox/config.go`

Symptoms:

- PicoClaw config rendering lives in `internal/agent`.
- OpenClaw config rendering lives in `internal/runtime/openclawsandbox`.
- `PicoClawRuntimeHost` and `OpenClawRuntimeHost` inject different `EnsureGatewayConfig` functions.
- `ensureGatewayConfigForAgent` and manager bootstrap know that only some worker/manager runtimes need this config step.

Problem:

The runtime preparation contract is inconsistent. One runtime's bootstrap artifacts are owned by `internal/agent`; another runtime's artifacts are owned by `internal/runtime/...`.

### 4. Sandbox box creation details still leak through “gateway” concepts

Files:

- `internal/agent/runtime_state.go`
- `internal/agent/service.go`
- `internal/runtime/sandboxgateway/runtime.go`

Symptoms:

- `gatewayBoxFactory`, `gatewayConfigurer`, `createGatewayBox`, `gatewayCreateSpec`, `gatewayStartCommand`, `forceRemoveBox`.
- `EnsureManager` and `CreateWorker` still contain gateway-specific control flow and special test paths using `testCreateGatewayBoxHook`.
- `Service` owns bootstrap progress phases like `gatewayBoxPhasePreparing/Creating`.

Problem:

The shared sandbox runtime helper is useful, but the abstraction leaked upward. `agent.Service` still knows that some runtimes are “gateway runtimes” needing config, workspace preparation, projects mount, custom log paths, and special bootstrap rules.

### 5. Runtime profile translation is partly centralized in agent

Files:

- `internal/agent/runtime_state.go`
- `internal/agent/manager_config.go`
- `internal/app/runtimewiring/picoclaw.go`

Symptoms:

- `runtimeProfileForKind` rewrites Codex profiles to use the manager LLM bridge.
- `picoclawBridgeModelID` exists both in `internal/agent/manager_config.go` and `internal/app/runtimewiring/picoclaw.go`.
- `bridgeLLMEnvVars` and reserved env filtering live in `internal/agent`, but are only meaningful for some runtime families.

Problem:

Profile normalization belongs in agent domain, but *runtime-facing translation* should belong to the runtime adapter. Today the same “bridge” knowledge is duplicated across packages.

### 6. Persistence and hydration still assume specific runtime defaults

Files:

- `internal/agent/store.go`
- `internal/agent/runtime_record.go`

Symptoms:

- legacy workers default to `RuntimeKindPicoClawSandbox`
- manager load path forces `RuntimeKindPicoClawSandbox`
- runtime record normalization defaults empty kind to PicoClaw

Problem:

Some of this is acceptable as compatibility migration logic, but it should be isolated as migration code. Right now it also reinforces PicoClaw as a hidden semantic default inside active domain code.

### 7. Adjacent code still relies on concrete runtime types

Files:

- `cli/serve/serve.go`

Symptoms:

- `newCodexBridgeManager` fetches runtime by kind, then asserts `*runtimecodex.Runtime`.

Problem:

This is outside `internal/agent`, but it shows the same pattern: feature integration depends on concrete runtime structs rather than capability interfaces.

## Root Cause

The current `agentruntime.Runtime` interface is intentionally minimal:

- `Create`
- `Start`
- `Stop`
- `Delete`
- `State`
- `Info`

That works for pure lifecycle, but CSGClaw also needs runtime-owned behavior around:

- bootstrap preparation
- workspace layout and seeding
- config/materialization on host
- profile-to-runtime translation
- log sourcing
- optional side-channel integrations

Since those capabilities are not modeled explicitly, `internal/agent` filled the gap with:

- helper structs like `PicoClawRuntimeHost`
- runtime-kind checks
- side interfaces (`gatewayConfigurer`, `gatewayBoxFactory`)
- direct imports of concrete runtime packages

## Recommended Target Design

### Principle

Keep `agent.Service` as the orchestrator of agent records and lifecycle policy, but move runtime-specific preparation behind a richer runtime capability model.

`internal/agent` should know:

- the agent record
- the selected runtime kind
- generic lifecycle states
- whether a runtime supports optional capabilities

`internal/agent` should not know:

- OpenClaw vs PicoClaw workspace directory layouts
- how a gateway config file is rendered
- which env vars a runtime needs
- which start command a sandbox runtime runs
- how a runtime maps an LLM profile into its own config schema

## Proposed Interfaces

### 1. Keep the base lifecycle interface small and rename `Create` to `New`

The main public runtime interface should stay lightweight. Its creation entry should mean:

- instantiate or open the runtime execution object
- return the runtime handle
- not own workspace/config/materialization concerns

Recommended shape:

```go
type Runtime interface {
    Kind() string

    New(ctx context.Context, spec NewSpec) (Handle, error)
    Start(ctx context.Context, h Handle) (State, error)
    Stop(ctx context.Context, h Handle) (State, error)
    Delete(ctx context.Context, h Handle) error
    State(ctx context.Context, h Handle) (State, error)
    Info(ctx context.Context, h Handle) (Info, error)
}
```

Why `New`:

- it better matches the intended lightweight semantics than the current overloaded `Create`
- it reduces the pressure to stuff preparation logic into the base runtime interface
- it makes room for a separate provisioning phase with a distinct meaning

### 2. Add a separate `Provisioner` capability for preparation and initialization

Preparation should be an optional runtime-owned capability, not part of the base lifecycle interface.

Recommended shape:

```go
type Provisioner interface {
    Provision(ctx context.Context, req ProvisionRequest) error
}
```

`Provision(...)` is the core abstraction for moving PicoClaw/OpenClaw details out of `internal/agent`.

It should own things like:

- workspace layout choice
- template seeding
- config generation
- host-side runtime directories
- runtime-specific env/config materialization
- other idempotent preparation steps required before `New(...)`

That config scope should explicitly include runtime-owned bootstrap artifacts such as
gateway/manager config files. In practice, helpers like
`ensureWorkerGatewayConfig`, `ensureManagerPicoClawConfig`, and similar
runtime-specific config writers should be folded into `Provision(...)` where
practical, so `internal/agent` no longer owns concrete config file shapes or
write paths.

It should not:

- return the final runtime handle
- implicitly replace `New(...)`
- hide agent-domain validation/persistence logic

Recommended boundary:

- `Provision(...)` prepares the environment
- `New(...)` creates the runtime instance and returns the handle

This keeps the semantics crisp and avoids a heavy one-size-fits-all `Create`.

### 3. Add optional narrow capabilities where useful

Recommended capability split:

```go
type ProfileAdapter interface {
    RuntimeProfile(ctx context.Context, req ProfileRequest) (Profile, error)
}

type LogSource interface {
    StreamLogs(ctx context.Context, h Handle, opts LogOptions) error
}
```

Notes:

- exact names can change; the important part is separating provisioning from instantiation
- `sandboxgateway.Runtime` can implement `Provisioner` plus `Runtime`
- Codex can implement `Runtime` and only opt into `Provisioner` if it truly needs host-side preparation
- Notifier can likely skip `Provisioner` entirely

### 4. Introduce a runtime descriptor / behavior object

Besides interfaces on runtime instances, add a static descriptor per runtime kind:

```go
type Descriptor struct {
    Kind string
    SupportsManager bool
    SupportsWorker bool
    DefaultManagerImage string
    DefaultRole RoleBehavior
}
```

This replaces scattered helpers like:

- `isGatewayRuntimeKind`
- `managerImageForRuntimeKind`
- parts of `runtimeForAgent`

The service can ask a registry for the descriptor instead of hardcoding runtime families.

## Concrete Refactor Plan

### Phase 1. Rename `Create` to `New`

Do first:

1. rename the lightweight public runtime `Create` entry to `New`
2. keep semantics unchanged: instantiate/open the runtime execution object and return `Handle`
3. do not move provisioning, workspace setup, or config generation into `New(...)`

Expected result:

The base runtime lifecycle interface becomes clearer without forcing an immediate
behavioral refactor.

### Phase 2. Introduce and adopt `Provisioner`

This phase is best split into smaller incremental steps.

#### Phase 2.1. Add the `Provisioner` boundary

Do first:

1. add `Provisioner` in `internal/runtime`
2. define `ProvisionRequest` narrowly around preparation concerns
3. document the intended `Provision(...)` / `New(...)` boundary clearly

Expected result:

The codebase has the intended abstraction boundary before control flow and
runtime implementations start moving.

#### Phase 2.2. Move the bulk of provision-like logic behind `Provision(...)`

Do next:

1. migrate the majority of runtime-owned preparation into `Provision(...)`
2. prioritize config generation, workspace seeding, and host directory
   preparation
3. fold helpers such as `ensureWorkerGatewayConfig`,
   `ensureManagerPicoClawConfig`, and similar runtime-specific config writers
   into runtime `Provision(...)` implementations where practical

Expected result:

Most runtime-specific preparation logic now lives behind the `Provisioner`
boundary instead of in `internal/agent`.

#### Phase 2.3. Update orchestration to use `Provision(...)`

Do next:

1. update `agent.Service` create/recreate flows to call `Provision(...)` before
   `New(...)`
2. keep `agent.Service` responsible for orchestration and timing, not concrete
   config/workspace details
3. defer only the remaining runtime leakage that does not fit cleanly into
   `Provision(...)` yet

Expected result:

The new boundary is not only defined but exercised in the main lifecycle paths,
while remaining edge-case runtime leakage can still be migrated later.

### Phase 3. Replace `PicoClawRuntimeHost` with runtime capability interfaces

Current problem:

`PicoClawRuntimeHost` is effectively an untyped adapter bag used to smuggle runtime behavior from `agent.Service` into runtime constructors.

Recommendation:

- remove `PicoClawRuntimeHost` / `OpenClawRuntimeHost`
- runtime wiring should assemble dependencies directly for each concrete runtime
- generic host-side helpers can live in a neutral helper package, but not as a public abstraction returned by `agent.Service`

Expected result:

`internal/agent` stops being a factory for runtime implementation dependencies.

### Phase 4. Move workspace/config preparation into `Provisioner`

Refactor:

- move `workspaceTemplateForAgent`, `resolveRuntimeTemplateRoot`, `ensurePicoClawWorkspace`, `ensureOpenClawWorkspace`, `picoClawWorkspaceLayout`, `openClawWorkspaceLayout` behind `Provisioner`
- move runtime-specific config preparation such as `ensureWorkerGatewayConfig`,
  `ensureManagerPicoClawConfig`, and related gateway config writers behind
  `Provisioner`
- keep only generic file-copy helpers in `internal/agent` or move them to a small shared package like `internal/runtime/workspacefs`

Suggested flow:

```text
agent.Service.CreateWorker
  -> runtime.Provision(...)
  -> runtime.New(...)
```

where `Provision(...)` internally handles config + workspace seeding + host
directory creation, including runtime-owned gateway/manager config files.

Expected result:

No `switch runtimeKind` in `workspaceLayoutForOverlay`.

### Phase 5. Keep runtime instantiation lightweight via `New`

Refactor:

- rename the current lightweight public `Create` entry to `New`
- keep `New(...)` focused on creating the runtime execution object and returning `Handle`
- do not move host-side preparation into `New(...)`

Expected result:

The public lifecycle interface stays small even after preparation logic moves behind runtime-owned abstractions.

### Phase 6. Replace gateway runtime branching with descriptor-driven behavior

Refactor:

- replace `isGatewayRuntimeKind` with descriptor/capability checks
- replace `runtimeKindForGatewayAgent` and parts of `runtimeForAgent` with explicit runtime selection rules
- encode manager support, worker support, and default image in runtime descriptors

Example:

- `EnsureManager` should resolve “the configured manager runtime”
- `CreateWorker` should resolve either requested runtime or default worker runtime
- service should not care whether the runtime happens to be sandbox gateway, codex, notifier, or future runtime

Expected result:

Adding a new runtime kind becomes “register runtime + descriptor + capabilities”, not “touch many branches in agent”.

### Phase 7. Isolate compatibility defaults and migrations

Refactor:

- keep legacy JSON upgrade logic in `store.go`, but move PicoClaw defaulting into clearly named migration helpers
- avoid using PicoClaw defaults in active runtime selection logic except where truly required by backward compatibility

Expected result:

Runtime choice becomes an explicit persisted property, not an emergent fallback hidden across load/hydrate paths.

### Phase 8. Remove concrete runtime type assertions in adjacent layers

For example:

- replace `*runtimecodex.Runtime` assertion in `cli/serve/serve.go` with a capability interface like:

```go
type CodexBridgeRuntime interface {
    SessionManager() codex.Manager
    EventSink() codex.SessionEventSink
}
```

Expected result:

feature integrations depend on behavior, not concrete structs.

## Suggested Responsibility Split

### `internal/agent`

Should own:

- agent CRUD and persisted state
- runtime selection policy
- lifecycle orchestration
- profile completeness policy
- compatibility migration

Should not own:

- runtime config file formats
- runtime workspace topology
- runtime env var schema
- runtime bootstrap shell commands

### `internal/runtime/<kind>`

Should own:

- concrete profile translation
- provisioning/materialization
- config rendering
- host artifact preparation
- workspace layout conventions
- runtime-specific logs
- runtime-specific optional integrations

### `internal/app/runtimewiring`

Should own:

- construction and registration of concrete runtimes
- dependency injection of shared helpers

Should not require `agent.Service` to expose large runtime-host structs.

## `Provision` / `New` Boundary

This boundary matters more than naming.

`Provision(...)` should be:

- runtime-owned
- idempotent
- safe to call during create/recreate and, if needed later, before start
- responsible for materializing host-side state

`New(...)` should be:

- lightweight
- focused on creating/opening the runtime execution object
- the only place that returns the final handle

Bad split:

- `Provision(...)` secretly creates the box/session already
- `New(...)` mostly becomes a no-op wrapper

Good split:

- `Provision(...)` writes config, prepares workspace, ensures directories
- `New(...)` creates the box/session and returns `Handle`

## Pragmatic Implementation Strategy

I would not try to redesign everything in one pass. Recommended order:

1. rename the lightweight public `Create` method to `New`
2. add the `Provisioner` boundary
3. move most runtime-specific preparation logic behind `Provision(...)`
4. update `agent.Service` to orchestrate `Provision(...)` plus `New(...)`
5. defer the remaining runtime-specific leakage that does not fit neatly into
   `Provision(...)` yet
6. after that, extract duplicated bridge/config/env helpers as needed
7. clean up persistence fallbacks and adjacent type assertions

This sequence reduces risk because the first cut still avoids a full runtime
redesign, but it does not stop at a nominal interface. The `Provisioner`
boundary should earn its keep by covering most preparation/config/workspace
behavior from the start.

## Recommended First Slice

If you want the smallest reasonable first slice, I recommend this:

1. Rename the lightweight public `Create` interface method to `New`.
2.1. Add `Provisioner` in `internal/runtime`, define `ProvisionRequest`, and
     document the intended `Provision(...)` / `New(...)` boundary.
2.2. Move the majority of runtime-owned preparation into `Provision(...)`,
     including workspace/config/host-state setup that already maps cleanly to
     provisioning.
2.3. Change `agent.Service` create/recreate flows to call `Provision(...)`
     before `New(...)`.

That slice is still conservative because it avoids a full runtime-architecture
rewrite, but it is substantive: `Provisioner` is introduced precisely so it can
start covering most runtime-specific preparation logic rather than existing as
an unused abstraction.

What remains for later is the smaller set of runtime-specific details that do
not fit cleanly into `Provision(...)` yet, such as broader descriptor cleanup,
host wiring reshaping, and adjacent concrete-type assumptions.

## Suggested Service Flow

The near-term target `agent.Service` create path should look closer to:

```text
resolve runtime
  -> normalize agent/profile/runtime options
  -> if runtime implements Provisioner: Provision(...)
  -> New(...)
  -> persist agent/runtime record
```

And recreate should look like:

```text
resolve runtime
  -> Delete(old handle)
  -> if runtime implements Provisioner: Provision(...)
  -> New(...)
  -> persist refreshed handle/state
```

This keeps `agent.Service` as orchestrator, while the runtime owns the bulk of
preparation details early and the remaining edge-case leakage can be cleaned up
afterward.

## Why This Is The Core Recommendation

This design avoids both failure modes:

- current state: too much runtime detail leaks into `internal/agent`
- over-correction: one giant heavy `Create` method that mixes preparation and instantiation

With the first migration slice in place, `Provisioner + New` gives:

- a small base runtime lifecycle interface
- a real runtime-owned home for most PicoClaw/OpenClaw-specific preparation
- a clean separation between materialization and instantiation
- an incremental migration path from the current code

## Summary

The main issue is not only “there are a few `if/else` checks”. The bigger issue is that `internal/agent` currently acts as:

- agent domain service
- runtime family classifier
- sandbox bootstrap coordinator
- workspace topology owner
- partial runtime config renderer

Those extra roles are what make the abstraction porous.

The clean direction is:

- keep lifecycle orchestration in `internal/agent`
- keep the public runtime lifecycle interface lightweight via `New`
- move concrete preparation and translation into `internal/runtime/...` behind `Provisioner`
- express optional behavior through capability interfaces and runtime descriptors
- treat PicoClaw/OpenClaw differences as runtime implementation detail, not agent-domain branching
