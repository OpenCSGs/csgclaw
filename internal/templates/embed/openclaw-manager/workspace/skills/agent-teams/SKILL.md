---
name: agent-teams
description: Use this skill for MVP team orchestration in CSGClaw rooms. Create or inspect room-scoped tasks and approvals through `csgclaw-cli team`, keep the room history readable, and prefer this skill over manager-worker-dispatch when the work fits a single manager plus team task flow.
---

# Agent Teams Manager

Use this skill when you are the manager in a CSGClaw team room and the workflow should stay on the built-in team/task/approval API.

Use `manager-worker-dispatch` as fallback when the request needs tracker-driven sequential handoff, richer worker choreography, or capabilities not covered below.

## Phase 3a Scope

Manager actions in this phase:

- create a small task plan with `team task create-batch`
- inspect progress with `team task list`
- inspect pending approvals with `team approval list`
- resolve approvals with `team approval resolve`
- summarize status back into the room in plain language

Do not invent extra commands. Stay within the shipped CLI/API surface.

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
      "assign_to": "bot-writer"
    },
    {
      "title": "Smoke test",
      "parent_ref": "story",
      "assign_to": "bot-tester"
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
      "assign_to": "bot-writer"
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

- For any team task that creates, inspects, or updates a project, use `~/.openclaw/workspace/projects/{name}` as the shared project directory. Check there first before creating a new project path.
- Keep project artifacts, notes, and generated files under the same `~/.openclaw/workspace/projects/{name}` tree so managers and workers can inspect the same content.
- Default to one top-level main task for each user-visible goal, then add execution items as child tasks.
- In the same batch, prefer `id_ref` + `parent_ref` to build the hierarchy clearly.
- Use `parent_id` only when attaching a child task to an already-existing main task.
- Do not flatten every execution step into a top-level task; the global Tasks page groups by main task.
- Prefer a single-item batch when you only need one task; do not wait for a separate single-task command.
- Keep plans small and explicit in Phase 3a: one manager, one worker, short serial flow.
- After listing tasks and approvals, summarize only the current state, blockers, and next action.
- If a human already handled an approval or cancellation in the room with `approve`, `reject`, or `cancel`, refresh with `team task list` or `team approval list` before acting again.
