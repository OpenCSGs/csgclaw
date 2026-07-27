# Bot Create Refactor TODO

## Goal

Align the domain boundaries around these concepts:

- `agent`: runtime entity only
- `user`: IM identity only
- `bot`: binding between agent and user

After the refactor:

- `create agent` only creates an agent
- `create user` owns IM bootstrap side effects
- `create bot` orchestrates agent creation and user provisioning, then stores the bot binding

## Why This Needs Incremental Changes

The current implementation spreads IM side effects across multiple layers:

- `internal/api/handler.go`
  `handleCreateAgentWorker()` calls `ensureWorkerIMState()`
- `internal/bot/service.go`
  `ensureChannelUser()` calls `im.EnsureAgentUser()`, but discards the returned room
- `internal/im/service.go`
  `EnsureAgentUser()` mutates IM state and may create the admin/agent bootstrap room

There is also no `csgclaw` implementation for `user create` yet:

- `cli/user/user.go`
  `user create` currently only supports `--channel feishu`
- `internal/api/handler.go`
  `/api/v1/users` supports `GET` and `DELETE`, but not `POST`

Because of that, a direct refactor would mix:

- new API surface
- CLI behavior changes
- service extraction
- event publishing changes
- test rewrites

## Target Design

### Service responsibilities

- `agent.Service`
  Only manage agent lifecycle
- `im.Service`
  Own IM domain state and explicit IM operations
- new app-level IM provisioning layer
  Wrap IM state changes with event publication and bootstrap messaging
- `bot.Service`
  Coordinate agent creation and IM user provisioning

### Reuse rule

Reuse existing IM domain methods where the behavior is already part of the IM model:

- user creation or ensure logic
- room creation logic
- message creation logic

Do not duplicate those behaviors in handlers or bot-specific code.

At the same time, do not route internal orchestration through CLI commands or HTTP handlers just to "reuse" them.

The reuse boundary should be:

- reuse `im.Service`
- do not reuse `cli/*`
- do not call `/api/v1/*` from inside the server

### What should stay in `im.Service`

`im.Service` should own explicit IM operations such as:

- `EnsureAgentUser(...)` or a renamed equivalent for IM user provisioning
- `CreateRoom(...)`
- `CreateMessage(...)`
- room/member lookup and mutation

Longer term, bootstrap room creation should be represented by explicit IM operations instead of hidden side effects inside user ensure flows.

That means code like `ensureAdminAgentRoomLocked()` should eventually be removed or reduced to a lower-level helper behind an explicit public method.

Preferred direction:

- keep user ensure explicit
- keep room creation explicit
- keep message creation explicit
- avoid "ensure user and maybe also create a room and maybe also seed a message" as a hidden side effect

### What should stay out of `im.Service`

These concerns should remain outside `im.Service`:

- `publishUserEvent(...)`
- `publishRoomEvent(...)`
- `publishMessageCreated(...)`
- asynchronous bootstrap-message scheduling
- bot/agent orchestration

Those are application-layer concerns, not IM state concerns.

### Provisioner responsibility

The new IM provisioner should orchestrate existing IM operations in a consistent order:

1. ensure the IM user exists
2. ensure the bootstrap room exists
3. publish user/room events when newly created
4. create the bootstrap message when needed
5. publish message-created event when that message is created

This lets `bot create` and `user create` share one orchestration path without pushing event-bus knowledge into `im.Service`
and without hiding room/message creation inside `EnsureAgentUser(...)`.

### IM provisioning contract

Introduce a focused application service, for example:

- `internal/im/provisioning.go`

Candidate API:

```go
type AgentIdentity struct {
    ID          string
    Name        string
    Description string
    Handle      string
    Role        string
}

type ProvisionResult struct {
    User        im.User
    Room        *im.Room
    UserCreated bool
    RoomCreated bool
}

func (p *Provisioner) EnsureAgentUser(ctx context.Context, identity AgentIdentity) (ProvisionResult, error)
```

Responsibilities of this layer:

- call `im.Service.EnsureAgentUser(...)`
- call explicit room-creation logic rather than relying on hidden room creation side effects
- call explicit message-creation logic for bootstrap messaging
- publish `user_created` when the user is newly created
- publish `room_created` when the bootstrap room is newly created
- asynchronously send the admin bootstrap message when a new bootstrap room is created

This keeps event bus logic out of handlers and avoids duplicating bootstrap behavior between `agent create`, `user create`, and `bot create`.

### Existing code that should be converged

