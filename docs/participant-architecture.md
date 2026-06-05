# Participant Identity Architecture and API Plan

## Quick Review Points

- The core change is to separate `Participant`, `Agent`, `ChannelUser`, and the
  product-facing word `Bot`: participants are the collaboration identities used
  by rooms, messages, members, and mentions; agents are runtime execution
  entities; channel users are channel-internal identity/profile records; bot is
  no longer a backend API or storage model.
- When the UI creates an Agent that should appear in the built-in CSGClaw IM, it
  should call `POST /api/v1/channels/csgclaw/participants` with `type=agent` and
  `agent_binding.mode=create`. The server provisions the Agent, ChannelUser, and
  Participant in one operation.
- For a newly created agent-backed participant, the generated Agent ID keeps the
  old relationship: `agent_id = u-{participant_id}`. The participant ID should
  come from an explicit `id` or stable key, not from a later-editable `name`.
- Cross-channel reuse no longer depends on equal IDs. One Agent can have many
  participants, such as `csgclaw:qa -> agent:u-qa` and
  `feishu:test -> agent:u-qa`.
- Mentions belong to the participant identity layer. Feishu human mentions
  resolve through `channel_user_ref` plus `channel_app_ref` /
  `channel_user_kind`; humans do not need their own bot app/config.
- A notification is a `type=notification` participant representing a webhook,
  system event, or pull relay source. It does not bind to an Agent or expose an
  LLM bridge by default.
- New message APIs use `mentions` / `mention_ids` arrays; room membership moves
  from `user_ids` to `participant_ids` or structured `ParticipantRef` values.
- Deleting a participant deletes only the channel identity binding by default,
  not the underlying Agent. Agent cleanup requires explicit semantics such as
  `delete_agent=if_unreferenced`.
- This is a coordinated breaking API change across frontend, backend, runtime
  bridge, CLI, and embedded templates. API compatibility is not the migration
  focus; old bot routes and public `/users` routes do not keep compatibility
  aliases.
- Legacy on-disk data such as `bots.json` should still be migratable. If an old
  runtime image or template contract is outdated, show a recreate warning in the
  UI; the current recreate flow only promises to preserve user-installed skills.
- Matrix alignment is limited to identity, membership, message, mention, and
  thread shapes touched by this work. It does not implement a full Matrix
  homeserver or Client-Server API.

## Migration Priorities

Frontend and backend ship together, so API breaking changes are not the
migration risk. Migration focuses on local on-disk state and old runtime images.

- **`bots.json`**: legacy bot records must still load and migrate into
  participant records. Normal bots become `type=agent` participants while
  preserving the original `agent_id` / `channel_user_ref`; notification bots
  become `type=notification` participants.
- **IM state**: identity references in `im/state.json` and `im/sessions/*.jsonl`
  must migrate from old user/bot IDs to participant IDs, including `users`, room
  `members`, message `sender_id`, `mentions`, and thread context.
- **Feishu config**: app/config entries in `channels/feishu.toml` currently keyed
  by old `bot_id` must migrate to participant/channel-app semantics so Feishu
  sending and mention resolution do not lose configuration.
- **Team state**: `teams/*` fields such as `lead_bot_id`, `member_bot_ids`,
  `bot_id`, `actor_id`, `created_by`, `assigned_to`, `requested_by`, and
  `approver_id` must migrate to participant IDs.
- **Agents state**: `agents/state.json` does not need Agent ID rewrites; new
  participant records keep pointing at existing Agents through `agent_id`.
- **Outdated image warning**: whenever the runtime image/template contract is
  detected as outdated, show a recreate warning in the UI.
- **Recreate preserves skills**: the current recreate flow only promises to
  preserve user-installed skills; preserving workspace/project state is not
  added in this plan.

## Background

CSGClaw currently uses `bot`, `agent`, and channel `user` concepts in ways that
work for the first local multi-agent workflow, but do not scale cleanly to
multi-channel and human-in-the-loop collaboration.

The immediate problem is mention identity. Agents need to mention real people in
the built-in CSGClaw IM, Feishu, and future IM integrations. The current bot
model assumes that message senders and mentions are bot-like identities. In
Feishu, that means a mention is resolved through configured bot app identities,
so an agent cannot cleanly mention a real human open_id.

There is also a naming and ownership problem. The UI exposes many workflows as
agent management, while parts of the client call channel bot APIs. At the same
time, the underlying runtime agent is already reusable across channels. For
example, a Feishu bot can be modeled as a Feishu channel identity plus an
underlying agent that was originally created from the CSGClaw channel. Today
that reuse depends heavily on matching IDs such as `u-manager`. If the CSGClaw
agent is `u-qa` and the Feishu-facing participant is `test`, the relationship
cannot be represented cleanly.

