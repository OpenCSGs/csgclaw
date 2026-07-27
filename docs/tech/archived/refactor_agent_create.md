# Refactor Plan for `agent create --replace`

## Target Semantics

- `agent create`: normal create; fail if the agent already exists.
- `agent create --replace --id <id>`: load the existing agent by `id`; fail if not found; use the existing config as the base; override it with CLI fields explicitly passed this time; then recreate the agent.
- `agent delete <id>`: delete only; it should not participate in recreate flow.

## Recommended Short-Term Design

Keep the replace flow as one create request from the CLI to the API, but keep
field-level merge in the CLI for now.

Rationale:

- The CLI can already tell which flags were explicitly passed by the user.
- The current JSON create request uses normal string zero values, so the service
  cannot reliably distinguish "field omitted" from "field intentionally set to
  empty" without changing the API shape.
- The service should still own the recreate execution because it knows the
  manager and worker runtime differences.
- The CLI should not call `DELETE /api/v1/agents/{id}` before create. A two-call
  delete-then-create flow can leave the system without the original agent if the
  second call fails.

### Short-Term Steps

1. Keep CLI visited-flag tracking.

- keep the current ability to track explicitly passed flags
- for `--replace`, load the existing agent by `id`
- merge the existing config with only the CLI fields explicitly passed this time
- fail directly if the existing agent cannot be loaded

2. Send a single create request with explicit replace intent.

- include `replace: true` in the create request payload
- do not issue a CLI-side delete request
- treat the merged request as the final desired create spec

3. Keep recreate execution in the service.

- API decodes the request and calls the unified create entry
- service handles:
  - `replace=false`: normal create
  - `replace=true`: require an existing agent, then recreate from the final spec
- manager and worker should share the same upper-layer replace entry
- only the final runtime recreate execution may differ internally

4. Keep `agent delete <id>` delete-only.

- delete should not participate in recreate flow
- delete tests should not validate replace behavior

5. Add or update tests for the short-term behavior.

- worker replace loads old config in CLI and sends one create request
- manager replace follows the same API and service entry
- replace fails when the target agent does not exist
- normal create still fails on duplicate agent
- delete remains delete-only

## Future Work

### Cleaner Service Request Shape

`replace` is an operation intent, not an agent property. Keep it out of the
`Agent` entity and consider separating create spec from operation options:

```go
type CreateAgentSpec struct {
	ID          string
	Name        string
	Description string
	Image       string
	Role        string
	Profile     string
	ModelID     string
}

type CreateRequest struct {
	Spec    CreateAgentSpec
	Replace bool
}
```

Alternatively, expose separate service methods:

```go
Create(ctx, CreateRequest)
Replace(ctx, ReplaceRequest)
```

This would make the application-service command boundary clearer while keeping
the persisted/domain `Agent` model free of operation-only fields.

### Field-Mask or Patch-Style API

Implemented with `field_mask` on the create request. The CLI now sends explicit
field intent and the API/service resolves the final replace spec from the
existing agent plus the masked fields before recreating it.

The service now splits replace into two phases:

- resolve final create spec from old config plus explicitly selected new fields
- execute recreate from that final spec

Remaining alternatives for a later API cleanup:

- use pointer fields in the API request DTO
- introduce a dedicated replace/patch request instead of overloading create
