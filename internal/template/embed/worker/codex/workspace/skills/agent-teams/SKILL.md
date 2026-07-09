---
name: agent-teams
description: Use this skill when you are a worker operating inside a CSGClaw task execution room. Only act on explicit task dispatch messages, claim the dispatched task, report blocked/completed/failed status through the team CLI, and keep all execution aligned with server task state.
---

# Agent Teams Worker

Use this skill when the manager is coordinating work through CSGClaw team tasks.

Only begin work after an explicit dispatch message in the task execution room:

```text
@<worker_display_name> <manager_display_name> dispatched <task_id> to <worker_display_name>. Claim: csgclaw-cli team task claim --team <team_id> --task <task_id> --participant-id <worker_participant_id>
```

The claim command uses participant IDs, for example `u-frontend-dev`. Rendered mentions may show only the display name or handle, for example `@frontend-dev`; do not use the display name as the CLI participant ID unless the claim command shows it.

Do not use legacy `--bot-id` for team commands. If `csgclaw-cli team task claim --help` does not show `--participant-id`, the sandbox has a stale `csgclaw-cli`; report that blocker and stop instead of retrying with `--bot-id`.

Ignore team setup and planning events, including messages such as `enabled team`, `created task`, `created tasks`, `completed planning for task`, and `created execution room`. Those messages are not permission to start work.

## Worker actions

- claim the **dispatched** task with the exact `<team_id>` and `<task_id>` from the dispatch message
- report `blocked`, `completed`, or `failed`
- request approval through the manager or human when needed

Do not self-assign work. When the room dispatch message includes a task id, always use `--task`. Use `claim-next` only when no task id was provided.

Never infer `team_id` from the room id. A room id such as `room-178...` is not a team id. Use the `--team <team_id>` value shown in the dispatch message. If the only available value starts with `room-`, stop and report blocked instead of continuing.

## Commands

Claim the dispatched task:

```bash
csgclaw-cli team task claim --team <team_id> --task <task_id> --participant-id <worker_participant_id>
```

If the dispatch did not include a task id, claim the next available task:

```bash
csgclaw-cli team task claim-next --team <team_id> --participant-id <worker_participant_id>
```

After claiming, confirm the response status is `in_progress` for the same task before doing the work.

Report a completed task:

```bash
csgclaw-cli team task update --team <team_id> --task <task_id> --actor-id <worker_participant_id> --status completed --result "<summary>"
```

The task is not complete until this CLI status update succeeds. Sending a normal room message with the result is useful, but it does not update task state.
Do not update tracker files or rely on manual handoff; task completion is recorded only through the team CLI.

Report a blocked task:

```bash
csgclaw-cli team task update --team <team_id> --task <task_id> --actor-id <worker_participant_id> --status blocked --reason "<why blocked>"
```

Report a failed task:

```bash
csgclaw-cli team task update --team <team_id> --task <task_id> --actor-id <worker_participant_id> --status failed --error "<failure>"
```

## Working Rules

- For any team task that creates, inspects, or updates a project, use `~/.openclaw/workspace/projects/{name}` as the shared project directory. Check there first before creating a new project path.
- Keep project artifacts, notes, and generated files under the same `~/.openclaw/workspace/projects/{name}` tree so managers and workers can inspect the same content.
- Claim before you execute. Do not start speculative work on pending tasks.
- Complete/fail/block through the same `<team_id>/<task_id>` that you claimed; do not finish with only a chat message.
- When blocked by a command or external action, update the task to `blocked` and explain the reason clearly.
- If the room later shows an approval resolution, refresh your understanding from the room and continue on the same task if it moved back to `in_progress`.
- Keep your room reply short and factual: what changed, what result you produced, or what blocker remains.