The target design separates these concerns:

- `Participant` is the collaboration identity used by rooms, messages, members,
  and mentions.
- `Agent` is the runtime execution entity that owns model, profile, lifecycle,
  logs, and sandbox state.
- `Bot` is removed from the API and storage model. It may remain only as UI copy
  where the product intentionally says "bot" to a user.

## Goals

- Agents can mention real people in CSGClaw IM, Feishu, and future IMs.
- Rooms and messages can include humans, agent-backed participants, and
  notification participants with one unified participant reference.
- One underlying agent can be reused by many channel participants without
  relying on equal IDs.
- `GET /api/v1/agents` can show all runtime agents and the participants that
  expose them in any supported IM, including Feishu.
- The UI can support both creating a new agent-backed participant and adding an
  existing agent to another channel without forcing users to understand the
  internal model first.
- The frontend, backend, runtime bridge, and embedded templates are updated
  together. Existing bot routes are removed rather than kept as old aliases.
- The new shape stays close to Matrix concepts where this work already touches
  rooms, members, messages, mentions, and threads.

## Non-Goals

- Do not merge the same real person across multiple channels in the first
  implementation. A later `Identity` layer can group channel-specific human
  participants if product requirements need cross-channel audit or permissions.
- Do not require every participant to have an agent. Humans and notification
  participants do not need runtime execution.
- Do not implement a full Matrix homeserver or complete Client-Server API in
  this change. Only the identity, membership, message, mention, and thread
  shapes touched by this work should be Matrix-friendly.

## Target Model

### Agent

An agent is global to CSGClaw and independent of any channel.

```text
Agent
  id
  name
  role
  runtime_id
  runtime_kind
  image
  runtime_options
  status
  agent_profile
  created_at
```

Rules:

- Agent IDs are global.
- When an agent-backed participant is created and the request does not specify
  an Agent ID, the server generates it as `u-{participant_id}`. This preserves
  the old worker/bot ID habit; for example participant `qa` maps to agent
  `u-qa`.
- The bootstrap manager is the reserved exception: its default CSGClaw
  participant ID is `manager`, while its Agent ID remains `u-manager`.
- If the caller specifies an Agent ID explicitly, it must still be globally
  unique and does not need to match the participant ID. Cross-channel reuse
  usually passes an existing `agent_id`.
- Agent lifecycle operations remain under `/api/v1/agents`.
- Agent profile, model, runtime, logs, start, stop, restart, and recreation stay
  owned by the agent service.

### Participant

A participant is the identity visible inside one channel.

```text
Participant
  id                  # stable within its channel
  channel             # csgclaw | feishu | matrix | ...
  type                # human | agent | notification
  name
  avatar
  channel_user_ref    # csgclaw user id, feishu open_id, matrix user_id, ...
  channel_user_kind   # local_user_id | open_id | matrix_user_id | ...
  channel_app_ref     # optional, for bot app/config identity such as Feishu app_id
  agent_id            # optional FK -> Agent.id, only meaningful for type=agent
  lifecycle_status    # provisioning | active | disabled | failed
  presence            # optional channel presence or room member view
  mentionable
  metadata
  created_at
  updated_at
```

Rules:

- The canonical participant key is `(channel, id)`.
- `channel_user_ref` is required for participants that can send, receive, or be
  mentioned in the channel.
- Active participants should be unique by that channel's channel-user identity.
  For simple channels this is `(channel, channel_user_ref)`. For app-scoped
  identities such as Feishu, `channel_app_ref` and `channel_user_kind` must also
  be part of the unique key.
- `lifecycle_status` describes whether the participant record itself is usable;
  `presence` describes the channel or room online/offline view; runtime running
  state comes from the bound Agent and should not be persisted on the participant.
- `mentionable=false` means the participant may remain visible in lists or
  history, but should not be available as a new mention target.
- `type=human` must not require `agent_id`.
- `type=notification` must not require `agent_id`.
- `type=agent` may bind an existing agent, create a new agent, or be registered
  without a runtime binding and bound later.
- A single agent can have many participants across channels:

```text
csgclaw:qa     -> agent:u-qa
feishu:test    -> agent:u-qa
matrix:qa-bot  -> agent:u-qa
```

### Participant ID Generation

