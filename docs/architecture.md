# CSGClaw Architecture

## Overview

The following diagram shows the relationships among the main CSGClaw concepts.

```
+----------------------------------------------------------------------------------------+
| Channel                                                                                |
|                                                                                        |
|   +----------------+      +----------------+      +----------------------+             |
|   | CSGClaw IM     |      | Feishu / Lark  |      | Matrix     (planned) |             |
|   +-------|--------+      +-------|--------+      +-----------|----------+             |
+-----------|-----------------------|--------------------------|-------------------------+
            |                       |                          |
            | control               | control                  | control
            v                       v                          v
+----------------------------------------------------------------------------------------+
| Room                                                                                   |
|                                                                                        |
|   +----------------+      +----------------+                 +----------------+         |
|   | Room 1         |      | Room 2         |       ...       | Room N         |         |
|   |                |      |                |                 |                |         |
|   |   Manager      |      |   Manager      |                 |   Manager      |         |
|   |  /   |   \     |      |  /   |   \     |                 |  /   |   \     |         |
|   | W1   W2  WN    |      | W1   W2  WN    |                 | W1   W2  WN    |         |
|   +----------------+      +----------------+                 +----------------+         |
+----------------------------------------------------------------------------------------+
              |
              | dependency
              v
+----------------------------------------------------------------------------------------+
| Participant                                                                            |
|                                                                                        |
|  +--------------------------+  +----------------------------+  +--------------------+    |
|  | Agent Participant        |  | Notification Participant   |  | Human Participant  |    |
|  |                          |  |                            |  |                    |    |
|  |  User <-------> Agent    |  |  User <-------> Pull/Push   |  | User identity      |    |
|  |                 |        |  |        Notification        |  |                    |    |
|  +-----------------|--------+  +----------------------------+  +--------------------+    |
+--------------------|-------------------------------------------------------------------+
                     |
                     | dependency
                     v
+----------------------------------------------------------------------------------------+
| Runtime                                                                                |
|                                                                                        |
|        +------------------+        +------------------+        +------------------+     |
|        | PicoClaw Sandbox |        | OpenClaw Sandbox |        | Codex            |     |
|        +--------|---------+        +--------|---------+        +------------------+     |
+-----------------|--------------------------|-------------------------------------------+
                  | dependency               | dependency
                  v                          v
+----------------------------------------------------------------------------------------+
| Sandbox                                                                                |
|                                                                                        |
|        +------------------+        +------------------+        +------------------+     |
|        | BoxLite          |        | Docker           |        | CSGHub           |     |
|        +------------------+        +------------------+        +------------------+     |
+----------------------------------------------------------------------------------------+
```

<details>
<summary>View the colored version</summary>

![CSGClaw concepts relationship diagram](../assets/architecture.png)

</details>

CSGClaw is a Go-based local multi-agent platform. It runs a single local HTTP server, serves the Web UI, exposes REST/SSE/WebSocket APIs, and manages channels, rooms, participants, agents, runtimes, sandboxes, users, and messages.

The ASCII diagram describes the system as five layers:

- **Channel**: the external or built-in interaction surface, such as `csgclaw` IM, Feishu / Lark, or a planned Matrix integration.
- **Room**: the collaboration container controlled by a channel. Each room can contain humans, agent participants, and notification participants.
- **Participant**: the product-facing channel identity inside a room. Participant types are `human`, `agent`, and `notification`.
- **Agent**: the runtime-managed execution identity optionally bound to an `agent` participant.
- **Runtime**: the executable agent runtime, such as PicoClaw Sandbox, OpenClaw Sandbox, or Codex.
- **Sandbox**: the isolation backend used by a runtime, such as BoxLite, Docker, or CSGHub.

The dependency direction in the diagram is intentional:

```text
channel -> room -> participant -> agent -> runtime -> sandbox
```

Each upper layer orchestrates the layer below it. A channel controls rooms, a room contains participants, an agent participant may bind to an Agent, the Agent delegates execution to a runtime, and the runtime relies on a sandbox provider for isolation.

Within that model, a participant is the stable binding object exposed to users:

```text
participant
 ├─ type: human | agent | notification
 ├─ channel + participant_id ─► stable channel identity
 ├─ channel_user_ref ─────────► user identity in the selected channel
 └─ agent_id ─────────────────► optional runtime Agent
```

