# Add Agent Hub

## Goal

Add a new `hub` module so CSGClaw can:

- manage agent templates from built-in or configured registries
- let users pick a template when creating an agent
- publish the current agent into a hub registry
- support both local-folder and remote-HTTP stores

Each hub agent template contains:

- `name`
- `description`
- `runtime_kind`
- `image`
- `workspace`

The `workspace` is a directory tree and may include `AGENTS.md`, `skills/`, and other agent-scoped files.

## Design Principles

- Keep `cli/` responsible for command flow and output only.
- Keep `internal/api/` as thin HTTP transport only.
- Put hub domain logic in a new `internal/hub/` package.
- Do not couple hub storage to sandbox/runtime startup logic.
- Keep v1 simple: metadata + workspace snapshot as template source.

## Scope

V1 should support these flows:

1. List templates from all enabled registries.
2. Inspect one template.
3. Create a new agent from a selected template.
4. Publish an existing local agent into a selected registry.

V1 can explicitly defer:

- auth for remote registries beyond simple token/header support
- version history and rollback
- partial workspace merge
- dependency/image signing
- marketplace ranking or search relevance

## Package Layout

Recommended layout:

```text
cli/hub/                   CLI commands: list, get, publish
internal/hub/              domain service and registry abstraction
internal/hub/storelocal/   local folder store
internal/hub/storeremote/  remote HTTP store
internal/hub/hubtypes/     shared DTOs if needed
```

If the codebase prefers fewer packages, `storelocal` and `storeremote` can stay under `internal/hub/` first.

## Core Model

Suggested domain types:

```go
type Template struct {
	ID          string
	Name        string
	Description string
	RuntimeKind string
	Image       string
	WorkspaceRef WorkspaceRef
	Source      RegistryRef
	UpdatedAt   time.Time
}

type RegistryRef struct {
	Name string
	Kind string // builtin | local | remote
}

type WorkspaceRef struct {
	Kind string // dir | tarball
	Path string // local path or remote relative path
}
```

For local persistence, each template should use a manifest file plus a workspace directory:

```text
<registry-root>/
  templates/
    frontend-alice/
      agent.toml
      workspace/
        AGENTS.md
        skills/
        ...
```

Example manifest:

```toml
name = "frontend-alice"
description = "Frontend worker with UI and styling skills"
runtime_kind = "codex"
image = ""
```

Rationale:

- easy to inspect and edit by hand
- workspace can be copied directly without archive unpacking in local mode
- remote mode can still expose the same shape through HTTP

Although worker base templates are stored under runtime-specific roots in the codebase, the current `picoclaw` and `openclaw` worker workspace trees do not have meaningful structure differences, so v1 should keep one simple template workspace layout.

## Store Abstraction

Use one service interface over multiple backends:

```go
type Store interface {
	List(ctx context.Context) ([]Template, error)
	Get(ctx context.Context, id string) (Template, error)
	FetchWorkspace(ctx context.Context, id string) (WorkspaceReader, error)
	Publish(ctx context.Context, spec PublishSpec) (Template, error)
}
```

`WorkspaceReader` can be a simple extracted directory path in v1. Avoid over-generalizing early.

Suggested concrete stores:

- `builtin`: read-only templates shipped with CSGClaw
- `local`: read/write filesystem folder
- `remote`: HTTP client against a registry service

Then add a small registry manager:

```go
type Service struct {
	stores []NamedStore
}
```

Responsibilities:

- merge template lists from all configured stores
- namespace template IDs by registry when needed, for example `builtin/frontend-alice`
- route publish requests to one target store
- normalize duplicate names and report source clearly

Registry kinds are not mutually exclusive. Multiple registries can be enabled at the same time.

Recommended capability model:

- `builtin`: readable, not publishable
- `local`: readable and publishable
- `remote`: readable and publishable

That means a typical setup can expose:

- built-in templates for discovery
- one personal local registry for custom templates
- one or more team remote registries for sharing

Publish should always target one explicit writable registry. Reads can aggregate all enabled registries.

## Built-in Registry

Built-in templates should be embedded, similar to current workspace templates.

Suggested location:

```text
internal/hub/embed/templates/<template-id>/agent.toml
internal/hub/embed/templates/<template-id>/workspace/...
```

This keeps a clean path for shipping a few curated worker templates without requiring network access.

## Config Changes

Add a new config section:

```toml
[hub]
default_registry = "local"
default_publish_registry = "local"

[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

[[hub.registries]]
name = "local"
kind = "local"
path = "~/.csgclaw/hub"
enabled = true

[[hub.registries]]
name = "team"
kind = "remote"
url = "https://hub.example.com"
token = "${CSGCLAW_HUB_TOKEN}"
enabled = true
```

Notes:

- registries are additive, not mutually exclusive
- `builtin` can coexist with any number of `local` and `remote` registries
- `default_registry` is the default read/source registry when a command needs one registry context
- `default_publish_registry` is the default target for publish when the user does not pass `--registry`
- `builtin` should be rejected as a publish target
- update loader, saver, defaults, tests, and docs together
- keep token redaction rules consistent with existing config handling
- built-in registry should work even if `[hub]` is omitted