Participant IDs are channel identities that users and the CLI see often, so they
should prefer readable and stable values instead of defaulting to bare UUIDs.
UUIDs can remain the internal random source or fallback, but exposing them
directly makes room membership, mentions, and CLI operations harder to read.

Participant IDs must not be generated from `name`. `name` is a display name and
may become editable later; once an ID is referenced by room membership, messages,
mentions, agent binding, and CLI commands, it must remain stable. The common
industry pattern is "stable slug plus short random collision suffix", such as
Kubernetes object names or many SaaS workspace slugs. Opaque type-prefixed IDs
such as `usr_...` or `agt_...` fit internal-only objects better than
user-operated participant IDs.

Recommended algorithm:

1. If the request explicitly passes `id`, normalize it and verify uniqueness.
2. If no `id` is provided, derive a slug only from stable sources such as a
   separate `slug` / `handle` field in the create request, an embedded template
   key, a role key, an immutable external channel handle, or a legacy bot/user ID
   during migration. Do not use editable display `name`.
3. Slug rules: lowercase; trim surrounding whitespace; replace consecutive
   non-`[a-z0-9]` characters with `-`; collapse repeated `-`; trim leading and
   trailing `-`; keep length between 3 and 48 characters.
4. If the slug is empty, use a readable type prefix plus a short random suffix,
   such as `agent-8f3k2m`, `human-8f3k2m`, or `notification-8f3k2m`.
5. If the slug already exists, append a short random suffix, such as
   `qa-8f3k2m`. The suffix can be a truncated base32/base36 value derived from
   UUID, ULID, or nanoid.
6. The server returns the final participant ID. Repeated requests with the same
   `request_id` or `client_transaction_id` must return the same ID.

For agent-backed participants, the default Agent ID rule remains:

```text
agent_id = "u-" + participant_id
```

For example, creating a CSGClaw IM Agent as participant `qa` generates agent
`u-qa` by default, while the built-in CSGClaw channel user ref can also remain
`u-qa`. This preserves old runtime, workspace, and mention habits.

### Channel User / Channel Identity

The current public `User` model should not remain as a top-level product API
after participants are introduced. It should be replaced by participant APIs.

A user-like record is still needed internally, but its meaning changes: it is a
channel-scoped identity/profile record owned by the channel adapter, not the
primary collaboration identity.

```text
ChannelUser
  channel             # csgclaw | feishu | matrix | ...
  ref                 # csgclaw local user id, Feishu open_id/union_id, Matrix user_id
  kind                # local_user_id | open_id | matrix_user_id | ...
  app_ref             # optional, used for Feishu app_id/tenant-scoped identity
  display_name
  handle
  avatar_url
  presence
  raw_profile
  updated_at
```

Rules:

- `Participant.channel_user_ref` points to a channel user or equivalent
  channel-native identity.
- For the built-in CSGClaw channel, this internal record replaces the current
  `User` storage shape for profile, avatar, handle, and presence.
- For Feishu, this is an adapter-owned record. With `kind=open_id`, `ref` is an
  app-scoped open_id and must be interpreted together with `app_ref`. If later
  implementations use `union_id` or `user_id`, that choice must be explicit in
  `kind`.
- For Matrix, this maps naturally to Matrix user IDs and member profile fields
  such as `displayname`, `avatar_url`, and membership state.
- Public clients should create, list, update, and delete people through
  participants. They should not call a separate `/users` API.

### Feishu Identity Scope

Feishu user identity cannot be modeled as a bare `open_id` string. The same real
person can have different IDs across apps, tenants, or identity types, and the
same string must not be reused across scopes implicitly.

Rules:

- `channel_app_ref` identifies the Feishu app/config for this participant, for
  example `cli_xxx`.
- With `channel_user_kind=open_id`, `channel_user_ref` is the open_id in that app
  scope. The unique key should include at least
  `(channel, channel_app_ref, channel_user_kind, channel_user_ref)`.
- `type=agent` participants need `channel_app_ref` so the sender credentials are
  unambiguous.
- `type=human` participants use `channel_user_ref` as the mention target and do
  not require that human to have a bot app/config.
- If an adapter receives only `user_id`, `union_id`, or another identity from a
  Feishu event, it should store that raw identity with an explicit
  `channel_user_kind` first, then resolve it explicitly when sending or
  mentioning. It must not treat the value as a bot ID implicitly.

### Notification Participant

A notification is also a channel participant, but by default it is not a runtime
agent.

```text
Participant(type=notification)
  channel_user_ref       # notification sender identity or local webhook identity
  channel_app_ref        # optional, app/config needed by an external channel
  metadata.notification  # webhook, remote_pull, subscription, delivery config
```

