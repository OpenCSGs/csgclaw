# Agent Engine and RuntimeExtension

中文：[agent-engine-decoupling.zh.md](agent-engine-decoupling.zh.md)

## Implemented architecture

The final backend split replaces the former Service-backed facade with independent resource, repository, lifecycle and adapter owners.
Phase 1 (Session), Phase 2 (shared contract and MemoryClient), Phase 3 (shared production Engine), Phase 4 (backend ownership) and RuntimeExtension are implemented.
This page describes actual contracts, not additional proposed interfaces.
The platform diagram and capability matrix are in [architecture.md](architecture.md).

```text
API / Channel / Session
  -> agentengine.Interface
       Agents()
       Conversations(agentID)
       RuntimeExtensions(agentID)
  -> shared lifecycle.Coordinator
  -> registered Runtime Adapter
```

The public Go types live in `internal/agentengine/contract` and are re-exported by `internal/agentengine`.
That package separation avoids an Engine/concrete-Adapter import cycle; it is not a compatibility facade.
The Engine never imports a concrete Runtime, IM, Participant or Channel.

## Owners

| Owner | Responsibility |
| --- | --- |
| `agents.Repository` | Existing local persistence for Agent desired state, Runtime observations and Extension records |
| `agents.Controller` | Agent resource operations, field masks, provisioning and lifecycle orchestration |
| `Engine` conversation coordinator | Turn admission, identity/replay, event ordering, Cancel, Reset, Resolve and immutable file handles |
| `interactionstate.Coordinator` | Native and detached pending/terminal interaction state, validation, expiry and single resolution |
| `lifecycle.Coordinator` | Per-Agent execution and exclusive mutation leases |
| `registry.Registry` | Registered Runtime selection and deterministic shutdown |
| `runtimeExtensionManager` | Extension resources, Source registration/resolution, desired/observed generation and recovery |
| Runtime Adapter | Layout, Provision, process lifecycle, probes, native conversations and Extension Driver |
| `WorkspaceService` / `ModelConfiguration` | Workspace/log/template support and model configuration through narrow interfaces |

The Controller is the resource implementation, not a delegate to the deleted `internal/agent.Service`.
The old Agent facade, Service adapter, Codex bridges and Channel lifecycle callbacks are removed.
API constructors receive an Engine explicitly.
Runtime registration is sealed after composition; unregistered capabilities are errors.

## Agent resource

`Agents()` exposes Create, Get, List, Update, Delete and Recreate.
Start/Stop use a `desired_state` field mask.
Profile/model, Skills, MCP, instructions and memory HTTP operations translate to field-masked updates.
An omitted field mask means complete desired-spec replacement; an ID-only recreate uses an explicit mask to retain existing configuration.
Write-only credentials and model secrets are preserved when omitted, and never returned as credential contents.

`RuntimeSpec.Credentials` and `InitShell` belong to complete base provisioning.
Incremental Extension reload never reapplies them.
Workspace browsing, logs, template publishing and model configuration use dedicated support interfaces.
Ordinary Agent Get reads the stored resource; `ProbeRuntime` explicitly requests live observation.

## Conversations

`Conversations(agentID)` exposes Run, Cancel, Reset, Resolve, GetInteraction and Files.
A ConversationKey is caller-owned and opaque.
The native Runtime mapping belongs to the Adapter, not to the IM transcript or Session Binding Store.

Run validates the request, admits one active Turn per Agent/key, acquires an execution lease and selects a registered ConversationProvider.
A missing implementation returns `runtime_adapter_unavailable`, with no fallback.
Reject, wait and supersede admission are distinct policies.
A repeated Turn ID must have the same request and replays the same completed result rather than executing twice.
Engine keeps at most 1024 completed dispatched Turns in memory.

Cancel targets one Turn.
Reset excludes new admission for that conversation, cancels and waits for active execution, then resets its native mapping.
Neither operation deletes the IM transcript.
Runtime output files become immutable Engine-scoped file handles only after success; failed or canceled Turn files are not published.

## Interaction resolution

The only production interaction path is:

```text
HTTP / Channel action
  -> trusted Channel route
  -> Conversations().GetInteraction / Resolve
  -> interactionstate.Coordinator
  -> selected Runtime Adapter
  -> native permission or user-input request
```

Engine atomically claims a pending interaction and validates its response.
The optional transcript callback runs before releasing the native request.
A failed callback leaves the request pending; a duplicate response cannot persist a second transcript.
Explicit invalidation cancels in-flight response callbacks and prevents them from completing a detached answer afterward.
Successful Turn completion is distinct from cancellation because a native response can unblock execution before its response call returns.
Public snapshots redact secret answers.

Successful structured command output can create a detached question after the Turn ends.
Its lifecycle belongs to Engine, not to Codex's native request broker.
Answer/expiry/cancel/reset/new-Turn/lifecycle events update the original UI projection.
The Channel may submit a valid detached continuation through its normal ingress after rendering the answer.
It does not call the Runtime directly.
The old Codex detached, Channel-binding and transcript APIs have been removed.

## Shared lifecycle

Run holds an execution lease until Runtime cleanup finishes.
Agent Update/Delete/Recreate and Extension Apply/Delete take a mutation lease.
The mutation closes execution admission, drains active leases and serializes changes to that Agent.
Context cancellation applies while waiting and draining; the default drain timeout is two minutes.
Mutation contexts are reentrant so integration fact writes and nested Extension Apply use the same lease.
Different Agents remain independent.