## Agent Creation Flow

There are two reasonable options.

### Option A: Extend `agent create`

Example:

```bash
csgclaw agent create --from-template builtin/frontend-alice --name alice
```

Behavior:

1. resolve template from hub service
2. map template metadata into `agent.CreateAgentSpec`
3. create the agent as usual
4. after agent home/workspace exists, copy template workspace into the agent workspace

This is the better v1 choice because it reuses the existing agent creation path.

### Option B: Add dedicated `hub create`

Example:

```bash
csgclaw template create builtin/frontend-alice --name alice
```

This is clearer conceptually, but it duplicates part of agent creation UX.

Recommendation:

- v1: use Option A
- optionally add `csgclaw template list` and `csgclaw template publish`

## Workspace Materialization

This is the most important behavior detail.

Current agent creation already chooses a built-in workspace template. Hub templates add a second source of workspace content. The clean rule for v1 is:

1. create agent home/workspace using the current runtime-specific base template
2. overlay the hub template `workspace/` on top

That preserves runtime-required bootstrap files while allowing hub content to customize `AGENTS.md`, `skills/`, and project scaffolding.

Collision rule:

- default to overwrite files from the base workspace with hub workspace files
- reject path traversal and symlink escapes
- keep file modes simple and safe

This likely belongs in `internal/agent/workspace.go` as a reusable copy helper, with the hub service only supplying the source tree.

## Publish Flow

Example CLI:

```bash
csgclaw template publish --agent u-alice --registry local --name frontend-alice
```

Behavior:

1. load the current agent from `internal/agent.Service`
2. locate its workspace root
3. build a publish spec from:
   - agent name or explicit template name
   - description
   - runtime kind
   - image
   - workspace snapshot
4. write manifest + workspace into the target store

Important rule:

- publish should capture a snapshot of the current workspace, not a live reference

This avoids later local edits silently mutating the published template.

## Remote Registry API

Keep the remote protocol small and manifest-oriented.

Suggested endpoints:

```text
GET    /api/v1/hub/templates
GET    /api/v1/hub/templates/{id}
GET    /api/v1/hub/templates/{id}/workspace
POST   /api/v1/hub/templates
```

Suggested behavior:

- list/get return template metadata only
- workspace download returns a tar.gz archive
- publish uploads multipart or tar.gz + JSON metadata

For local CSGClaw support, these endpoints can live in `internal/api/` and be backed by `internal/hub`.

## CLI Surface

Recommended minimal commands:

```text
csgclaw template list
csgclaw template get <template>
csgclaw template publish --agent <id> [--registry <name>] [--name <template-name>]
```

And extend:

```text
csgclaw agent create --from-template <registry/template>
```

This keeps the mental model simple:

- `agent` still manages actual runtime instances
- `hub` manages reusable templates

## API Boundary

Recommended internal ownership:

- `cli/hub`: parse flags, call HTTP API, render output
- `internal/api`: expose `/api/v1/hub/*`
- `internal/hub`: registry resolution, manifest validation, publish/fetch logic
- `internal/agent`: create actual agents and materialize workspaces

Avoid putting template merge or publish logic directly into API handlers.

## Validation Rules

At minimum:

- template name must be non-empty
- `runtime_kind` must be one of supported agent runtime kinds
- `workspace` must contain at least one file, and usually should allow missing `AGENTS.md` but warn in docs
- reject `..`, absolute paths, and unsafe symlinks in workspace content
- remote publish/download size should have reasonable limits

## Suggested Implementation Order

1. Add `hub` config structs, defaults, load/save, and tests.
2. Add `internal/hub` domain types and the `Store` abstraction.
3. Implement local folder store.
4. Implement built-in embedded store.
5. Add `agent create --from-template` and workspace overlay support.
6. Add `template publish`.
7. Add HTTP endpoints for local API.
8. Add remote HTTP store using the same API shape.

This order keeps the first useful version local-first and testable without needing a server-to-server dependency.

## Recommended Rollout Plan

This section is intended to guide incremental code changes with clear checkpoints.

### Step 1: Add config types first

Goal:

- make hub registry configuration representable without changing runtime behavior yet

Code areas:

- `internal/config/config.go`
- `internal/config/config_test.go`
- `docs/tech/config.md`

Changes:

- add `HubConfig`
- add `HubRegistryConfig`
- add `default_publish_registry`
- keep built-in registry behavior available even when `[hub]` is missing

Checkpoint:

- config can load/save hub fields
- existing serve and agent flows remain unchanged

Why first:

- it creates the stable configuration contract before service and CLI code depend on it

### Step 2: Add `internal/hub` model and service skeleton

Goal:

- introduce the hub domain boundary without wiring it into CLI or API yet

Code areas:

- `internal/hub/`

Changes:

- add `Template`, `RegistryRef`, `PublishSpec`
- add `Store` interface
- add `Service` that resolves configured registries and routes read/write operations

Checkpoint:

- pure unit tests can validate registry selection and capability checks
- publish to `builtin` is rejected by service rules

Why now:

- this keeps later CLI/API work thin and avoids pushing hub logic into transport layers

### Step 3: Implement the local store

Goal:

- get the first end-to-end useful backend working without network concerns

Code areas:

- `internal/hub/storelocal/` or `internal/hub/`

Changes:

- implement `List`, `Get`, `Publish`
- persist templates as `agent.toml + workspace/`
- validate template paths and reject unsafe entries

Checkpoint:

- tests can publish a temp workspace into a temp registry and read it back

Why before builtin:

- local store exercises the full read/write lifecycle, while builtin is read-only

### Step 4: Implement workspace copy helpers

Goal:

- make workspace overlay behavior explicit and reusable before wiring template-based creation

Code areas:

- `internal/agent/workspace.go`
- `internal/agent/*_test.go`

Changes:

- extract a reusable helper for copying a filesystem tree into an agent workspace
- support overlay semantics
- enforce path safety and simple file mode handling

Checkpoint:

- tests verify base workspace creation plus overlay result

Why here:

- this is the bridge between hub templates and actual agent creation

### Step 5: Add built-in embedded store

Goal:

- ship a few curated templates with zero user setup

Code areas:

- `internal/hub/embed/...`
- builtin store implementation

Changes:

- embed manifests and workspace trees
- expose them through the same `Store` interface

Checkpoint:

- `Service.List` can aggregate builtin + local results

Why after local:

- local store defines the data shape and avoids designing builtin storage in isolation

### Step 6: Extend agent creation with template input

Goal:

- make hub useful from the existing CLI without adding a large new creation path

Code areas:

- `cli/agent/agent.go`
- `internal/apitypes/agent.go`
- `internal/api/handler.go`
- `internal/agent/service.go`

Changes:

- add `--from-template` to `agent create`
- pass template reference through API request
- resolve template server-side
- create the agent normally, then overlay template workspace
- map template metadata into create spec defaults

Checkpoint:

- `csgclaw agent create --from-template ...` works against builtin/local templates
- normal `agent create` behavior is unchanged

Why server-side resolution:

- avoids duplicating registry access logic in the CLI
- keeps the CLI as a thin API client

### Step 7: Add template list/get API and CLI

Goal:

- let users discover templates before publish support lands

Code areas:

- `cli/hub/`
- `cli/app.go`
- `internal/api/router.go`
- `internal/api/handler.go`

Changes:

- add `csgclaw template list`
- add `csgclaw template get <template>`
- add `GET /api/v1/hub/templates`
- add `GET /api/v1/hub/templates/{id}`

Checkpoint:

- users can see aggregated templates and source registries

Why before publish CLI:

- read-only discovery is simpler and helps validate naming, IDs, and rendering early

### Step 8: Add publish API and CLI

Goal:

- complete the local-first authoring loop

Code areas:

- `cli/hub/`
- `internal/api/handler.go`
- `internal/hub/`
- `internal/agent/`

Changes:

- add `csgclaw template publish --agent <id> --registry <name>`
- add `POST /api/v1/hub/templates`
- snapshot agent workspace and publish into selected writable registry

Checkpoint:

- a user can publish an existing agent to local registry and then create a second agent from it

Why this is a milestone:

- after this step, the feature is already practically complete for single-machine usage

### Step 9: Add remote registry handler/client compatibility

Goal:

- reuse the same hub API shape for cross-machine/team sharing

Code areas:

- remote store client
- local API handlers
- contract tests

Changes:

- implement remote `Store` using HTTP
- add workspace archive upload/download
- add request size limits and auth header support

Checkpoint:

- one CSGClaw instance can publish to and read from another hub-compatible service

Why last:

- remote mode is easiest once local semantics, manifest shape, and API contract are already stable

## Low-Risk Merge Strategy

To keep each change set reviewable, prefer these PR boundaries:

1. config only
2. `internal/hub` types + local store + tests
3. workspace overlay helper + tests
4. builtin store
5. API support for template-based `agent create`
6. `template list/get` CLI + API
7. `template publish` CLI + API
8. remote store + contract tests

Each boundary leaves the repo in a usable state and reduces rollback risk.

## Testing

Recommended tests:

- config load/save for `[hub]`
- local store list/get/publish
- built-in store list/get
- create agent from template overlays workspace correctly
- publish snapshots current workspace into target registry
- remote handler/client contract tests
- path safety tests for malicious workspace entries

Targeted tests should be enough for early iterations. Run `go test ./...` once the feature crosses package boundaries.

## Open Questions

1. Should template identity be `registry/name` only, or include versions later such as `registry/name:version`?
2. Should publish include LLM profile metadata in the future, or keep hub strictly runtime/image/workspace scoped?
3. Should the Web UI expose template selection immediately, or can v1 ship CLI/API first?

## Recommended V1 Summary

The simplest solid v1 is:

- add `internal/hub` with `builtin`, `local`, and later `remote` stores
- store each template as `agent.toml + workspace/`
- extend `agent create` with `--from-template`
- add `csgclaw template list/get/publish`
- overlay hub workspace onto the normal runtime base workspace

That gives users a practical template registry without forcing a large redesign of current agent lifecycle code.