Rules:

- Notification participants can send room messages so system notifications,
  third-party webhooks, or pull relays have a visible source.
- Notification participants do not have `agent_id` by default and do not expose an
  LLM bridge.
- Notification webhook/pull config belongs on participant metadata or a dedicated
  notification profile, not on a bot-shaped API.
- The old `POST /api/v1/channels/csgclaw/bots/{id}/notifications` should become a
  participant-scoped notification endpoint:

```text
POST /api/v1/channels/{channel}/participants/{id}/notifications
```

### Room, Message, and Mention

Rooms and messages should reference participants, not agents.

```text
Room
  channel
  members: ParticipantRef[]

Message
  event_id
  room_id
  sender: ParticipantRef
  mentions: ParticipantRef[]
  content
  relates_to

ParticipantRef
  channel
  id
```

New APIs should avoid bot-shaped names. Fields such as `sender_id`,
`member_ids`, and `user_ids` are acceptable only when their meaning is explicitly
participant or Matrix user identity, not legacy bot identity. New message APIs
should prefer `mentions: ParticipantRef[]` or `mention_ids: []`, not a single
`mention_id`. Where ambiguity exists, prefer `participant_id`,
`participant_ids`, or a structured `ParticipantRef`.

### Future Chat History Sync Compatibility

Chat history sync is not part of this implementation plan. It is a compatibility
constraint for the participant model.

If a user chats in Feishu and CSGClaw later syncs the room, the imported message
must be able to pass through the same participant resolution path as locally
created messages. The participant model therefore must not assume that every
message was authored locally or that every sender is an agent-backed
participant.

Future sync work should be able to add a `MessageEvent` or adjacent sync record
with fields such as:

- `external_room_ref`, such as Feishu `chat_id` or Matrix room ID;
- `external_event_id`, such as Feishu `message_id` or Matrix event ID;
- `origin_server_ts` from the source IM;
- `received_at` as the local ingestion time;
- `sync_batch` or cursor metadata for resumable sync;
- `raw_event` as a redacted or bounded source payload for debugging.

This plan should not implement sync storage, sync APIs, or backfill jobs now.
It only keeps sender, mention, room, and event identity compatible with that
future work.

### Optional Future Identity Layer

If CSGClaw later needs to know that the same person appears as a CSGClaw local
user, Feishu open_id, and Matrix user_id, add an identity grouping layer:

```text
Identity 1 -> N Participant(type=human)
```

This is not required to solve agent-to-human mentions or cross-channel agent
reuse.

## Matrix Alignment

The planned Matrix direction should influence the new participant model where it
overlaps with this work. The goal is not to implement the full Matrix
Client-Server API now, but to avoid creating a second incompatible IM model.

Use the following alignment points:

- Matrix user IDs map to `Participant.channel_user_ref` with
  `channel_user_kind=matrix_user_id`, for example `@qa:example.org`.
- Matrix room IDs map to channel room references, for example
  `!roomid:example.org`; room aliases can be stored as channel metadata.
- Room membership should be representable as `m.room.member` state:
  `membership`, `displayname`, and `avatar_url` belong on the room membership or
  participant view, not on the runtime agent.
- Text messages should be representable as `m.room.message` events with
  `msgtype=m.text`, `body`, and optional `format` / `formatted_body`.
- Mentions should be representable as Matrix `m.mentions`, especially
  `m.mentions.user_ids` for user mentions and `m.mentions.room` for room
  mentions.
- Existing thread metadata should continue to use Matrix-shaped
  `m.relates_to.rel_type=m.thread` and root event IDs.
- Future CSGClaw sync APIs should follow the same high-level shape as Matrix
  `/sync`: clients receive joined room timelines and a resumable batch token
  similar to `next_batch` / `since`.

This makes CSGClaw's own IM easier to move toward Matrix later while still
letting Feishu and other adapters keep their native transport details.

## API Plan

### Participant APIs

Introduce participant APIs under the channel namespace. These replace the old
channel bot CRUD routes.

```text
GET    /api/v1/channels/{channel}/participants
POST   /api/v1/channels/{channel}/participants
GET    /api/v1/channels/{channel}/participants/{id}
PATCH  /api/v1/channels/{channel}/participants/{id}
DELETE /api/v1/channels/{channel}/participants/{id}
```

The following routes should be deleted:

```text
GET    /api/v1/channels/{channel}/bots
POST   /api/v1/channels/{channel}/bots
GET    /api/v1/channels/{channel}/bots/{id}
PATCH  /api/v1/channels/{channel}/bots/{id}
DELETE /api/v1/channels/{channel}/bots/{id}
GET    /api/v1/channels/feishu/bots/{id}/events
POST   /api/v1/channels/csgclaw/bots/{id}/notifications
GET    /api/bots/{id}/events
POST   /api/bots/{id}/messages/send
GET    /api/bots/{id}/llm/models
GET    /api/bots/{id}/llm/v1/models
POST   /api/bots/{id}/llm/chat/completions
POST   /api/bots/{id}/llm/v1/chat/completions
POST   /api/bots/{id}/llm/responses
POST   /api/bots/{id}/llm/v1/responses
```

The current public user routes should also be removed from the product API
surface:

```text
GET    /api/v1/users
POST   /api/v1/users
DELETE /api/v1/users/{id}
GET    /api/v1/channels/csgclaw/users
POST   /api/v1/channels/csgclaw/users
DELETE /api/v1/channels/csgclaw/users/{id}
```

Use participants instead:

```text
GET  /api/v1/channels/{channel}/participants?type=human
POST /api/v1/channels/{channel}/participants
```

List query parameters:

- `type=human|agent|notification`
- `agent_id=<agent_id>`
- `include_agent=true`
- `include_channel_user=true`

Create an agent-backed participant by creating a new agent:

```json
{
  "id": "qa",
  "type": "agent",
  "name": "qa",
  "channel_user": {
    "ref": "u-qa",
    "kind": "local_user_id"
  },
  "agent_binding": {
    "mode": "create",
    "agent": {
      "id": "u-qa",
      "name": "qa",
      "role": "worker",
      "runtime_kind": "codex",
      "agent_profile": {
        "provider": "api",
        "model_id": "gpt-5.4"
      }
    }
  }
}
```

This endpoint is the replacement for the old UI "create bot" behavior that
created both an agent and a channel user. It must be implemented as one
provisioning operation owned by the server, not as UI-side chaining of
`POST /api/v1/agents` followed by participant creation.

For the built-in CSGClaw channel, `agent_binding.mode=create` must create:

```text
1. Agent
2. Channel user / Matrix-shaped member identity
3. Participant(type=agent, agent_id=<created agent>)
```

The operation should commit only after all three resources are valid. If user or
participant creation fails after the agent is created, the server must either
roll back the created agent or mark the operation as failed and retry-safe with
an idempotency key.

For external channels, true distributed transactions are not available. The API
still stays single-call from the UI, but the server must use idempotency and
compensation:

- accept an optional `request_id` or `client_transaction_id`;
- make repeated requests with the same key return the same final resource;
- record partially completed steps;
- retry or compensate failed channel-user provisioning before exposing the
  participant as active.

Create a channel participant that reuses an existing agent:

```json
{
  "id": "test",
  "type": "agent",
  "name": "QA",
  "channel_user": {
    "ref": "ou_xxx",
    "kind": "open_id"
  },
  "channel_app_ref": "cli_xxx",
  "agent_binding": {
    "mode": "reuse",
    "agent_id": "u-qa"
  }
}
```

Create a human participant:

```json
{
  "id": "alice",
  "type": "human",
  "name": "Alice",
  "channel_user": {
    "ref": "ou_alice",
    "kind": "open_id"
  }
}
```

Supported `agent_binding.mode` values:

- `create`: create a new agent and bind it to this participant.
- `reuse`: bind this participant to an existing agent.
- `none`: create the participant without runtime binding. This is valid for
  humans, notifications, and draft agent participants.

Validation:

- `type=human` rejects `agent_binding.mode=create`.
- `type=notification` rejects `agent_binding.mode=create` unless a future
  notification runtime explicitly needs it.
- `type=agent` with `mode=reuse` requires `agent_id`.
- `type=agent` with `mode=create` requires enough agent fields to create a
  valid worker or manager.
- Participant ID and agent ID are not required to match.

Deletion rules:

- `DELETE /api/v1/channels/{channel}/participants/{id}` deletes the participant
  and its channel binding by default. It does not delete the underlying Agent.
- If a `type=agent` participant is bound to an Agent, deleting the participant
  only removes that channel identity from the Agent. Other participants for the
  same Agent are unaffected.
- If the caller wants to clean up an Agent that is no longer referenced, it must
  use an explicit parameter such as `delete_agent=if_unreferenced`. If any other
  participant still references that Agent, the server must reject Agent deletion.