The following code paths currently overlap and should converge on the new split:

- `internal/api/handler.go`
  `ensureWorkerIMState()`
- `internal/im/service.go`
  `ensureAdminAgentRoomLocked()`
- `internal/bot/service.go`
  `ensureChannelUser()`

Target convergence:

- `ensureWorkerIMState()` disappears
- `ensureChannelUser()` stops owning hidden IM bootstrap behavior
- bootstrap room/message creation happens through explicit IM operations coordinated by the provisioner

### Bootstrap room semantics to settle during implementation

Before replacing implicit room creation with explicit `CreateRoom(...)`, keep these semantics aligned:

- whether bootstrap room creation should also create the standard `room_created` event message
- whether the admin bootstrap instruction message should remain a separate follow-up message
- whether bootstrap rooms need a dedicated title/description convention

This matters because `CreateRoom(...)` already creates a structured room-created event, while the current bootstrap path also creates a custom bootstrap message.

The likely target is:

- use explicit room creation
- keep the explicit bootstrap instruction message
- make both behaviors visible and testable

## Incremental Plan

### Phase 1: Extract IM bootstrap orchestration

- move `ensureWorkerIMState()` behavior out of `internal/api/handler.go`
- create a reusable app-level provisioner around:
  - `im.Service`
  - `im.Bus`
- as a temporary migration step, keep the current `POST /api/v1/agents` behavior unchanged by making it call the new provisioner

Note:

- this is only to preserve compatibility during the refactor
- it is not the target design
- the target design is still:
  - `POST /api/v1/agents` creates agents only
  - `POST /api/v1/users` creates IM users only
  - `POST /api/v1/bots` orchestrates both when needed

Acceptance:

- existing `POST /api/v1/agents` behavior is preserved during this phase
- `bot create` still goes through `POST /api/v1/bots`
- no external API contract changes yet

### Phase 2: Add `csgclaw user create`

- add `POST /api/v1/users` for `csgclaw`
- add CLI support in `cli/user/user.go`
- route user creation through the new IM provisioner

Suggested request shape:

```json
{
  "id": "u-alice",
  "name": "alice",
  "handle": "alice",
  "role": "worker"
}
```

Acceptance:

- `csgclaw user create` becomes the only entry point that creates IM bootstrap side effects directly
- events and bootstrap message are emitted from the provisioner, not the handler

### Phase 3: Move `bot create` to orchestration-only behavior

- update `bot.Service.Create(...)`
- for `channel=csgclaw`:
  - ensure/create agent
  - provision IM user via the shared provisioner
  - persist bot binding
- stop discarding room/bootstrap results

Acceptance:

- `POST /api/v1/bots` becomes the canonical orchestration entry point for bot creation
- `bot create --channel csgclaw` and `user create --channel csgclaw` share the same IM provisioning path
- bootstrap room visibility and event behavior are consistent

### Phase 4: Remove IM side effects from `agent create`

- remove implicit IM provisioning from `POST /api/v1/agents`
- keep `agent create` runtime-only

Acceptance:

- `POST /api/v1/agents` no longer affects IM users or rooms
- tests clearly separate runtime creation from IM creation

## Test Plan

Add or update targeted tests in these areas:

- `internal/im/service_test.go`
  keep state mutation tests here only
- `internal/api/handler_test.go`
  API behavior and event publication
- `internal/bot/service_test.go`
  bot orchestration behavior
- `cli/app_test.go`
  CLI routing and payloads for `user create --channel csgclaw`

Critical cases:

- creating a new worker bot in `csgclaw` creates:
  - agent
  - user
  - admin/agent room
  - admin bootstrap message
- room creation reuses the same IM room-creation semantics as other explicit room creation flows
- bootstrap messaging reuses the same IM message-creation semantics as other explicit message flows
- re-running create is idempotent
- duplicate handles are rejected
- existing agent + missing IM user can be repaired by bot/user provisioning
- `agent create` no longer creates IM side effects after Phase 4

## Non-Goals For The First Refactor

- changing Feishu user creation semantics
- redesigning IM room naming
- changing persisted IM state format
- changing bootstrap room creation rules inside `im.Service`

## Recommended Landing Order

1. extract provisioner
2. switch existing `agent create` to use provisioner
3. add `csgclaw user create`
4. switch `bot create` to use provisioner
5. remove IM side effects from `agent create`

This order keeps each patch small and testable, while avoiding a half-migrated state where behavior changes but no dedicated user-creation path exists yet.
