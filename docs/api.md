# CSGClaw API Documentation

This document is generated from the HTTP routes and behaviors currently implemented in the codebase. The default example server address is `http://127.0.0.1:18080`.

## Conventions

- Except for streaming endpoints, requests and responses use `application/json`
- Time fields use RFC3339 / ISO8601
- Most non-streaming errors are returned as plain-text response bodies
- SSE endpoints use `text/event-stream`
- The current API is mainly grouped into 3 areas:
  - Core API: `/api/v1/*`
  - Channel and participant API: `/api/v1/channels/*`
  - Health check: `/healthz`

CSGClaw skill scripts can emit resource links and interactive questions through the runtime-neutral [CSGClaw Structured Skill Output Protocol](structured-output.md).
That protocol ultimately submits question responses to the channel activity endpoint documented below.

## Activity API

### `POST /api/v1/channels/{channel}/activities/{activity_id}:respond`

Answers a pending structured question activity.
The body is the exact `RequestUserInputResponse` object:

```json
{
  "answers": {
    "verification": {
      "answers": ["Standard (Recommended)"]
    },
    "note": {
      "answers": []
    }
  }
}
```

An empty outer `answers` object skips the entire request.
An empty inner `answers` array skips one question.
The server derives the room and responder from the stored activity and rejects unknown fields.

## Authentication

- Most `/api/v1/*` endpoints do not require authentication by default
- The following endpoints require `Authorization: Bearer <token>`, where the token is the server access token:
  - `GET /api/v1/channels/{channel}/participants/{id}/events`
  - `POST /api/v1/channels/csgclaw/participants/{id}/messages`
  - `GET /api/v1/agents/{id}/llm/models`
  - `GET /api/v1/agents/{id}/llm/v1/models`
  - `POST /api/v1/agents/{id}/llm/chat/completions`
  - `POST /api/v1/agents/{id}/llm/v1/chat/completions`
  - `POST /api/v1/agents/{id}/llm/responses`
  - `POST /api/v1/agents/{id}/llm/v1/responses`
  - `GET /api/v1/agents/{id}/llm/responses`
  - `GET /api/v1/agents/{id}/llm/v1/responses`
- If the server runs with `no_auth`, the checks above are skipped

## Health Check

### `GET /healthz`

Health check endpoint.

Example response:

```text
ok
```

## Core API

### `GET /api/v1/version`

Returns the current server version.

Example response:

```json
{
  "version": "0.1.0"
}
```

### Upgrade

#### `GET /api/v1/upgrade/status`

Returns the upgrade status.

Response fields:

- `current_version`
- `latest_version`
- `update_available`
- `checking`
- `upgrading`
- `last_checked_at`
- `last_error`

#### `POST /api/v1/upgrade/apply`

Starts the upgrade helper.

On success, returns `202 Accepted`:

```json
{
  "status": "accepted",
  "message": "upgrade helper started"
}
```

Returns `503 Service Unavailable` if the upgrade manager is not configured.

## Participant API

Participants are channel-scoped identities used by rooms, messages, mentions,
notifications, and runtime bridges. A participant can represent a human, an
agent-backed channel identity, or a notification sender.

### `GET /api/v1/channels/{channel}/participants`

Returns participants for the specified channel.

Path parameters:

- `channel`: `csgclaw` or `feishu`

Optional query parameters:

- `type`: `human`, `agent`, or `notification`
- `agent_id`

Response fields:

- `id`
- `channel`
- `type`
- `name`
- `avatar`
- `channel_user_ref`
- `channel_user_kind`
- `channel_app_ref`
- `agent_id`
- `lifecycle_status`
- `presence`
- `mentionable`
- `metadata`
- `created_at`
- `updated_at`

Examples:

- `GET /api/v1/channels/csgclaw/participants`
- `GET /api/v1/channels/csgclaw/participants?type=notification`
- `GET /api/v1/channels/feishu/participants?agent_id=u-worker`

### `POST /api/v1/channels/{channel}/participants`

Creates a participant in the specified channel.

Path parameters:

- `channel`: `csgclaw` or `feishu`

Example request body:

```json
{
  "id": "qa",
  "type": "agent",
  "name": "QA",
  "channel_user": {
    "ref": "u-qa",
    "kind": "local_user_id"
  },
  "agent_binding": {
    "mode": "create",
    "agent": {
      "name": "QA",
      "role": "worker",
      "runtime_kind": "picoclaw_sandbox",
      "from_template": "builtin.openclaw-worker"
    }
  }
}
```

Notes:

- `type` is required and must be `human`, `agent`, or `notification`
- `name` is required
- The effective channel comes from the route path rather than the request body
- `agent` participants can create or reuse an Agent through `agent_binding`
- `human` and `notification` participants do not create runtime agents
- In the example above, `qa` is the participant ID; `u-qa` is used only as the local channel user ref and generated backing agent ID.
- For `csgclaw`, `channel_user.ref` is a local IM user ID
- For `feishu`, `channel_user.ref` is the channel-native open ID

Examples:

- `POST /api/v1/channels/csgclaw/participants`
- `POST /api/v1/channels/feishu/participants`

### `GET /api/v1/channels/{channel}/participants/{id}`

Returns one participant.

### `PATCH /api/v1/channels/{channel}/participants/{id}`

Updates editable participant fields such as `name`, `avatar`, `mentionable`, and
`metadata`.

### `DELETE /api/v1/channels/{channel}/participants/{id}`

Deletes the specified participant in the specified channel.

Returns `204 No Content` on success.

Examples:

- `DELETE /api/v1/channels/csgclaw/participants/qa`
- `DELETE /api/v1/channels/feishu/participants/qa`

## Agent API

### Agent Response Shape

`/api/v1/agents*` returns agent objects with fields like:

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "runtime_id": "codex",
  "runtime_kind": "codex",
  "image": "example/image:latest",
  "box_id": "codex-session-alice",
  "role": "worker",
  "status": "running",
  "created_at": "2026-05-16T08:00:00Z",
  "profile": "api.gpt-5.4",
  "runtime_options": {},
  "mcpServers": {
    "workspace-filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  },
  "agent_profile": {
    "provider": "api",
    "base_url": "https://api.example.com/v1",
    "api_key_set": true,
    "api_key_preview": "sk-1...",
    "model_id": "gpt-5.4",
    "reasoning_effort": "medium",
    "profile_complete": true
  },
  "profile_complete": true,
  "detection_results": []
}
```

Notes:

- The real `api_key` is never returned in `agent_profile`
- `runtime_options` is sanitized before being exposed by the API
- `mcpServers` is a direct server-name-to-server-config map. Generic Agent
  responses redact secret values in server `env` and `headers` maps.
- `profile` is the normalized selector generated by the server, for example `api.gpt-5.4`
- `detection_results` is used to present default profile detection results

### `GET /api/v1/agents`

Lists all agents.

The server reloads state before returning the latest snapshot.

### `POST /api/v1/agents`

Creates an agent.

Request fields:

- `id`
- `name`
- `description`
- `image`
- `runtime_kind`
- `from_template`
- `replace`
- `field_mask`
- `role`
- `status`
- `created_at`
- `profile`
- `runtime_options`
- `mcpServers`
- `agent_profile`

Example request body:

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "runtime_kind": "codex",
  "profile": "api.gpt-5.4",
  "agent_profile": {
    "provider": "api",
    "base_url": "https://api.example.com/v1",
    "api_key": "sk-xxx",
    "model_id": "gpt-5.4",
    "reasoning_effort": "medium"
  }
}
```

Additional notes:

- `name` is required
- `replace=true` enables replace logic
- `field_mask` limits which fields are overwritten during replacement
- `agent_profile.api_key` is write-only and will be redacted in reads

OpenClaw, PicoClaw, and Codex CLI agents configure MCP servers through the
top-level `mcpServers` field. Its value is the direct server-name-to-server-
config map used by every supported runtime:

```json
{
  "name": "alice",
  "runtime_kind": "openclaw_sandbox",
  "mcpServers": {
    "workspace-filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/home/node/.openclaw/workspace"
      ]
    }
  },
  "profile": "api.gpt-5.4"
}
```