- Channel users / channel identities may be cleaned up only when CSGClaw owns
  them and no other active participant references them. External channels should
  usually delete only the local mapping or mark it inactive, not delete the real
  remote user.
- Deleting a notification participant should also remove local notification
  profile, webhook token, and remote_pull subscription metadata.

### Agent APIs

Keep `/api/v1/agents` as the runtime-agent API for lower-level runtime
management: edit model/profile, start, stop, restart, recreate, delete, view
logs, and support internal provisioning flows.

CSGClaw's own product UI should not use `POST /api/v1/agents` as the primary
"Create Agent" action when the result is expected to appear in the CSGClaw IM.
It should use the same participant provisioning API as third-party channels,
with `channel=csgclaw`.

Extend list and detail responses with participant bindings.

```text
GET /api/v1/agents?include_participants=true
GET /api/v1/agents/{id}?include_participants=true
```

Example response excerpt:

```json
{
  "id": "u-qa",
  "name": "qa",
  "role": "worker",
  "runtime_kind": "codex",
  "status": "running",
  "participants": [
    {
      "id": "qa",
      "channel": "csgclaw",
      "type": "agent",
      "channel_user_ref": "u-qa"
    },
    {
      "id": "test",
      "channel": "feishu",
      "type": "agent",
      "channel_user_ref": "ou_xxx"
    }
  ]
}
```

This satisfies the requirement that the agent list includes agents represented
in any supported IM, including Feishu.

### Runtime Bridge API Replacement

Deleting `/api/bots/*` requires replacing each old bridge surface with an
explicit participant or agent scoped route.

Participant event streams are channel identity concerns:

```text
GET /api/v1/channels/{channel}/participants/{id}/events
```

This replaces both:

```text
GET /api/v1/channels/feishu/bots/{id}/events
GET /api/bots/{id}/events
```

Participant-authored message sending is also a channel identity concern:

```text
POST /api/v1/channels/{channel}/participants/{id}/messages
```

The sender comes from the path participant. The body contains `room_id`,
`content`, optional `mentions`, and optional thread relation fields. This
replaces:

```text
POST /api/bots/{id}/messages/send
```

Notification delivery is also a channel identity concern:

```text
POST /api/v1/channels/{channel}/participants/{id}/notifications
```

This replaces:

```text
POST /api/v1/channels/csgclaw/bots/{id}/notifications
```

LLM bridge calls are runtime agent concerns, not channel identity concerns:

```text
GET  /api/v1/agents/{agent_id}/llm/models
POST /api/v1/agents/{agent_id}/llm/chat/completions
POST /api/v1/agents/{agent_id}/llm/responses
```

These replace:

```text
GET  /api/bots/{id}/llm/models
GET  /api/bots/{id}/llm/v1/models
POST /api/bots/{id}/llm/chat/completions
POST /api/bots/{id}/llm/v1/chat/completions
POST /api/bots/{id}/llm/responses
POST /api/bots/{id}/llm/v1/responses
```

If a runtime only knows its channel participant ID, it must resolve the
participant first and use `agent_id` for LLM calls. This keeps message identity
and model/runtime identity separate.

### UI API Replacement

The current UI issue is that an "Agent" workflow calls the deleted channel bot
API. The replacement is not UI-side chaining of multiple APIs. CSGClaw's own UI
must use the same model as Feishu and future third-party IMs: create a
participant in the target channel and bind or create an underlying agent.

When CSGClaw's UI action means "create an Agent that can chat in CSGClaw IM",
call:

```text
POST /api/v1/channels/csgclaw/participants
```

with `type=agent` and `agent_binding.mode=create` to create both the runtime
agent and the CSGClaw participant in one operation. This is the direct
replacement for the previous bot API that created both agent and user.

When the UI adds an existing agent to another channel, use the same endpoint for
that channel with `agent_binding.mode=reuse`.

The UI must not call `POST /api/v1/agents` and then separately create a user or
participant for this flow. That split can leave a created agent without a
channel identity, or a channel identity without a valid runtime binding.

Recommended UI-to-API mapping:

```text
CSGClaw Agent UI
  Create Agent for CSGClaw IM -> POST /api/v1/channels/csgclaw/participants
                                  type=agent, agent_binding.mode=create
  Edit runtime/model/profile  -> PATCH/PUT /api/v1/agents/{id}...
  Start/stop/logs             -> /api/v1/agents/{id}/...

Channel or room page
  Create new agent identity   -> POST /api/v1/channels/{channel}/participants
                                  type=agent, agent_binding.mode=create
  Add existing agent identity -> POST /api/v1/channels/{channel}/participants
                                  type=agent, agent_binding.mode=reuse
  Add real person             -> POST /api/v1/channels/{channel}/participants
                                  type=human, agent_binding.mode=none
```

