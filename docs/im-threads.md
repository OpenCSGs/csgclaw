# CSGClaw IM Threads

This document describes the CSGClaw local IM thread model. It is meant for
maintainers who need to understand how thread storage, APIs, participant bridges,
agent context, and UI behavior fit together.

## Summary

CSGClaw uses an incremental Matrix-shaped thread model inside the existing IM
APIs. It does not implement the full Matrix Client-Server protocol today. The
goal is to adopt the useful shape of Matrix relationships while preserving the
current CSGClaw room, user, participant, auth, and local state model.

A thread is a sub-conversation in a room or DM. It starts from one existing
top-level message, called the root message. The canonical thread ID is the root
message ID.

Thread replies use Matrix-style relation metadata:

```json
{
  "relates_to": {
    "rel_type": "m.thread",
    "event_id": "msg-root"
  }
}
```

## Why Not Full Matrix Yet

The local IM stack does not currently have Matrix homeserver semantics such as
Matrix auth, `/sync`, event IDs, room state, power levels, federation, or state
resolution. Replacing the IM API with raw Matrix now would create a split
between existing CSGClaw concepts and partial Matrix behavior.

Instead, CSGClaw keeps the existing API namespaces and data model, then adds
Matrix-shaped relation fields and relation-query endpoints where they map
cleanly.

## Core Model

- A thread belongs to exactly one room or DM.
- A thread root must be an existing top-level message in that same room.
- Nested thread roots are rejected. A thread reply cannot start another thread.
- The root message ID is the stable thread ID.
- Thread replies are ordinary messages with `relates_to.rel_type = "m.thread"`
  and `relates_to.event_id = <root_message_id>`.
- Root messages expose a `thread` summary when a thread exists.
- Thread replies do not expose their own thread summary.

## Persistence

Messages are still stored as JSONL. Thread fields are optional, so older message
records remain readable.

Each room may also persist `threads`, a list of `ThreadState` records:

- `root_message_id`: canonical root/thread ID.
- `created_at`: when the thread was explicitly started.
- `context`: hidden context snapshot captured at thread start.
- `summary`: deterministic context summary for display and agent prompts.

`ThreadState` is deliberately room-local. Moving to a full Matrix adapter later
should translate this state into Matrix relations rather than changing the local
storage contract first.

## Hidden Context Snapshot

Starting a thread captures hidden context around the root message:

- up to 5 top-level messages before the root,
- the root message,
- up to 2 top-level messages after the root,
- subject to the payload-size cap.

This context is not inserted into the visible thread. It exists so LLM-backed
agents can start a clean new conversation for the thread while still
understanding why the thread was opened.

The v1 summary is deterministic and does not call an LLM:

- `root_excerpt`
- `message_count`
- `before_count`
- `after_count`

Preview text strips leading markdown code fences with language labels such as
`text`, so thread lists and panel titles show the actual content instead of
formatting markers.

## Timeline Semantics

Thread replies stay out of the main room/DM timeline by default. This mirrors
Slack-style thread behavior and keeps rooms readable.

Default timeline surfaces return top-level messages only:

- `GET /api/v1/messages?room_id=...`
- `GET /api/v1/rooms`
- `GET /api/v1/bootstrap`
- csgclaw channel mirror routes

Use `include_thread_replies=true` on the message-list endpoint when a caller
needs the full message history including thread replies.

Sending a thread reply still emits `message.created`, and it also updates the
root thread summary through `thread.updated`.

## API Surface

Thread APIs live in the CSGClaw namespace, not the raw `/_matrix` namespace:

```text
POST /api/v1/rooms/{room_id}/threads
GET  /api/v1/rooms/{room_id}/threads?include=all|participated&limit=&from=
GET  /api/v1/rooms/{room_id}/threads/{root_message_id}
GET  /api/v1/rooms/{room_id}/relations/{event_id}/m.thread
POST /api/v1/messages
GET  /api/v1/messages?room_id=...&include_thread_replies=true
```

The csgclaw channel mirror exposes equivalent thread routes under
`/api/v1/channels/csgclaw/rooms/{room_id}/...`.

See [api.md](./api.md) for request and response shapes.

## Events

The local IM event stream may publish:

- `thread.created`: a new `ThreadState` was created for a root message.
- `thread.updated`: replies or summary data changed for a thread.
- `message.created`: still emitted for normal messages and thread replies.

Thread-aware clients should apply `thread.created` and `thread.updated` to the
root message summary and thread list, while applying `message.created` to the
main timeline only when the message is not a thread reply.

## Participant Bridge and PicoClaw

The participant API is the message bridge used by PicoClaw-style integrations
and the Codex bridge:

```text
GET  /api/v1/channels/csgclaw/participants/{id}/events
POST /api/v1/channels/csgclaw/participants/{id}/messages
```

Thread-aware participant events may include:

- `thread_root_id`: root message ID when the event is inside a thread.
- `thread_context`: hidden context snapshot and summary for the thread root.
- `context.topic_id`: PicoClaw-native topic/session ID. For CSGClaw IM
  threads this is the same value as `thread_root_id`.

`thread_context` is prompt context, not visible thread history.

Participant sends may include either CSGClaw fields (`room_id`, `text`,
`thread_root_id`) or PicoClaw outbound fields (`chat_id`, `content`,
`context.topic_id`). When a thread root/topic is present, the message is sent as
a reply in that thread. Participant sends that omit `thread_root_id`, `topic_id`, and
`context.topic_id` are treated as top-level room/DM messages; CSGClaw does not
infer a thread from the participant's most recent room event.

This maps to PicoClaw/topic isolation requirements: a runtime should treat
`room_id` as the normal conversation key and `room_id:thread_root_id` as the
thread conversation key. Generated PicoClaw configs set session dimensions to
`["chat", "topic"]` so `context.topic_id` creates a separate PicoClaw session
for every CSGClaw IM thread.

## Codex Bridge Session Isolation

The Codex bridge derives runtime conversation identity from the message scope:

- top-level room/DM message: `room_id`
- thread message: `room_id:thread_root_id`

This prevents thread work from bleeding into the room-level Codex session and
prevents one thread from sharing prompt/tool context with another thread.

Tool-call result messages are attached to the response thread so the room
timeline does not become noisy while still preserving tool traceability.

## User Deletion and Thread Cleanup

Deleting a user removes that user's messages from room history. Thread state is
then rebuilt from the surviving messages:

- thread roots sent by the deleted user are pruned,
- hidden context snapshots are regenerated without deleted-user messages,
- replies from the deleted user no longer appear in thread summaries,
- existing thread creation timestamps are preserved where possible.

## UI Behavior

The Web UI follows Slack-like thread behavior:

- message rows expose a hover-only "Reply in thread" action,
- thread replies open in a right-side thread panel,
- the thread composer replies only in the thread,
- thread panels auto-scroll to the latest reply,
- thread lists are grouped by room/DM,
- thread previews use cleaned compact text,
- the room timeline does not gain horizontal scroll from thread panels.

Threads are currently shown as a dedicated workspace tab next to Messages
because the view behaves as a cross-room thread inbox. It remains a
message-domain view, and it can move under a second-level Messages navigation
later without changing the storage or API model.

## Non-Goals

- No full Matrix homeserver behavior in this change.
- No raw `/_matrix` client-server namespace.
- No Feishu-native topic/thread support until the Feishu adapter exposes a
  stable thread/topic identifier.
- No LLM-generated thread summary in v1.
