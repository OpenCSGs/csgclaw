# Control-plane API

The [OpenAPI 3.1.1 definition](../../api/openapi/control-plane.json) describes Agent administration, Runtime installation, model providers, Skills, MCP, knowledge bases and templates. Paths in the definition are relative to `/api/v1`; configure the existing instance prefix for platform deployments.

Send the current server, platform or Desktop credential in `Authorization: Bearer <token>`. Agent credentials retain their existing resource restrictions. The MCP file bridge requires its existing credential bound to an Agent and server. Authentication and login behavior remain owned by the existing listeners and middleware.

## Agent configuration

`POST /agents` creates an Agent and returns `201`. `GET /agents` returns `{ "items": [...] }`; `GET /agents/{id}` and lifecycle operations return an Agent object. Runtime selection, options, image and observed state are grouped under `runtime`. Model configuration is under `model_config`, and Agent MCP configuration is under `mcp_servers`.

```json
{
  "name": "reviewer",
  "runtime": {
    "name": "codex",
    "sandbox_enabled": false,
    "options": {}
  },
  "model_config": {
    "selector": "provider.model"
  },
  "mcp_servers": {}
}
```

`from_template` selects a template during creation. Current Manager restrictions, name validation and template resource availability checks apply. Creation may include `skipped_resources` in the response. Creation assigns its time and starts the Agent according to the existing service rules.

`POST /channels/{channel}/participants` uses the same `runtime`, `model_config` and `mcp_servers` names inside `agent_binding.agent`. Its existing create/reuse modes and Participant response remain unchanged. An explicit `skills` array replaces the template selection, including `[]`; omission retains the template defaults.

`PATCH /agents/{id}` updates submitted fields:

| Input | Meaning |
| --- | --- |
| Omitted field | Retain its current value |
| `name`, `description`, `instructions`, `runtime`, or `model_config` set to `null` | Retain its current value |
| `description: ""` or `instructions: ""` | Clear the value |
| `runtime.options: {}` | Clear options; a nonempty map replaces options |
| `runtime.options: null` | Retain options |
| `mcp_servers: null` or `{}` | Clear MCP configuration |
| `model_config: {"selector": "provider.model"}` | Update only model selection |
| Other submitted `model_config` fields | Use existing whole-profile validation and update semantics |

`PATCH` accepts no `field_mask`. Agent creation, patch and recreate requests reject unsupported fields and multiple JSON values. Runtime options and MCP configuration keep their extension keys. Read-only state and plaintext secret output are excluded from writable schemas.

`GET` and `PUT /agents/{id}/profile` retain the effective-profile read and whole-profile update operations. Both include configured image and video model selections. API keys are represented by their current set/preview fields in responses. The OpenAPI input schemas distinguish the fields accepted by creation from the additional fields supported by profile updates and Participant creation.

## Recreate and lifecycle

`POST /agents/{id}/recreate` returns `200`. An empty body or `{}` rebuilds with the current configuration. `configuration` without a nonempty `field_mask` uses full replacement rules, including the existing Runtime/model retention behavior and retained role. A nonempty mask replaces only its selected fields:

```json
{
  "configuration": {
    "description": "",
    "runtime": {"options": {}}
  },
  "field_mask": ["description", "runtime.options"]
}
```

Allowed masks are `name`, `description`, `instructions`, `runtime`, `runtime.image`, `runtime.options`, `model_config`, `model_config.selector`, and `mcp_servers`. A nonempty mask requires `configuration`; parent and child paths cannot be combined. Invalid masks return `400 invalid_field_mask`. Recreate uses the existing running-state rule. Template selection belongs to creation.

Start, stop, upgrade, archive, restore, archive-impact, instructions, Memory and log operations retain their business behavior. Archive responses include `archived_by`. Restore starts the Runtime and preserves the disabled state of archived schedules. Permanent deletion requires an archived Agent and the exact name in the existing JSON body `{ "name": "reviewer" }`.

## Collections and resource pagination

The following ordinary collections return `items`:

- Agents, Runtime directory and image candidates.
- Model providers, provider presets and model discovery (`provider` remains alongside `items`).
- Global Skills, discovery rescan and unpaged Agent Skill summaries.
- Template directories and template version history.

Paged resource responses retain `items`, `total`, `list_revision`, `has_more` and applicable page/cursor fields. Use `limit` for page size, from 1 to 100. Local resource pages default to 20. Remote Skill, MCP and knowledge-base pages default to 16, 12 and 20 respectively. Remote totals and next-page markers remain optional where the upstream result makes them optional.

```text
GET /agents/{id}/skill-summaries?pagination=page&page=1&limit=20
GET /agents/{id}/resources?pagination=cursor&limit=20&type=skill
```

`/agents/{id}/resources` requires both `type=skill|connector|knowledge_base` and `pagination=page|cursor`. Skill summaries and Agent MCP listings also support their existing complete-list form. Cursor and page parameters are mutually constrained as described in OpenAPI. Preserve `ETag` and submit the current `resource_version` when toggling resource enablement. A changed list or resource version retains its existing conflict code.

MCP configuration remains a dictionary: global responses use `mcpServers`; Agent MCP listing and mutation responses use `agent_id`, `runtime_kind` and `servers`. Source synchronization returns the corresponding configuration view. Tree listings, files, model capability checks and protocol responses retain their separate structures.

## Skills and templates

`POST /skills:upload` accepts a `multipart/form-data` field named `file`. The ZIP may be up to 64 MiB; the multipart envelope has a separate bounded allowance. Successful installation returns `201`; an oversized upload returns `413 payload_too_large`. Existing archive validation and duplicate handling apply.

Template listings accept the current `scope=local|remote` filter. Template responses preserve revision numbers, version counts and publication records. `DELETE /hub/templates/{id}/versions/{revision}` deletes an eligible historical revision; deleting the current revision is rejected.

`POST /hub/templates` uses the existing `template_id` publication branch when supplied. Otherwise it saves a snapshot from `agent_id`. `expected_revision` applies to publication; local/official snapshot saving follows source-Agent identity. `replace_template_id` applies to the custom registry branch. Workspace file reads, edits, version inspection and restore retain their existing behavior.

## Model and MCP protocols

The model proxy has two scopes:

```text
/agents/{id}/llm/v1
/agents/{id}/rooms/{room_id}/llm/v1
```

Each supports `GET /models`, `POST /chat/completions`, `POST /responses`, and `GET /responses` for WebSocket sessions. Room requests resolve the Room configuration and verify Agent membership. Runtime BaseURLs include `/v1`; clients append the model endpoint once. JSON, SSE, WebSocket, tool calls and protocol failures retain the current formats. POST model request bodies retain their 128 MiB bound.

The MCP file bridge registers `POST`. Knowledge-base and OpenCSG MCP proxies register `GET`, `POST` and `DELETE`, preserving their protocol bodies, session headers and streams.

## Errors and client generation

Ordinary operations described here return business failures as:

```json
{"error":{"code":"invalid_request","message":"..."}}
```

Existing business codes, status codes, retry headers and additional failure information are preserved. Authentication, model/MCP protocols and errors after a successful stream begins retain their existing formats. The endpoint-specific OpenAPI descriptions identify these cases.

Regenerate the frontend boundary types with:

```sh
pnpm --dir web/packages/app generate:api
```

The frontend API layer adapts those definitions to the existing page models. HTTP DTOs map to current service requests; the service records, saved file fields and directory layout remain unchanged. `TestControlPlaneOpenAPIMatchesRoutes` checks registered operations against the definition in both directions for ordinary control operations.