Runtime adapters map this shared input as follows:

- OpenClaw: `mcpServers` -> `openclaw.json` `mcp.servers`
- PicoClaw: `mcpServers` -> PicoClaw config `tools.mcp.servers`
- Codex CLI: `mcpServers` -> managed `[mcp_servers."<name>"]` blocks in the isolated Codex home `config.toml`

MCP commands run inside the target runtime environment, so filesystem server
directory arguments must be paths visible to that runtime. MCP-shaped values
under `runtime_options` (including `mcp` and `mcpServers`) are rejected; use
the top-level `mcpServers` field instead.

### `GET /api/v1/agents/{id}`

Gets a single agent.

Returns `404` if it does not exist.

### `PATCH /api/v1/agents/{id}`

Updates basic agent fields.

Supported fields:

- `name`
- `description`
- `image`
- `runtime_options`
- `mcpServers`
- `agent_profile`

Example request body:

```json
{
  "description": "updated description",
  "runtime_options": {
    "sandbox": "default"
  }
}
```

Notes:

- Omitted fields are left unchanged
- `runtime_options` uses whole-object replacement when submitted
- `mcpServers` uses whole-map replacement when submitted; send `null` to clear
  the CSGClaw-managed server set
- Updating MCP servers on OpenClaw, PicoClaw, or Codex CLI agents may recreate
  that agent runtime so the native configuration takes effect
- If `agent_profile.api_key` is sent empty, the server keeps the existing key
- If `agent_profile.env` changes, `env_restart_required` may become `true` in the response

### Agent MCP server endpoints

#### `GET /api/v1/agents/{id}/mcp-servers`

Returns the agent's MCP server configuration as a direct raw
server-name-to-server-config map in `servers`. Unlike generic Agent responses,
this endpoint does not redact configured token values.

```json
{
  "agent_id": "u-alice",
  "runtime_kind": "openclaw_sandbox",
  "servers": {
    "context7": {
      "command": "uvx",
      "args": ["context7-mcp"],
      "env": { "CONTEXT7_API_KEY": "secret" }
    }
  }
}
```

#### `POST /api/v1/agents/{id}/mcp-servers:batchAdd`

Accepts `{ "names": ["..."] }` and merges the named server definitions from
the MCP catalog into the agent's managed MCP servers. The response has the same
shape as the `GET` endpoint.

#### `POST /api/v1/agents/{id}/mcp-servers:batchDelete`

Accepts `{ "names": ["..."] }` and removes the named servers from the
agent's managed MCP servers. The response has the same shape as the `GET`
endpoint.

### MCP server catalog

The reusable MCP server catalog remains at `/api/v1/mcp-servers`:

- `GET /api/v1/mcp-servers` lists catalog servers
- `POST /api/v1/mcp-servers` creates a catalog server
- `PUT /api/v1/mcp-servers/{name}` replaces one catalog server
- `DELETE /api/v1/mcp-servers/{name}` deletes one catalog server

Catalog list and mutation responses use `mcpServers` as the direct server map.
Creating or replacing one catalog entry uses `{ "name": "...", "config": { ... } }`;
`config` is that entry's server configuration.

### `DELETE /api/v1/agents/{id}`

Deletes an agent.

Returns `204 No Content` on success.

### `POST /api/v1/agents/{id}/start`

Starts the agent and returns the updated agent object.

### `POST /api/v1/agents/{id}/stop`

Stops the agent and returns the updated agent object.

### `GET /api/v1/agents/{id}/logs`

Returns agent logs.

Query parameters:

- `lines`: defaults to `20`
- `follow`: `1/true/yes/on` enables streaming follow mode

Response type: `text/plain; charset=utf-8`

Notes:

- With `follow=false`, errors are returned as normal HTTP errors
- With `follow=true`, streaming-time errors may be written into the response body

### `GET /api/v1/agents/{id}/profile`

Returns the redacted profile for a single agent.

### `PUT /api/v1/agents/{id}/profile`

Replaces the profile for a single agent.

The request body uses the `agent_profile` shape, for example:

```json
{
  "provider": "api",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx",
  "model_id": "gpt-5.4",
  "reasoning_effort": "medium",
  "headers": {
    "x-org": "demo"
  },
  "env": {
    "FOO": "bar"
  }
}
```

Notes:

- Unlike `PATCH /api/v1/agents/{id}`, this endpoint semantically replaces the current profile with the new one
- If `api_key` is empty, the server keeps the existing key

### `POST /api/v1/agents/{id}/recreate`

Recreates the agent using its current configuration and returns the new agent state.

Common failures:

- `404`: agent does not exist
- `400`: profile is incomplete, or the runtime does not allow recreation

## Agent Profile Helper API

### `POST /api/v1/agent-profiles/models`

Returns the available model list for the given provider configuration.

Request fields:

- `agent_id`
- `provider`
- `base_url`
- `api_key`
- `headers`

Example request body:

```json
{
  "provider": "api",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx"
}
```

Example response:

```json
{
  "provider": "api",
  "models": ["gpt-5.4", "gpt-5.4-mini"]
}
```

Notes:

- For `provider=codex` or `claude_code`, model choices are obtained through CLIProxy
- For `provider=api`, the server calls the target OpenAI-compatible `/models` endpoint
- If `agent_id` is provided and `api_key` is omitted in the request, the server may reuse the saved key from that agent
- If `agent_id` is omitted and `api_key` is omitted, the server may reuse the saved default API key only when `provider=api` and `base_url` matches the current default profile

### `GET /api/v1/agent-profile-defaults`

Returns the redacted view of the current default agent profile.

This is commonly used by the frontend to initialize default provider/model state.

## Hub Template API

Templates use the following layout:

```text
<template>/
  agent.toml
  instructions/AGENTS.md
  skills/<skill>/...
  mcps/mcp.json
  memories/MEMORY.md
```

`AGENTS.md` and `mcp.json` are always emitted when publishing. Other instruction and memory files are optional. During agent creation, instruction files and memories are overlaid onto the runtime workspace, skills are installed under `skills/`, and MCP servers from `mcp.json` are applied unless the create request explicitly supplies `mcpServers`.

### `GET /api/v1/hub/templates`

Lists all templates from readable registries.

Response fields:

- `id`
- `name`
- `description`
- `runtime_kind`
- `image`
- `updated_at`
- `source.name`
- `source.kind`
- `workspace.kind`

### `POST /api/v1/hub/templates`

Publishes an existing agent workspace to the hub.

Request body:

```json
{
  "agent_id": "u-alice",
  "registry": "local"
}
```

Notes:

- `agent_id` is required
- `registry` uses the default publish registry when omitted
- Successful publish returns `201 Created`

### `GET /api/v1/hub/templates/{id}`

Returns template details.

In addition to the list view fields, this endpoint also returns:

- `workspace.entries`

Example `workspace.entries` payload:

```json
{
  "workspace": {
    "kind": "dir",
    "entries": [
      {"path":"agent.toml","name":"agent.toml","type":"file","depth":0,"size":128},
      {"path":"instructions","name":"instructions","type":"dir","depth":0,"size":0}
    ]
  }
}
```

### `GET /api/v1/hub/templates/{id}/workspace/file?path=...`

Reads a single file preview from the template workspace.

Query parameters:

- `path`: required relative path

Response fields:

- `path`
- `content`
- `size`
- `truncated`
- `binary`

Notes:

- Non-UTF-8 files return `binary=true`
- Text content larger than `256 KiB` is truncated and returned with `truncated=true`
- Absolute paths and `..` traversal are rejected

## CLIProxy Auth API

### `GET /api/v1/cliproxy/auth/status?provider=...`

Returns the local auth status for a provider.

`provider` is required.

The response is provided by CLIProxy and commonly includes:

- `provider`
- `authenticated`
- `login_required`
- `message`
- `supports_login`

### `POST /api/v1/cliproxy/auth/login`

Triggers provider login.

Request body:

```json
{
  "provider": "codex",
  "no_browser": true
}
```

Returns the current provider auth status on success.

Notes:

- Missing `provider` returns `400`
- Login failure returns `502 Bad Gateway`

## Bootstrap Config API

### `GET /api/v1/config/bootstrap`

Returns the bootstrap config view.

Response fields:

- `default_manager_template`
- `default_worker_template`
- `runtime_kind`
- `effective_manager_image`
- `supported_runtime_kinds`
- `runtime_default_images`

### `PUT /api/v1/config/bootstrap`

Updates the default bootstrap templates.

Request body:

```json
{
  "default_manager_template": "builtin.manager",
  "default_worker_template": "local.review-bot"
}
```

Notes:

- Both fields are optional
- The updated config is validated after modification
- If the default templates change and the agent service is mounted, the gateway runtime is updated as well

## Local IM API

These endpoints expose CSGClaw local IM data.

For the thread model, invariants, hidden context behavior, and bridge rules, see
[im-threads.md](./im-threads.md).

### `GET /api/v1/bootstrap`

Returns IM bootstrap data.

Response fields:

- `current_user_id`
- `users`
- `rooms`
- `invite_draft_user_ids`

Room message lists in the bootstrap response follow the default timeline
contract: top-level messages only; thread replies are hidden.

### `GET /api/v1/events`

Subscribes to the local IM event stream.

Returns `text/event-stream`. After the connection is established, the server first writes:

```text
: connected
```

Then it streams JSON events as SSE `data:` frames. The heartbeat frame is:

```text
: ping
```

Currently observed event types include:

- `message.created`
- `room.created`
- `room.members_added`
- `thread.created`
- `thread.updated`
- `upgrade.status_changed`

Event JSON fields:

- `type`
- `room_id`
- `room`
- `user`
- `message`
- `thread`
- `sender`
- `upgrade`

### `GET /api/v1/users`

Lists local IM users.

### `POST /api/v1/users`

Creates a local IM user.

Request body:

```json
{
  "id": "alice",
  "name": "Alice",
  "role": "worker"
}
```

Notes:

- `id` is required
- `name` is required
- For `worker` or `agent` roles, if participant and agent services are both enabled, prefer the participant API for agent-backed identities

### `DELETE /api/v1/users/{id}`

Deletes a local IM user.

Deleting a user also rebuilds thread state from the surviving room messages.
Thread roots sent by the deleted user are removed, hidden context snapshots are
regenerated without deleted-user messages, and surviving thread creation times
are preserved where possible.

Common responses:

- `204`: deleted successfully
- `404`: user not found
- `409`: attempted to delete the current user

### `GET /api/v1/rooms`

Lists local IM rooms.

Room message lists exclude thread replies by default. Root messages still expose
their thread summaries when a thread exists.

### `POST /api/v1/rooms`

Creates a room.

Request body:

```json
{
  "title": "Launch",
  "description": "coordination",
  "creator_id": "manager",
  "member_ids": ["alice", "bob"],
  "locale": "en"
}
```

Compatibility field:

- Legacy `participant_ids` is still accepted and mapped to `member_ids`

### `DELETE /api/v1/rooms/{id}`

Deletes a room and returns `204` on success.

### `GET /api/v1/rooms/{id}/members`

Lists room members.

### `POST /api/v1/rooms/{id}/members`

Adds members to the specified room.

Request body:

```json
{
  "inviter_id": "manager",
  "user_ids": ["bob"],
  "locale": "en"
}
```

Notes:

- The path `{id}` is used as `room_id`
- If `room_id` is also present in the body, it must match the path value

### `POST /api/v1/rooms/invite`

Adds members by room ID, with semantics similar to `POST /api/v1/rooms/{id}/members`.

Request body:

```json
{
  "room_id": "room-1",
  "inviter_id": "manager",
  "user_ids": ["bob"],
  "locale": "en"
}
```

### `GET /api/v1/messages?room_id=...`

Returns the message list for the specified room.

`room_id` is required.

By default, thread replies are excluded from the room timeline. Add
`include_thread_replies=true` to include threaded replies in the returned
message list.

### `POST /api/v1/messages`

Sends a message.

Request body:

```json
{
  "room_id": "room-1",
  "sender_id": "manager",
  "content": "hello @alice",
  "mention_id": "alice"
}
```

Notes:

- `room_id` is required
- Returns `201 Created` on success
- A successful send also publishes `message.created` to `/api/v1/events`
- To send a thread reply, include `relates_to: {"rel_type":"m.thread","event_id":"<root_message_id>"}`
- `relates_to.rel_type` currently supports `m.thread`; the root must be a
  top-level message in the same room
- A thread reply also publishes `thread.updated`
- To send attachments, use `multipart/form-data` with a `payload` JSON part containing the same fields and one or more `files` parts.
- Attachment-only messages are valid when at least one file is present.
- Each returned message can include `attachments` with `id`, `name`, `kind`, `media_type`, `size_bytes`, `sha256`, `created_at`, `download_url`, optional `preview_url`, optional image dimensions, and optional `workspace_path` for agent-facing deliveries.

Multipart example:

```text
payload={"room_id":"room-1","sender_id":"manager","content":""}
files=@diagram.png;type=image/png
```

### `GET /api/v1/attachments/{id}`

Downloads a stored chat attachment by attachment ID.

The `download_url` returned in attachment metadata includes an attachment-scoped capability token and can be used directly by browsers and agents.

Callers may instead request the bare path with the configured server Bearer token.

The endpoint serves the original bytes with the stored media type and `X-Content-Type-Options: nosniff`.

Treat the capability URL as a secret and avoid sharing it outside the room context.

Image attachments are served inline.

Other file attachments are served with attachment disposition.

### `POST /api/v1/rooms/{id}/threads`

Starts a thread from an existing top-level message. The thread identity is the
root message ID, matching Matrix `m.thread` relationship semantics without
using the raw `/_matrix` namespace.

Request body:

```json
{
  "root_message_id": "msg-root"
}
```

Responses:

- `201 Created`: a new thread state was created
- `200 OK`: the thread already existed and was returned idempotently

The response is a `ThreadView`:

```json
{
  "room_id": "room-1",
  "root": { "id": "msg-root" },
  "context": [],
  "replies": [],
  "summary": {
    "root_id": "msg-root",
    "reply_count": 0,
    "participants": [],
    "current_user_participated": true,
    "context_summary": {
      "root_excerpt": "root text",
      "message_count": 1,
      "before_count": 0,
      "after_count": 0
    }
  }
}
```

`ThreadView.root` is the visible root message, `context` is the hidden snapshot
for LLM context, `replies` is the visible thread reply list, and `summary` is
the root-level thread summary used by timelines and thread lists.

Thread context is snapshotted when the thread starts: up to five top-level
messages before the root, the root message, and up to two top-level messages
after it, capped by payload size. This context is not rendered as thread
messages; it is hidden context for LLM-backed agents so a thread can begin with
a clean conversation while still understanding what it was started from.

### `GET /api/v1/rooms/{id}/threads?include=all|participated&limit=&from=`

Lists room threads. `include` defaults to `all`; `participated` returns only
threads where the current user is the root sender or a reply participant.
`limit` and `from` implement offset-style pagination.

### `GET /api/v1/rooms/{id}/threads/{root_message_id}`

Returns one `ThreadView`, including the root message, hidden context window,
replies, and summary.

### `GET /api/v1/rooms/{id}/relations/{event_id}/m.thread`

Returns Matrix-style child events for a thread root:

```json
{
  "chunk": []
}
```

## Channel API

## `csgclaw` Channel

`/api/v1/channels/csgclaw/*` is essentially a mirrored entrypoint for the local IM API.

### Users

- `GET /api/v1/channels/csgclaw/users`
- `POST /api/v1/channels/csgclaw/users`
- `DELETE /api/v1/channels/csgclaw/users/{id}`

Notes:

- `GET` and `POST` reuse the local IM user logic
- `DELETE` uses channel-specific delete handling, but still semantically deletes the local user

### Rooms

