---
name: basics
description: Handle routine CSGClaw CLI administration for rooms, bot listing, room members, and IM mentions. Use for list bots, member create, message create, and room operations. Do NOT use for creating a new worker—use agent-creator instead (hub list + bot create --from-template).
---

# CSGClaw CLI Basics

Execute common `csgclaw-cli` operations directly and keep the flow simple.
Prefer this skill for room, member, and message operations after workers already exist.

## Scope

This skill covers direct CLI actions such as:

- create a room
- list rooms
- list all bots
- list room members
- add a bot as a room member
- send a message, including a message with a mention
- check command help for the current CLI surface before assuming flags

Do **not** use this skill to **create a new worker**. For any new agent/bot/worker provisioning, use `agent-creator` (`hub list`, `hub get`, `bot create --from-template`).

Do not use this skill when the task requires any of the following:

- break a request into multiple worker-owned tasks
- orchestrate a multi-worker workflow
- manage cross-worker sequencing or tracking state
- create an agent from a hub template with required image env vars

For hub template selection and `--from-template` creation, use `agent-creator` instead.

## Workflow

1. Identify the exact room, bot, or member operation the user needs.
2. If room context matters, inspect it first with `room list` or `member list`, especially to see whether the room is direct.
3. Run `csgclaw-cli <entity> -h` or `csgclaw-cli <entity> <verb> -h` if the current command surface is not already clear.
4. Execute the smallest direct CLI command that completes the request.
5. Show the user the key result such as the room ID, bot ID, member list summary, or sent message result.

## Common Commands

Create a room:

```bash
csgclaw-cli room create --title test-room --creator-id u-manager --member-ids u-manager,u-dev --channel <current_channel>
```

Use CSGClaw bot IDs in room, member, and message commands.

List rooms and check whether a room is direct:

```bash
csgclaw-cli room list --channel <current_channel>
```

List bots:

```bash
csgclaw-cli bot list --channel <current_channel>
```

Create a bot. Always include `--description`:

```bash
# Do not use this for new workers. Use agent-creator with --from-template instead.
```

List members in a room:

```bash
csgclaw-cli member list --room-id oc_xxx --channel <current_channel>
```

Add a bot into a non-direct room:

```bash
csgclaw-cli member create --room-id oc_xxx --user-id u-alex --inviter-id u-manager --channel <current_channel>
```

If the current room is direct in the local `csgclaw` channel, do not try to add the bot directly. Create a new room that includes the current DM participants plus the new bot:

```bash
csgclaw-cli room create \
  --title "manager-dev-alex" \
  --creator-id u-manager \
  --member-ids u-manager,u-dev,u-alex \
  --channel <current_channel>
```

For Feishu, keep the same bot ID parameters:

```bash
csgclaw-cli room create \
  --title "manager-dev-alex" \
  --creator-id u-manager \
  --member-ids u-manager,u-dev,u-alex \
  --channel feishu
```

Send a message with a mention. Use the mentioned bot ID for `--mention-id`:

```bash
csgclaw-cli message create --room-id oc_xxx --sender-id u-manager --content "Please take a look." --mention-id u-alex --channel <current_channel>
```

## Notifying workers in IM (critical)

Workers are configured with **`mention_only`**: they only process group messages that contain a structured mention tag, not plain text like `@gitlab-worker`.

| Do | Do not |
|----|--------|
| `csgclaw-cli message create ... --mention-id u-gitlab-worker` (ID from `bot list`) | Type `@gitlab-worker` or `@worker-name` in `--content`, room replies, or the PicoClaw `message` tool |
| Verify delivery with `message list` — content must include `<at user_id="u-...">` | Assume a human-style `@` in prose wakes the worker |
| Run `bot list` and `member list` before the first dispatch | Skip membership checks and post assignment text only |

Minimal handoff flow:

1. `csgclaw-cli bot list` — resolve the worker **bot ID** (e.g. `u-gitlab-worker`, not the display name).
2. `csgclaw-cli member list` — confirm the worker is in the room; `member create` if missing.
3. `csgclaw-cli message create` with `--mention-id` and the task body.
4. `csgclaw-cli message list` — confirm the stored message contains `<at user_id="...">`.

For multi-worker team tasks, use `agent-teams` (`csgclaw-cli team` plan/start) instead of manual room messages. Use `manager-worker-dispatch` only when team tasks are not in use.

Example worker handoff (replace room ID, worker ID, and channel):

```bash
csgclaw-cli message create \
  --room-id <room_id> \
  --sender-id u-manager \
  --mention-id u-alex \
  --content "Please implement the login page changes we discussed." \
  --channel <current_channel>
```

Do **not** post `@alex` plain text in the room instead of `--mention-id`.

## Operating Rules

- Prefer direct `csgclaw-cli` commands over ad hoc HTTP calls.
- Use `bot list` before creating a new bot if the user may be referring to an existing one.
- When a **new** worker is needed, use `agent-creator`; do not run bare `bot create` from this skill.
- Verify room membership with `member list` after adding a member when room presence matters.
- A direct room cannot accept an added bot as a new member. Create a new room with `--member-ids` containing the existing DM bots and the new bot.
- Keep `csgclaw-cli` parameters bot-facing across channels: use bot IDs such as `u-manager`, `u-dev`, and `u-alex`.
- Never notify a worker with plain-text `@name`; always use `message create --mention-id` and verify `<at user_id="...">` in `message list`.
- Keep the response focused on the concrete CLI result instead of introducing external planning artifacts.
- Hand off to `agent-teams` for multi-worker team orchestration; use `manager-worker-dispatch` only if the user explicitly needs tracker handoff outside team tasks.