### Message and Mention APIs

For participant-scoped send APIs, the sender comes from the path participant and
the body does not include `sender_id`. Mentions are arrays so one message can
mention multiple participants.

```json
{
  "room_id": "oc_xxx",
  "mentions": [
    {
      "id": "alice"
    }
  ],
  "content": "please take a look"
}
```

The channel adapter resolves:

```text
path id -> Participant(channel=feishu, id=test)
        -> channel_user_ref/channel_app_ref for sender credentials

mentions[].id -> Participant(channel=feishu, id=alice)
              -> channel_user_ref=open_id
```

For CSGClaw IM, the adapter renders a local mention using the local user ID. For
Feishu, the adapter renders `<at user_id="open_id">name</at>`. For future IMs,
the adapter uses that channel's mention syntax.

If a channel-level message endpoint remains, `sender_id` must explicitly mean a
participant ID in that channel. It must not mean bot ID. A single `mention_id`
does not belong in the new API and should only exist as a temporary
implementation detail while callers are updated in the same change.

Room member APIs should also move from user-shaped fields to participant-shaped
fields. For example, `user_ids` should become `participant_ids` or
`participants: ParticipantRef[]`. Matrix payload adapters may still emit or
accept Matrix-native `user_id` values at the protocol boundary, but the
CSGClaw domain model should resolve them to participants before room membership
or mention logic runs.

## UI Plan

The UI should not force users to choose between model terms such as
participant, agent, and channel user as the first decision.

Use intent-based entry points:

```text
Add Bot to Current Channel

  Create a new bot
    - create Participant(type=agent)
    - create and bind a new Agent
    - configure runtime, model, and template

  Add an existing bot
    - list agents that are not yet represented in this channel
    - create Participant(type=agent)
    - bind it to the selected Agent
    - confirm channel-specific identity settings
```

For human participants, use channel-native wording:

```text
Add Person
  - choose or enter channel user identity
  - create Participant(type=human)
  - make the person mentionable and optionally add them to a room
```

The UI can still use product-friendly labels such as Bot and Person. The backend
should keep the participant and agent split explicit.

## CLI Changes

The canonical CLI resource name should follow the backend model and use
`participant` for collaboration identities, with `pt` as a shorter subcommand
alias. Use `participant` in docs, scripts, and long-lived references; use `pt`
for interactive daily commands. `bot` can remain as a lightweight user-facing
alias for `type=agent` flows, but JSON output, API payloads, and errors should
use participant semantics instead of exposing a Bot storage model.

Recommended command shape:

```text
csgclaw participant list --channel csgclaw --type agent
csgclaw participant create --channel csgclaw --type agent --id qa --name QA --bind create
csgclaw participant create --channel feishu --type agent --id test --bind reuse --agent-id u-qa --channel-user-ref ou_xxx --channel-user-kind open_id --channel-app-ref cli_xxx
csgclaw participant create --channel feishu --type human --id alice --name Alice --channel-user-ref ou_alice --channel-user-kind open_id --channel-app-ref cli_xxx
csgclaw participant delete --channel feishu test
csgclaw participant delete --channel feishu test --delete-agent if-unreferenced
csgclaw pt list --channel csgclaw --type agent
csgclaw pt create --channel csgclaw --type agent --id qa --name QA --bind create
```

CLI field renames should match the API:

- `pt` is an exact short alias for `participant`; every subcommand, flag, output,
  and error must behave the same.
- `bot list/create/delete` is no longer canonical. If kept, it should only be a
  product alias for `participant --type agent` / `pt --type agent`.
- `agent create` only creates runtime-only Agents. It should not be the primary
  entry point for creating a chat-capable CSGClaw IM Agent.
- `user list/create/delete` moves to `participant list/create/delete --type human`.
- Room member commands should rename `--user-id`, `--user-ids`, and
  `--member-ids` to `--participant-id`, `--participant-ids`, or structured
  participant refs.
- Message commands should replace `--sender-id` with a path participant or an
  explicit `--participant-id`; `--mention-id` should become repeatable or be
  renamed to `--mention-participant-id` and sent as `mentions` / `mention_ids`
  arrays.
- Feishu config commands should replace `--bot-id` with either
  `--participant-id` or `--channel-app-ref`, depending on whether the command
  configures a participant binding or manages the Feishu app/config.