- `GET /api/v1/channels/csgclaw/rooms`
- `POST /api/v1/channels/csgclaw/rooms`
- `DELETE /api/v1/channels/csgclaw/rooms/{id}`
- `GET /api/v1/channels/csgclaw/rooms/{id}/members`
- `POST /api/v1/channels/csgclaw/rooms/{id}/members`
- `POST /api/v1/channels/csgclaw/rooms/{id}/threads`
- `GET /api/v1/channels/csgclaw/rooms/{id}/threads`
- `GET /api/v1/channels/csgclaw/rooms/{id}/threads/{root_message_id}`
- `GET /api/v1/channels/csgclaw/rooms/{id}/relations/{event_id}/m.thread`

### Messages

- `GET /api/v1/channels/csgclaw/messages?room_id=...`
- `POST /api/v1/channels/csgclaw/messages`

## `feishu` Channel

### Feishu Credentials

`/api/v1/channels/feishu/config` was removed. Feishu credentials are part of
the Feishu participant object (`channel_app_config`) and are set via participant
binding (`participant bind`) or participant update APIs.

In participant responses, the secret is masked as `present`.

### Participant Events

#### `GET /api/v1/channels/feishu/participants/{id}/events`

Subscribes to mention events for the specified participant in Feishu.

Characteristics:

- Requires Bearer Token
- Returns `text/event-stream`
- Only forwards events whose message mentions the participant open_id
- Writes `: connected` immediately after the stream is established

### Users

- `GET /api/v1/channels/feishu/users`
- `POST /api/v1/channels/feishu/users`
- `DELETE /api/v1/channels/feishu/users/{id}`

Example `POST` body:

```json
{
  "id": "ou_xxx",
  "name": "Alice",
  "role": "member",
  "avatar": "AL"
}
```

### Rooms

- `GET /api/v1/channels/feishu/rooms`
- `POST /api/v1/channels/feishu/rooms`
- `DELETE /api/v1/channels/feishu/rooms/{id}`
- `GET /api/v1/channels/feishu/rooms/{id}/members`
- `POST /api/v1/channels/feishu/rooms/{id}/members`

Room creation and member-addition requests are mostly aligned with local IM and still use:

- `title`
- `description`
- `creator_id`
- `member_ids`
- `locale`

Example add-members request:

```json
{
  "inviter_id": "manager",
  "user_ids": ["dev"],
  "locale": "zh-CN"
}
```

### Messages

- `GET /api/v1/channels/feishu/messages?room_id=...`
- `POST /api/v1/channels/feishu/messages`

Example send-message request:

```json
{
  "room_id": "oc_xxx",
  "sender_id": "manager",
  "content": "hello",
  "mention_id": "worker"
}
```

## Runtime Bridge API

Runtime clients use participant-scoped routes for channel messages and
agent-scoped routes for LLM provider traffic. The legacy `/api/bots/*` routes
are not registered.

For thread/session isolation rules used by runtime and Codex bridges, see
[im-threads.md](./im-threads.md).

### `GET /api/v1/channels/{channel}/participants/{id}/events`

Subscribes to the participant event stream.

Characteristics:

- Requires Bearer Token
- Returns `text/event-stream`
- Writes `: connected` immediately after the stream is established
- Uses `: heartbeat` as the heartbeat comment
- Uses `message` as the SSE event name
- If the client sends `Last-Event-ID`, the server may replay recent messages according to the replay rules

Example single event:

```text
id: msg-1
event: message
data: {"message_id":"msg-1","room_id":"room-1","channel":"csgclaw","chat_id":"room-1","sender_id":"admin","text":"hello","thread_root_id":"msg-root","context":{"channel":"csgclaw","chat_id":"room-1","chat_type":"direct","topic_id":"msg-root","sender_id":"admin","message_id":"msg-1"},"thread_context":{"root_message_id":"msg-root","context":[{"id":"msg-root","sender_id":"admin","content":"root text"}],"summary":{"root_excerpt":"root text","message_count":1,"before_count":0,"after_count":0}}}
```

For thread replies, `thread_root_id` is the root message ID and
`thread_context` carries the deterministic hidden context captured when the
thread was started. Runtime/LLM bridges use it as prompt context; it is not a list
of thread replies. PicoClaw-native clients can use `context.topic_id` as the
same thread/session identifier.