This keeps channel messaging in `internal/im` and `internal/channel`, room-level collaboration in the room and message services, participant provisioning in `internal/participant`, runtime execution in `internal/runtime` / `internal/agent`, and sandbox integration behind the runtime and sandbox packages.

In the current codebase, those layers map roughly as follows:

- **Channel layer**: implemented by the built-in `internal/im` services and external adapters under `internal/channel/*`.
- **Room layer**: represented by room, membership, message, and thread flows exposed through the IM and channel APIs.
- **Participant layer**: implemented by `internal/participant`, including human, agent, and notification participants.
- **Runtime layer**: implemented primarily by `internal/runtime/*` and `internal/agent`.
- **Sandbox layer**: implemented by sandbox backends such as `internal/sandbox/boxlitecli`, plus runtime-specific sandbox integration paths.

The local HTTP server and Web UI sit beside these layers as operator and user entrypoints. `internal/server` owns server lifecycle and static UI wiring, while `internal/api` owns route registration and request/response handling over the same underlying domains.

---

## Design Rules

- `cmd/csgclaw` and `cmd/csgclaw-cli` stay thin. They should only start their CLI entrypoints.
- `cli` owns command parsing, HTTP calls, and output formatting.
- `internal/api` owns HTTP request/response handling only.
- `internal/participant` owns participant creation and listing. It coordinates `agent` and channel user creation when needed.
- `internal/agent` owns agent lifecycle and logs through `internal/sandbox`.
- `internal/im` owns the built-in `csgclaw` IM.
- `internal/channel` owns external channel integrations such as Feishu.
- Secrets must not be hardcoded or printed. Logs and startup output must keep tokens redacted.

---

## IM Threads

The built-in IM thread model is documented in [im-threads.md](./im-threads.md).
Threads are root-message-anchored sub-conversations inside a room or DM. They
use Matrix-shaped `m.thread` relation metadata while staying inside the existing
CSGClaw IM API surface.

Thread replies are hidden from the main room timeline by default. Runtime and Codex
bridges scope normal conversations by `room_id` and thread conversations
by `room_id:thread_root_id`, so each thread starts with clean runtime context
plus the hidden root context snapshot.

---

## Package Layout

```text
cmd/csgclaw/            CLI entrypoint
cmd/csgclaw-cli/        lite CLI entrypoint
cli/                    command flows and user-facing output
cli/csgclawcli/         csgclaw-cli app wiring and global flag handling
cli/message/            shared message command implementation for csgclaw and csgclaw-cli
internal/server/        local HTTP server and static UI wiring
internal/api/           HTTP handlers and route registration
internal/participant/   participant lifecycle and optional agent/user binding
internal/agent/         agent runtime and storage
internal/sandbox/       runtime-neutral sandbox interfaces
internal/sandbox/boxlitecli/ BoxLite CLI sandbox implementation
internal/sandbox/csghub/ CSGHub sandbox implementation
internal/im/            built-in csgclaw IM and PicoClaw bridge
internal/channel/       external channel integrations, including Feishu
internal/config/        config defaults, load/save
web/app/                Web UI development source and Vite project
web/static-dist/        generated Web UI assets for Go embed; run make build-web
```

`internal/participant` is the business boundary for participant behavior. It should not be implemented as extra glue inside API handlers.

---

## Participant Model

The participant record is the stable channel identity exposed to users and higher-level workflows.

Typical fields:

```json
{
  "id": "alice",
  "channel": "csgclaw",
  "type": "agent",
  "name": "Alice",
  "channel_user_ref": "u-alice",
  "channel_user_kind": "local_user_id",
  "agent_id": "u-alice"
}
```

Legacy notes:

- Product-facing collaboration identities are participants, not bots.
- A participant is scoped to a channel and has `type=human|agent|notification`.
- `agent` participants may create or bind a runtime Agent.
- In the example above, `alice` is the participant ID; `u-alice` is not a participant ID.
- Channel user identity belongs to participant state, while runtime state belongs to Agent.

---

## HTTP API

All new product APIs should live under `/api/v1`.

