---
name: agent-teams
description: Use this skill when you are a worker operating inside a CSGClaw team room. Claim the next task, report blocked/completed/failed status through the team CLI, and keep all execution aligned with the room-scoped task state.
---

# Agent Teams Worker

Use this skill when the manager is coordinating work through CSGClaw team tasks instead of `manager-worker-dispatch`.

## Phase 3a Scope

Worker actions in this phase:

- claim the next task assigned to you
- report `blocked`, `completed`, or `failed`
- request approval through the manager or human when needed

Do not self-assign work. Use `claim-next`, then work only on the claimed task.

## Commands

Claim the next available task:

```bash
csgclaw-cli team task claim-next --team <team_id> --bot-id <worker_bot_id>
```

Report a completed task:

```bash
csgclaw-cli team task update --team <team_id> --task <task_id> --actor-id <worker_bot_id> --status completed --result "<summary>"
```

Report a blocked task:

```bash
csgclaw-cli team task update --team <team_id> --task <task_id> --actor-id <worker_bot_id> --status blocked --reason "<why blocked>"
```

Report a failed task:

```bash
csgclaw-cli team task update --team <team_id> --task <task_id> --actor-id <worker_bot_id> --status failed --error "<failure>"
```

## Working Rules

- For any team task that creates, inspects, or updates a project, use `~/.openclaw/workspace/projects/{name}` as the shared project directory. Check there first before creating a new project path.
- Keep project artifacts, notes, and generated files under the same `~/.openclaw/workspace/projects/{name}` tree so managers and workers can inspect the same content.
- Claim before you execute. Do not start speculative work on pending tasks.
- When blocked by a command or external action, update the task to `blocked` and explain the reason clearly.
- If the room later shows an approval resolution, refresh your understanding from the room and continue on the same task if it moved back to `in_progress`.
- Keep your room reply short and factual: what changed, what result you produced, or what blocker remains.