Channel Binding Managers own Worker lifetime.
Runtime Stop/Recreate/reload cannot delete a Binding or transcript or stop a Channel Worker.
Business credential changes explicitly reconcile the Channel Binding.
OpenClaw/PicoClaw retain their native gateway channels and command protocols.
Their binding configuration changes use Engine-managed recreation through the existing Runtime adapters; hosted Codex workers remain independent of Runtime lifecycle.

## RuntimeExtension contract

```go
RuntimeExtensions(agentID).Apply(ctx, RuntimeExtensionApplyRequest)
RuntimeExtensions(agentID).Get(ctx, name)
RuntimeExtensions(agentID).List(ctx)
RuntimeExtensions(agentID).Delete(ctx, name)
```

```go
type RuntimeExtensionSpec struct {
    Name          string
    Kind          string
    Source        RuntimeExtensionSourceRef
    FailurePolicy RuntimeExtensionFailurePolicy
}
type RuntimeExtensionSourceRef struct {
    Provider string
    Ref      string
}
```

Name is unique within the Agent.
The specification references business facts and contains no resolved secret, host path, file content or command.
Apply is an independent child-resource update, not a field of AgentSpec.
Replacing a Runtime preserves its Extension desired resources even if the new Adapter cannot configure them or fails to start.
An optional resource version rejects stale writes; each accepted Apply advances generation.
Repeated source revisions can reuse the same effective projection without rerunning bind or restarting a loaded process.

Status contains `configured / unavailable / error`, generation, observed_generation, source_revision, reason, message, checked_at, applied_at and runtime_loaded.
Unavailable describes a missing executable or failed availability probe, not an internal error.
An unknown driver yields `extension_unsupported`.
Deletion remains possible when the current Adapter has no Extension projections.
Deletion intents do not gate readiness, and cleanup reload success does not require unrelated Extensions to be ready.
Get/List read persisted status without executing commands or implicitly applying configuration.

## Source and Driver

Integration code registers a Source that resolves `(agentID, ref)` into a source revision and versioned opaque payload.
The resolved payload exists only during reconciliation.
Engine does not store or log it.
The Source does not own Runtime layout or restart policy.

A Runtime's ExtensionDriverProvider selects a Driver by kind.
The Driver validates, probes and prepares a reversible managed projection.
Its PreparedExtension exposes Projection, Activate, Rollback and Cleanup.
The Runtime's ExtensionHost lists projections, renders Engine-ordered instructions and prepares deletion independently of executable availability.

Each projection uses a private `<agent>/<extension-name>/generation-*` root.
Initialization runs in staging, using the eventual stable path.
An atomic active manifest selects the generation.
Drivers do not overwrite user files or other Extensions.
Environment conflicts fail unless the values are identical, including conflicts with explicit Agent model environment.
Instructions fragments are sorted by Extension name and merged by the Runtime renderer, preserving user-owned instructions outside managed blocks.

## Apply, recovery and deletion

1. Under the Agent mutation lease, validate and save the new desired generation.
2. Resolve current Source facts and select the registered Driver.
3. Probe, validate and stage the projection.
4. Validate the complete environment/instructions contribution set.
5. Activate and render; roll back managed projection failures when safe.
6. Reload an already-running Runtime only when its effective projection is not loaded.
7. Record observed generation, status and timestamps; clean obsolete staging.

Stopped Runtimes are never started by Apply/Delete.
For Codex, loaded state is checked against the live process's recorded projection digest, not inferred from files.
A configured-but-unloaded tool is visibly pending or carries a reload warning.

A failed retry of the same source revision can retain the previous active generation.
Once Source revision changes, failure disables the old projection instead of retaining stale credentials.
Source resolution failure also disables an existing projection.
Rollback covers managed files and Runtime contributions, not external side effects of commands.

Recreate reapplies desired Extensions in stable name order before Runtime start.
Optional failures degrade the Extension without blocking normal startup.
A block_runtime Extension must be configured and loaded before Agent readiness.
Delete records deletion intent, removes the Runtime projection and only then removes the desired resource.
Cleanup failures retain `delete_failed` for retry and startup recovery; they cannot accidentally reapply the deleted tool.
If projections cannot be safely removed from a live Runtime, it is stopped and cleanup remains retryable.

## Feishu integration

The fixed optional binding is `feishu-lark-cli / lark-cli / feishu-participant / <participant-id>`.
Registration and manual Participant writes serialize credential updates with Apply.
AppID exclusivity remains in `feishubind`.
Missing lark-cli does not prevent connecting Feishu; the UI shows installation/retry guidance.
Disconnect deletes the Participant first, invalidating old source tokens, then removes the Extension.
Partial cleanup has a dedicated retry action that refuses to remove a newly connected Bot.

Source tokens bind Agent, Participant, purpose and credential revision, also in no-auth mode.
App Secret stays in Participant storage.
The Codex Driver owns bind, private configuration, reserved environment and instructions.
CSGClaw does not install or upgrade lark-cli.
No general-purpose HTTP payload/command Apply endpoint exists.

## Verification and limits

The same resource and detached-interaction contract runs against the real Engine and MemoryClient.
Focused tests cover projection isolation, conflict, rollback, source invalidation, stopped-state preservation, single reload, delete retry and HTTP credential revocation.
`architecture_test.go` prevents concrete Runtime/Channel imports in Engine and native broker calls from API/Channel/CLI.

Runtime capability support is explicit in the platform matrix.
Pending interactions, Turn replay state and file handles are process-local.
There is no cross-process transaction with Feishu or a guarantee to undo external command side effects.