```text
# Participant
GET    /api/v1/channels/{channel}/participants       List participants
POST   /api/v1/channels/{channel}/participants       Create a participant
GET    /api/v1/channels/{channel}/participants/{id}  Get a participant
PATCH  /api/v1/channels/{channel}/participants/{id}  Update a participant
DELETE /api/v1/channels/{channel}/participants/{id}  Delete a participant

# Agent
GET    /api/v1/agents                List agents
POST   /api/v1/agents                Create an agent
GET    /api/v1/agents/{id}           Get agent status
DELETE /api/v1/agents/{id}           Stop and delete an agent
GET    /api/v1/agents/{id}/logs      Fetch or stream agent logs

# Built-in csgclaw IM
GET    /api/v1/rooms                 List rooms
POST   /api/v1/rooms                 Create a room
DELETE /api/v1/rooms/{id}            Delete a room
GET    /api/v1/users                 List users
DELETE /api/v1/users/{id}            Kick a user
GET    /api/v1/messages              Fetch message history
POST   /api/v1/messages              Send a message
POST   /api/v1/rooms/{id}/threads    Start a thread
GET    /api/v1/rooms/{id}/threads    List room threads
GET    /api/v1/rooms/{id}/threads/{root_message_id}  Get a thread
GET    /api/v1/rooms/{id}/relations/{event_id}/m.thread  List thread relations

# Feishu channel
GET    /api/v1/channels/feishu/users
POST   /api/v1/channels/feishu/users
GET    /api/v1/channels/feishu/rooms
POST   /api/v1/channels/feishu/rooms
GET    /api/v1/channels/feishu/rooms/{room_id}/members
POST   /api/v1/channels/feishu/rooms/{room_id}/members
POST   /api/v1/channels/feishu/messages
```

`POST /api/v1/channels/{channel}/participants` should be handled as a participant provisioning use case:

```text
API handler
  └─► internal/participant.Create
        ├─► create or bind Agent through internal/agent when type=agent
        ├─► create or bind channel user through internal/im or internal/channel
        └─► persist participant identity
```

The API layer should not directly duplicate participant provisioning logic.

---

## CLI

Both CLIs are thin HTTP clients. They should not call stores, BoxLite, or channel SDKs directly.

`csgclaw` is the full local management CLI for human operators. It owns server lifecycle, agent runtime commands, and the shared participant/room/member/user/message workflows.

`csgclaw-cli` is the lightweight CLI primarily intended for agents and scripts. It exposes only the participant, room, member, and message workflows that agents need for collaboration, and does not manage the local server lifecycle or agent runtime directly.

At a high level:

- `csgclaw` includes local operator workflows such as `serve`, `stop`, and agent management, plus shared collaboration commands.
- `csgclaw-cli` keeps only the collaboration-oriented command groups needed by participants, agents, and scripts.
- Shared collaboration commands select the target channel through flags and call the same local HTTP API surface.

For the current command tree, flags, defaults, and examples, see [cli.md](./cli.md) or [cli.zh.md](./cli.zh.md).

---

## Creation Flow

```text
csgclaw participant create --channel feishu --type agent
  └─► POST /api/v1/channels/feishu/participants
        └─► internal/participant.Create
              ├─► internal/agent creates or reuses runtime Agent
              ├─► internal/channel binds Feishu channel identity
              └─► internal/participant saves:
                    participant_id
                    type
                    channel
                    agent_id
                    channel_user_ref
```

For the built-in channel, the same flow uses `internal/im` to create the user identity.

---

## Persistence

Filesystem storage remains the default persistence layer.

Each domain owns its own records:

- `agent`: runtime metadata and sandbox state references
- `participant`: channel identity and optional agent binding
- `im`: built-in rooms, users, messages, and events
- `channel`: external channel integration state when needed

Do not store channel-specific details directly inside the agent record. The agent should remain the runtime object; channel identity belongs to participant/channel state.

---

## Notes

- Legacy bot compatibility routes are removed from the target API. Runtime clients should use participant-scoped event/message routes and agent-scoped LLM routes.
- Feishu support should live behind `internal/channel`, while participant provisioning decisions stay in `internal/participant`.
- When changing config fields or defaults for participant/channel behavior, update loader, saver, bootstrap initialization flow, tests, and docs together.
