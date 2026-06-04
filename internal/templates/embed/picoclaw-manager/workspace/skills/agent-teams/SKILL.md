---
name: agent-teams
description: Default manager skill for multi-worker CSGClaw orchestration. Create teams and tasks, plan and start work through `csgclaw-cli team`, and prefer this over manager-worker-dispatch so dispatch, claim, and status stay on server task state.
---

# Agent Teams Manager

Use this skill when you are the manager and need to coordinate one or more workers through CSGClaw team tasks.

Use `manager-worker-dispatch` only when the user explicitly needs tracker-driven sequential handoff outside team tasks.

## Default orchestration path

For executable multi-worker work:

1. Ensure a team exists (`team create` or reuse an existing `team_id`).
2. Create a main task (`team task create-batch`).
3. Plan subtasks with the manager LLM planner flow.
4. Start execution from Web Tasks **Start** when the CLI start command is not available. The server creates a **dedicated execution room** for that main task and projects dispatches there with structured mentions.
5. Do **not** also run `manager-worker-dispatch` / `start-tracking` for the same work.

Workers must claim and update tasks via `agent-teams` (specified `team task claim --task`, not ad-hoc room handoff).

## Manager actions

- create a team and tasks with `team create` / `team task create-batch`
- plan and start parent tasks through the shipped CLI or Web Tasks controls
- inspect progress with `team task list`
- inspect pending approvals with `team approval list`
- resolve approvals with `team approval resolve`
- summarize status back into the execution room in plain language

Do not invent extra commands. Stay within the shipped CLI surface and Web Tasks controls.

## Commands

Create or enable a team room:

```bash
csgclaw-cli team create --channel csgclaw --room-id <room_id> --lead-bot-id <manager_bot_id> --member-bot-ids <worker_bot_ids>
```

Create one or more tasks:

```bash
csgclaw-cli team task create-batch --team <team_id> --created-by <manager_bot_id> --file <tasks.json>
```

Recommended batch shape for a main task plus subtasks:

```json
{
  "tasks": [
    {
      "id_ref": "story",
      "title": "Release v1"
    },
    {
      "title": "Draft release note",
      "parent_ref": "story",
      "assign_to": "u-writer"
    },
    {
      "title": "Smoke test",
      "parent_ref": "story",
      "assign_to": "u-tester"
    }
  ]
}
```

Attach a new subtask to an existing main task with `parent_id`:

```json
{
  "tasks": [
    {
      "title": "Prepare rollback note",
      "parent_id": "task-12",
      "assign_to": "u-writer"
    }
  ]
}
```

List tasks:

```bash
csgclaw-cli team task list --team <team_id>
```

List approvals:

```bash
csgclaw-cli team approval list --team <team_id>
```

Resolve an approval:

```bash
csgclaw-cli team approval resolve --team <team_id> --approval <approval_id> --status approved --reason "<note>"
csgclaw-cli team approval resolve --team <team_id> --approval <approval_id> --status rejected --reason "<reason>"
```

## Working Rules

- For any team task that creates, inspects, or updates a project, use `~/.picoclaw/workspace/projects/{name}` as the shared project directory. Check there first before creating a new project path.
- Keep project artifacts, notes, and generated files under the same `~/.picoclaw/workspace/projects/{name}` tree so managers and workers can inspect the same content.
- Default to one top-level main task for each user-visible goal, then add execution items as child tasks.
- In the same batch, prefer `id_ref` + `parent_ref` to build the hierarchy clearly.
- Use `parent_id` only when attaching a child task to an already-existing main task.
- Do not flatten every execution step into a top-level task; the global Tasks page groups by main task.
- Prefer a single-item batch when you only need one task; do not wait for a separate single-task command.
- After **Start**, collaboration for that main task happens in its execution room (not the team home room).
- Keep plans small and explicit when possible; prefer clear parent/child tasks over flat room chatter.
- After listing tasks and approvals, summarize only the current state, blockers, and next action.
- If a human already handled an approval or cancellation in the room with `approve`, `reject`, or `cancel`, refresh with `team task list` or `team approval list` before acting again.
