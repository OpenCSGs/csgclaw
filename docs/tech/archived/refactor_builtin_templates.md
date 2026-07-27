# Refactor Builtin Templates

## Goal

Replace the demo data under `internal/hub/embed/templates` with builtin templates derived from `internal/templates/embed/runtimes`.

Recommended direction:

- keep runtime templates in `internal/templates/embed/runtimes/`
- make each runtime template a complete unit: `agent.toml + workspace/`
- let `internal/hub` read builtin templates from embedded runtime data
- remove `internal/hub/embed/templates` in the final state

This document is analysis only. It does not change code.

## Recommended Source Of Truth

Use `internal/templates/embed/runtimes` as the single source of truth.

Suggested layout:

```text
internal/templates/embed/runtimes/
  picoclaw/
    manager/
      agent.toml
      workspace/
    worker/
      agent.toml
      workspace/
  openclaw/
    worker/
      agent.toml
      workspace/
```

Why this is better than keeping a separate copy in `internal/hub/embed/templates`:

- the workspace content already belongs to runtime/bootstrap logic
- `internal/agent` already uses embedded runtime workspace data
- copying the same workspace tree into hub reintroduces duplication
- `agent.toml` in `runtimes` makes each template self-explanatory
- the same shape can later be reused as an example for `local` or `remote` registries

## Why Not Keep `internal/hub/embed/templates`

If builtin templates are derived from `internal/templates/embed/runtimes`, then `internal/hub/embed/templates` becomes redundant.

Final target state:

- keep template content in `internal/templates/embed/runtimes`
- embed runtime templates once
- let hub read builtin templates from that embedded runtime template set
- delete `internal/hub/embed/templates`

The only reason to keep the old hub directory temporarily is migration safety while refactoring.

## Template Format

Each exported runtime template should contain:

- `agent.toml`
- `workspace/`

Suggested meaning:

- `agent.toml` carries hub-facing metadata
- `workspace/` carries the runtime workspace snapshot

This keeps builtin templates, future local templates, and future remote templates conceptually aligned.

Example:

```text
internal/templates/embed/runtimes/picoclaw/worker/
  agent.toml
  workspace/
```

## Why Put `agent.toml` In `internal/templates/embed/runtimes`

This is more direct than keeping metadata separately in Go code.

Benefits:

- easier to understand by reading the filesystem
- `BuiltinStore` can reuse existing manifest loading logic
- easier to document
- easier to use as a sample structure for other registries later

Without `agent.toml`, hub still needs another place to define:

- template ID
- name
- description
- `runtime_kind`
- image

Putting the manifest next to the workspace is cleaner.

## Hub Exposure Control

Even if templates live in `internal/templates/embed/runtimes`, hub should still control which ones are publicly exposed.

Do not assume every directory under `internal/templates/embed/runtimes` is automatically a builtin hub template.

Recommended rule:

- `internal/templates/embed/runtimes` defines available template units
- `internal/hub` decides which units are exposed as builtin templates

This keeps room for:

- internal runtime-only files
- partially implemented runtime templates
- manager-specific templates that may need special handling

## Default Template Selection

To avoid scattering manager/worker default selection rules in code, add default template settings under `[hub]`.

Suggested config:

```toml
[hub]
default_registry = "builtin"
default_publish_registry = "local"
default_manager_template = "builtin/picoclaw-manager"
default_worker_template = "builtin/picoclaw-worker"
```

Recommended meaning:

- `default_manager_template` controls which template is used when the system needs a default manager template
- `default_worker_template` controls which template is used when the system needs a default worker template

These fields should be treated as default selectors only. They should not replace template validation.

Recommended validation:

- the referenced template must exist
- `default_manager_template` must point to a manager template
- `default_worker_template` must point to a worker template
- runtime support constraints must still be checked separately

This keeps responsibilities clean:

- `internal/templates/embed/runtimes` defines template content
- `hub` defines which templates are exposed
- `[hub]` default template settings define which exposed templates are chosen by default

## Current Reality

Today the repository has these runtime workspace roots:

- `internal/templates/embed/runtimes/picoclaw/manager/workspace`
- `internal/templates/embed/runtimes/picoclaw/worker/workspace`
- `internal/templates/embed/runtimes/openclaw/worker/workspace`

It does not have:

- `internal/templates/embed/runtimes/openclaw/manager/workspace`

It also still does not support OpenClaw as the bootstrap manager runtime in config validation today.

So the realistic builtin templates today are:

- `picoclaw-manager`
- `picoclaw-worker`
- `openclaw-worker`

`openclaw-manager` should not be exposed until a real runtime template exists for it.

That also means the safe initial defaults are:

- `default_manager_template = "builtin/picoclaw-manager"`
- `default_worker_template = "builtin/picoclaw-worker"`

## Creation Flow

No major create-flow change is required.

Current flow already works in the right order:

1. load template metadata
2. apply defaults into `CreateAgentSpec`
3. fetch template workspace
4. overlay template workspace onto the runtime base workspace

So the main refactor is about template source and organization, not about changing agent creation semantics.

## Unifying Runtime Template Loading

Under the new design, the duplicated template-entry logic in `internal/agent` should also be unified.

What should be unified:

- template root resolution
- manifest loading
- workspace copying

What should stay explicit:

- manager vs worker selection rules
- runtime support constraints

Recommended direction:

- resolve one runtime template unit by `runtime kind + role`
- read `agent.toml` from that unit
- copy `workspace/` from that unit

This keeps PicoClaw and OpenClaw differences in template data, not in copy-flow code paths.

## Recommended Implementation Direction

### 1. Add `agent.toml` into exported runtime template roots

Add `agent.toml` beside each exported runtime workspace.

Initial targets:

- `internal/templates/embed/runtimes/picoclaw/manager/agent.toml`
- `internal/templates/embed/runtimes/picoclaw/worker/agent.toml`
- `internal/templates/embed/runtimes/openclaw/worker/agent.toml`

### 2. Expand the embedded runtime template source

Make the runtime embed tree include complete template units, not only `workspace/`.

This means `internal/agent` and `internal/hub` can both read from the same embedded runtime template source.

### 3. Introduce unified template resolution in `internal/agent`

Replace scattered template-path constants and special entry points with one resolver based on:

- `runtime kind`
- `role`

The resolver should return one template unit root, then use common logic to load:

- `agent.toml`
- `workspace/`

### 4. Refactor `BuiltinStore`

Make `BuiltinStore` read manifests and workspaces from embedded runtime templates instead of `internal/hub/embed/templates`.

Hub should keep explicit control over which runtime templates are exposed as builtin templates.

### 5. Add `[hub]` default template fields

Extend config with:

- `default_manager_template`
- `default_worker_template`

Update together:

- config structs
- loader and saver
- defaults and validation
- tests
- docs

### 6. Switch default flows to config-driven template selection

Where the system currently relies on implicit default selection, use:

- `default_manager_template`
- `default_worker_template`

Validation should fail clearly if these point to missing or incompatible templates.

### 7. Remove demo hub templates

Delete:

- `internal/hub/embed/templates`

after the runtime-derived path is fully in place.

## Suggested Execution Steps

Recommended order:

1. Add `agent.toml` to the existing embedded runtime template roots.
2. Update runtime embedding so complete template units are embedded.
3. Introduce a shared runtime-template resolver in `internal/agent`.
4. Refactor `BuiltinStore` to read builtin templates from embedded runtime templates.
5. Add `[hub].default_manager_template` and `[hub].default_worker_template`.
6. Wire manager/worker default template selection to those config fields.
7. Remove `internal/hub/embed/templates`.
8. Update tests, config docs, and any hub/user-facing docs together.

Recommended verification when implementing:

1. targeted tests for `internal/config`
2. targeted tests for `internal/hub`
3. targeted tests for `internal/agent`
4. `go test ./...` once cross-package behavior is wired together

## Recommendation

Use `internal/templates/embed/runtimes` as the single source of truth, and make each exported runtime template a complete template unit with:

- `agent.toml`
- `workspace/`

Do not keep a second full copy under `internal/hub/embed/templates`.

This is the simplest long-term structure and the best base for future builtin, local, and remote template consistency.