- Team/task commands should replace `--lead-bot-id`, `--member-bot-ids`,
  `--bot-id`, and `--actor-id` with `--lead-participant-id`,
  `--member-participant-ids`, `--participant-id`, and
  `--actor-participant-id`. Use `--agent-id` only for runtime-specific
  operations.
- Runtime-embedded commands such as `csgclaw-cli` must be updated too. Embedded
  skills and templates must not keep depending on old `bot_id`, `sender_id`,
  `mention_id`, or `user_ids` semantics.

## One-Step Implementation Scope

- Add participant request/response types.
- Add participant storage with canonical key `(channel, id)`.
- Add a Participant ID generator: derive readable slugs from explicit `id` or
  stable keys, add short random suffixes on collision, never derive IDs from
  editable `name`, and keep the default Agent ID for newly created agent-backed
  participants as `u-{participant_id}`, except the bootstrap manager
  participant `manager` whose Agent ID is `u-manager`.
- Replace the public `User` API with participant APIs. Keep only an internal
  channel identity/profile store where the CSGClaw and external channel
  adapters need one.
- Add participant service methods for list, get, create, patch, delete, and
  agent binding.
- Register `/api/v1/channels/{channel}/participants`.
- Register participant event and message routes:
  `/api/v1/channels/{channel}/participants/{id}/events` and
  `/api/v1/channels/{channel}/participants/{id}/messages`.
- Register the participant notification route:
  `/api/v1/channels/{channel}/participants/{id}/notifications`.
- Register agent LLM routes under `/api/v1/agents/{agent_id}/llm/*`.
- Implement create modes: `create`, `reuse`, and `none`.
- Add response expansion for `include_agent` and `include_channel_user`.
- Add tests for creating a Feishu participant `test` bound to agent `u-qa`.
- Resolve sender and mention IDs through participant service.
- Update CSGClaw IM mention rendering to accept human and agent participants.
- Update Feishu send path so mentions resolve to participant `channel_user_ref`
  instead of requiring a configured bot app for every mention.
- Add tests for agent-to-human mentions in CSGClaw IM and Feishu.
- Add tests proving a Feishu human participant can be mentioned with
  `channel_app_ref + open_id` without having its own bot app/config.
- Change message send requests from a single `mention_id` to `mentions` /
  `mention_ids` arrays.
- Rename room membership request fields from `user_ids` to `participant_ids` or
  structured participant refs.
- Add `participants` expansion to `GET /api/v1/agents`.
- Include participants from Feishu and future channel stores.
- Add tests proving that agent `u-qa` appears with both `csgclaw:qa` and
  `feishu:test` participant bindings.
- Add tests proving that deleting the `feishu:test` participant does not delete
  agent `u-qa` while `csgclaw:qa` still references it.
- Replace the current create-agent/create-bot ambiguity with intent-based
  channel actions.
- Make CSGClaw UI create chat-capable agents through
  `POST /api/v1/channels/csgclaw/participants`, not direct agent creation plus
  separate user provisioning.
- Add "Create a new bot" and "Add an existing bot" flows.
- Add "Add Person" flow for human participants.
- Replace the current notification bot webhook/pull routes with a
  participant-scoped notification endpoint.
- Keep existing Agent pages focused on runtime configuration and lifecycle.
- Update CLI and `csgclaw-cli`: canonical commands use participant, register the
  `pt` short alias, and move old bot/user/member/message parameters to
  participant semantics.
- Migrate identity references in legacy `bots.json`, IM state, Feishu config,
  and Team state. Warn in the UI when the runtime image/template contract is
  outdated; the current recreate flow only preserves user skills.
- Do not implement chat history sync, sync storage, or sync APIs in this change;
  only keep the identity model compatible with future sync.
- Delete channel bot CRUD, Feishu bot event, `/api/bots/*`, and public `/users`
  routes in the same change set.
- Replace runtime bridge callers with participant/agent-scoped routes before
  removing the old handlers.

## Why This Solves the Problem Best

This model matches the real domain boundaries. People and bots are channel
identities. Agents are reusable runtime capabilities. Mentions belong to the
channel identity layer, not the runtime layer.

It also removes the ID-equality assumption. A Feishu participant can be named
`test` and bind to underlying agent `u-qa` explicitly. The relationship is a
stored foreign key rather than a naming convention.

The result is a single target API rather than two competing surfaces. UI code no
longer calls bot APIs when the user is creating an Agent, and channel workflows
create participants explicitly. The UI can stay simple by presenting user intent
instead of internal model terminology.
