---
name: manager-worker-dispatch
description: Use this skill to break an admin request into capability-aligned subtasks, provision or reuse workers through manager_worker_api, write the dispatch plan to todo.json, and start sequential task tracking. Do NOT use for generic planning or single-agent execution.
---

# Manager Worker Dispatch

Break an admin request into clear tasks, choose workers by capability, and dispatch them through the real CSGClaw API in sequence.

Reuse the bundled script instead of writing ad hoc requests.
Check the script help for the current CLI surface instead of reading reference docs.

## Fast Path

If the admin explicitly asks the manager to arrange or reuse workers such as `ux`, `dev`, and `qa`, do this directly:

1. Do not do the implementation work yourself.
2. Do not use `message` for progress chatter or to restate the request.
3. Do not use `spawn` or `subagent`.
4. Run `list-workers`, reuse matching workers, and create a worker only if a required capability is missing.
5. Ensure the chosen workers have joined the target room.
6. Write `todo.json` under `~/.picoclaw/workspace/projects/<slug>/todo.json`.
7. Start `start-tracking`.
8. Send at most one concise final room reply after tracking starts successfully.

If you already know this workflow and the script path is clear, do not reread this file just to paraphrase it back to the user.
Do not inspect or modify project implementation files before dispatch unless you need to choose the project slug or update `todo.json`.

## Workflow

1. Break the admin request into concrete deliverables.
2. Match each task to the needed capability; run `list-workers` first, reuse by matching `description`, and create a worker only when needed.
3. Ensure the required workers have joined the target room.
4. Choose a suitable project directory under `~/.picoclaw/workspace/projects`; create a short slug directory if none fits.
5. Write or overwrite `todo.json` in that directory as the only source of truth for the current dispatch plan.
6. Start `scripts/manager_worker_api.py start-tracking` against that `todo.json`.
7. Let the tracker own sequential handoff; workers must reply in-room with results or blockers, and neither the manager nor workers should manually assign the next worker while tracking is active.

Inside a manager/worker box, the shared project tree is `~/.picoclaw/workspace/projects`.
On the host machine, that same mount is `~/.csgclaw/projects`.
When reporting a project path back to a human user, translate the in-box path to the host path. Example:

- in box: `~/.picoclaw/workspace/projects/kanban-board`
- on host: `~/.csgclaw/projects/kanban-board`

## todo.json

`todo.json` must be valid JSON.

- Single task: write one task object.
- Multiple tasks: write `{ "tasks": [...] }`; array order is dispatch order.

Each task should keep these fields:

- `id`: task number, required, use `1`, `2`, `3` in dispatch order
- `assignee`: owner, usually a worker name or role-like label
- `category`: short task type such as `feature`, `bug`, or `test`
- `description`: task summary
- `steps`: array of execution steps
- `passes`: completion state, usually `false` at the start
- `progress_note`: progress, result, or blocker note, usually an empty string at the start

`id` must always be present and should increase sequentially with the task order in `todo.json`.

While tracking is active, task completion is a two-part gate:

- update `passes` to `true` and write a useful `progress_note`
- post a normal in-room reply to `@manager` with the result or blocker summary

Tool trace messages are not enough for handoff. The tracker waits for both the `todo.json` update and the assignee's room reply before dispatching the next task.

Example:

```json
{
  "tasks": [
    {
      "id": 1,
      "assignee": "frontend",
      "category": "feature",
      "description": "Build the settings page UI and connect the save action.",
      "steps": [
        "Implement the settings page layout",
        "Connect the save action to the API",
        "Reply to the manager with the implementation summary"
      ],
      "passes": false,
      "progress_note": ""
    },
    {
      "id": 2,
      "assignee": "qa",
      "category": "test",
      "description": "Validate the main settings page flows after frontend delivery.",
      "steps": [
        "Verify the main edit and save flows",
        "Record regressions and blockers",
        "Reply to the manager with QA results"
      ],
      "passes": false,
      "progress_note": ""
    }
  ]
}
```

## Capability Mapping

Choose workers by `description`, not just by `role`.

- `frontend`: UI, page work, styling, interaction
- `backend`: APIs, services, storage, data flow
- `qa`: validation, regression, acceptance checks

Split cross-capability work into multiple tasks instead of giving one vague package to a single worker.

## Script Usage

```bash
cd ~/.picoclaw/workspace/skills/manager-worker-dispatch
python scripts/manager_worker_api.py list-workers
python scripts/manager_worker_api.py create-worker --name alex --description "qa regression testing"
python scripts/manager_worker_api.py join-worker --room-id room-123 --worker-id u-alex
python scripts/manager_worker_api.py start-tracking --room-id room-123 --todo-path ~/.picoclaw/workspace/projects/demo/todo.json
python scripts/manager_worker_api.py stop-tracking --todo-path ~/.picoclaw/workspace/projects/demo/todo.json
```
Use `python scripts/manager_worker_api.py -h` to inspect the latest commands, flags, and environment variable fallbacks before invoking or updating the workflow.

If you need to direct the human user to the project files on their Mac, point them to the host-side path such as `~/.csgclaw/projects/demo/todo.json`, not the in-box `/home/picoclaw/...` path.

## Operating Rules

- Reuse workers before creating new ones.
- Keep `todo.json` aligned with the actual assignment being dispatched.
- Do not casually reorder tasks in the sequential flow.
- Let `start-tracking` drive dispatch from `todo.json`; do not duplicate that logic in manual room-message procedures.
- While tracking is active, do not manually tell the next worker to start in prose. The tracker is the only sequencer.
- When a worker finishes, they must reply in the shared room with a normal summary or blocker note; updating `todo.json` alone does not release the next task.
- If the API response shape differs from expectations, patch the script instead of improvising around it.