Events can include an `attachments` array with the same message attachment metadata returned by the message APIs.

For CSGClaw agents, the server also attempts to copy each attachment into the target agent workspace and sets `workspace_path` when that copy succeeds.

### `POST /api/v1/channels/csgclaw/participants/{id}/messages`

Sends a message as the specified local CSGClaw participant.

Example request body:

```json
{
  "room_id": "room-1",
  "text": "hello",
  "thread_root_id": "msg-root"
}
```

`thread_root_id`, `topic_id`, and `context.topic_id` are optional thread/topic
identifiers. When one is present, the participant response is sent as a reply inside
that IM thread. When all are omitted, the response is sent as a top-level room/DM
message; the server does not infer a thread from the participant's most recent room
event.

This endpoint also accepts the same multipart attachment format as `POST /api/v1/messages`.

Use a `payload` JSON part for the participant message fields and one or more `files` parts for generated files.

PicoClaw outbound message shape is also accepted:

```json
{
  "chat_id": "room-1",
  "content": "hello",
  "context": {
    "channel": "csgclaw",
    "chat_id": "room-1",
    "topic_id": "msg-root"
  }
}
```

### `GET /api/v1/agents/{id}/llm/models`

### `GET /api/v1/agents/{id}/llm/v1/models`

Forwards model-list requests to the LLM bridge.

Notes:

- Requires Bearer Token
- Response content type and body are determined by the upstream bridge

### `POST /api/v1/agents/{id}/llm/chat/completions`

### `POST /api/v1/agents/{id}/llm/v1/chat/completions`

Forwards chat-completions requests to the LLM bridge.

Notes:

- Requires Bearer Token
- The request body is read and forwarded as-is
- Maximum single request-body read size is `10 MiB`
- Failures may return either plain-text errors or a JSON error payload such as:

```json
{
  "error": {
    "code": "unauthorized",
    "message": "upstream auth failed",
    "provider": "openai"
  }
}
```

### `POST /api/v1/agents/{id}/llm/responses`

### `POST /api/v1/agents/{id}/llm/v1/responses`

### `GET /api/v1/agents/{id}/llm/responses`

### `GET /api/v1/agents/{id}/llm/v1/responses`

Forwards OpenAI-compatible Responses API requests to the LLM bridge. Codex runtime uses this entrypoint for provider traffic. If the selected upstream provider returns an unsupported Responses endpoint status, the bridge falls back to upstream chat completions and wraps the result in a Responses-compatible response for Codex.

The `GET` variants are websocket upgrade endpoints for Responses API sessions.

Example request body:

```json
{
  "model": "ignored-by-server",
  "input": "Review this patch.",
  "stream": true
}
```

Notes:

- Requires Bearer Token
- The request is first forwarded to the selected profile's `base_url + /responses`
- If upstream `/responses` returns `404` or `405`, the bridge retries via `base_url + /chat/completions`
- The `model` field is overwritten with the agent's resolved `model_id`
- Responses forwarding does not inject the chat-only top-level `reasoning_effort`
- Upstream Responses headers, status, and body are copied through, including streaming responses such as `text/event-stream`

## Compatibility Notes

- `CreateRoomRequest.participant_ids` is still accepted and mapped to `member_ids`
- `Message.mentions` remains backward-compatible with the legacy format:
  - New format: `[{ "id": "alice", "name": "Alice" }]`
  - Legacy format: `["u-alice"]`
- The local `csgclaw` channel routes are effectively mirrored entrypoints for `/api/v1/users|rooms|messages`

## Old Endpoints No Longer Exposed

The following paths often seen in older docs are no longer registered in the current router and should not be treated as public APIs:

- `/api/v1/notify/{agent_id}`
- `/api/v1/channels/{channel}/bots`
- `/api/v1/channels/{channel}/bots/{id}`
- `/api/v1/channels/feishu/bots/{id}/events`
- `/api/bots/{id}/events`
- `/api/bots/{id}/messages/send`
- `/api/bots/{id}/llm/*`
- Any other legacy path not registered in `internal/api/router.go`
