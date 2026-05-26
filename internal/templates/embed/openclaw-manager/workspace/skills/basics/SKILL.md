---
name: basics
description: Handle the most common basic CSGClaw CLI administration tasks. Use when the Manager needs to create a room, list bots, create a bot, inspect room members, add a bot into a room, notify a worker in IM (`message create --mention-id`, never plain @name), search ClawHub skills, or perform similar direct `csgclaw-cli` operations. Use worker dispatch (not manager install) when a skill must land in a worker sandbox.
---

# CSGClaw CLI Basics

Execute common `csgclaw-cli` operations directly and keep the flow simple.
Prefer this skill whenever the user is asking for basic room, bot, or member management.

## Scope

This skill covers direct CLI actions such as:

- create a room
- list rooms
- list all bots
- create a bot
- list room members
- add a bot as a room member
- send a message, including a message with a mention
- search, list versions, or inspect ClawHub skills (`skill search`, `skill versions`, `skill get`)
- check command help for the current CLI surface before assuming flags

Do not use this skill when the task requires any of the following:

- break a request into multiple worker-owned tasks
- orchestrate a multi-worker workflow
- manage cross-worker sequencing or tracking state

For **installing** a ClawHub skill into a **worker** sandbox, dispatch to that worker and have it run `csgclaw-cli skill install` locally (see ClawHub section below). The manager must not install into another agent's filesystem.

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
csgclaw-cli bot create --id u-alex --name alex --description "frontend worker for settings tasks" --role worker --channel <current_channel>
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
| `csgclaw-cli message create ... --mention-id u-gitlab-worker` (ID from `bot list`) | Type `@gitlab-worker` or `@worker-name` in `--content`, room replies, or channel `message` tools |
| Verify delivery with `message list` — content must include `<at user_id="u-...">` | Assume a human-style `@` in prose wakes the worker |
| Run `bot list` and `member list` before the first dispatch | Skip membership checks and post assignment text only |

Minimal handoff flow:

1. `csgclaw-cli bot list` — resolve the worker **bot ID** (e.g. `u-gitlab-worker`, not the display name).
2. `csgclaw-cli member list` — confirm the worker is in the room; `member create` if missing.
3. `csgclaw-cli message create` with `--mention-id` and the task body (install command, issue link, etc.).
4. `csgclaw-cli message list` — confirm the stored message contains `<at user_id="...">`.

For multi-worker sequencing, use `manager-worker-dispatch` (`start-tracking`) instead of manual room messages.

## ClawHub skills (`skill`)

Do **not** use OpenClaw/PicoClaw built-in `find_skills` or `install_skill`. They are disabled here; always use the `csgclaw-cli skill` commands below via `exec`.

Use `csgclaw-cli skill` against ClawHub-compatible registries. **search** tries **opencsg** (`claw.opencsg.com`) first and returns immediately on hits; it only queries **clawhub** (`clawhub.ai`) when opencsg has no results. Results include a `REGISTRY` column.

Registry skill slugs are **case-sensitive**; copy the exact `SLUG` and `REGISTRY` from search results.

| Subcommand | Purpose |
|------------|---------|
| `search <query>` | Search opencsg first; clawhub if empty (`--limit`, default 20) |
| `get <slug>` | Show one skill; `--registry opencsg\|clawhub` or auto (opencsg first) |
| `versions <slug>` | List published versions (newest first); `--registry`, `--limit` |
| `install <slug>` | Install into **this runtime's** `workspace/skills/<slug>/` (no `--agent`) |

### Manager vs worker install

Each agent runs in its own sandbox with its own mounted workspace. **`skill install` only writes to the local workspace** (default `~/.picoclaw/workspace/skills` or `~/.openclaw/workspace/skills` inside the container).

- **Manager** may run `skill search`, `skill get`, and `skill versions` to discover skills.
- To install a skill for **gitlab-worker** (or any worker), **dispatch** with `message create --mention-id` (see **Notifying workers in IM**). The worker runs install in **its** container; the manager only discovers via `skill search` / `get` / `versions`.

Example dispatch after search (replace room ID, worker ID, slug, and channel):

```bash
csgclaw-cli message create \
  --room-id <room_id> \
  --sender-id u-manager \
  --mention-id u-gitlab-worker \
  --content "Install ClawHub skill AIWizards--gitlab-fullstack-pro from opencsg. Run: csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg --version 1.0.0" \
  --channel <current_channel>
```

Do **not** run `skill install` from the manager sandbox for another agent. Do **not** post `@gitlab-worker` plain text in the room instead of `--mention-id`.

### Examples (run inside the target agent container)

Search and inspect:

```bash
csgclaw-cli skill search gitlab --limit 10
csgclaw-cli skill versions AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill get AIWizards--gitlab-fullstack-pro --version 1.0.0
```

Install (worker or any agent installing for itself):

```bash
csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg --version 1.0.0
csgclaw-cli skill install gitlab --registry clawhub --force
```

Optional override when workspace is non-standard:

```bash
csgclaw-cli skill install my-skill --skills-dir ~/.picoclaw/workspace/skills
```

Add `-o json` on any subcommand when structured output is needed. Run `csgclaw-cli skill -h` or `csgclaw-cli skill <subcommand> -h` for flags.

## Operating Rules

- Prefer direct `csgclaw-cli` commands over ad hoc HTTP calls.
- Use `bot list` before creating a new bot if the user may be referring to an existing one.
- When creating a bot, always pass a meaningful `--description` so later matching and reuse remain clear.
- Verify room membership with `member list` after adding a member when room presence matters.
- A direct room cannot accept an added bot as a new member. Create a new room with `--member-ids` containing the existing DM bots and the new bot.
- Keep `csgclaw-cli` parameters bot-facing across channels: use bot IDs such as `u-manager`, `u-dev`, and `u-alex`.
- For ClawHub skills, never guess or lower-case the slug; use the exact value from `skill search` or `skill get`.
- Never call `find_skills` or `install_skill`; use `csgclaw-cli skill` only.
- Never install a ClawHub skill into another agent from the manager sandbox; dispatch the worker to install locally.
- Never notify a worker with plain-text `@name`; always use `message create --mention-id` and verify `<at user_id="...">` in `message list`.
- Use `skill get` / `skill versions` before install when moderation status or version choice matters.
- Keep the response focused on the concrete CLI result instead of introducing external planning artifacts.
- Hand off to `manager-worker-dispatch` only if the user explicitly needs manager orchestration or multi-worker sequencing.
