---
name: agent-teams
description: Default manager skill for multi-worker CSGClaw orchestration. Create teams and tasks, plan and start work through `csgclaw-cli team`, and keep dispatch, claim, dependencies, and status on server task state.
---

# Agent Teams Manager

Use this skill when you are the manager and need to coordinate one or more workers through CSGClaw team tasks.

Do not create tracker files, run tracker scripts, or manually sequence worker handoff. Agent Teams is the source of truth for dispatch, claim, dependencies, progress, blockers, and completion.

## Default orchestration path

For executable multi-worker work:

1. Inspect existing participants and choose capable available workers before creating new ones.
2. Resolve the current channel context. Use `csgclaw` by default; use `feishu` when the user request came from Feishu.
3. Ensure selected workers have participant bindings in that execution channel. If a required worker is missing or unavailable, use `agent-creator` first, then return here after provisioning finishes.
4. Ensure a team member pool exists (`team create` or reuse an existing `team_id`).
5. Create one main task (`team task create-batch --execution-channel <channel>`) for the user-visible goal. This creates the execution room for the main task.
6. Plan and start it with `team task plan --start`; this uses the server planner, creates child tasks, creates dependencies, and dispatches ready child tasks.
7. Use `team task start` only when the parent already has subtasks but is not started yet. When `create-batch` creates a parent and runnable subtasks in the same batch, the server auto-starts the parent and projects dispatches.
8. Verify dispatch through `team task list` or recent room messages when reporting success.

Workers must claim and update tasks via `agent-teams` (specified `team task claim --task`, not ad-hoc room handoff).

## Manager actions

- create a team and tasks with `team create` / `team task create-batch`
- plan and start parent tasks through the shipped CLI
- use dependencies (`depends_on_refs`) for ordered handoff instead of tracker files
- inspect progress with `team task list`
- inspect pending approvals with `team approval list`
- resolve approvals with `team approval resolve`
- summarize status back into the execution room in plain language

Do not invent extra commands. Stay within the shipped CLI surface and Web Tasks controls.

## Commands

Create a reusable team member pool:

```bash
csgclaw-cli team create --title "<team_title>" --lead-agent-id <manager_agent_id> --member-agent-ids <worker_agent_ids>
```

Create one or more tasks:

```bash
csgclaw-cli team task create-batch --team <team_id> --created-by <manager_participant_id> --execution-channel <csgclaw|feishu> --file <tasks.json>
```

Start an existing parent task after subtasks are attached:

```bash
csgclaw-cli team task start --team <team_id> --task <parent_task_id>
```

Plan and start a parent task through the server planner:

```bash
csgclaw-cli team task plan --team <team_id> --task <parent_task_id> --start
```

Recommended batch shape for the default path:

```json
{
  "tasks": [
    {
      "id_ref": "story",
      "title": "Release v1",
      "body": "Goal, context, scope, and acceptance criteria for the server planner."
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
      "assign_to": "writer"
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
- Add `depends_on_refs` when testing, QA, review, validation, or release checks must wait for implementation work.
- Use task dependencies for sequential handoff; do not write or ask workers to update tracker files.
- Use `parent_id` only when attaching a child task to an already-existing main task.
- Do not flatten every execution step into a top-level task; the global Tasks page groups by main task.
- Prefer a single-item batch when you only need one task; do not wait for a separate single-task command.
- After `team task create-batch`, collaboration for that main task happens in its execution room. Do not use or create a team home room for Agent Teams.
- Keep plans small and explicit when possible; prefer clear parent/child tasks over flat room chatter.
- After listing tasks and approvals, summarize only the current state, blockers, and next action.
- If a human already handled an approval or cancellation in the room with `approve`, `reject`, or `cancel`, refresh with `team task list` or `team approval list` before acting again.
