---
name: manager-worker-dispatch
description: Use this skill only for manager-led multi-worker coordination in CSGClaw. Break a request into capability-aligned subtasks, decide which workers should handle them, prepare `todo.json`, and start sequential task tracking with `manager_worker_api`. Use this when the work must be dispatched across multiple workers with ordered handoff, not for generic planning, single-agent execution, or routine CLI administration.
---

# Manager Worker Dispatch

Break a multi-worker admin request into clear tasks, choose workers by capability, and dispatch them through CSGClaw's real local interfaces in sequence.

Use the `basics` skill for room, participant, and member operations that support the dispatch flow. Use `skill-installer` for registry skill search; workers use `skill-installer` for install (not `find_skills`).
Use `scripts/manager_worker_api.py` only for `start-tracking` and `stop-tracking`.
Check the current CLI surface through the `basics` skill instead of writing ad hoc API requests.

If the user only needs a direct CLI action and there is no real dispatch workflow, use the `basics` skill instead of this skill.

If the user wants to create or provision a new agent from hub templates (not execute a task), use the `agent-creator` skill instead of this skill.

## Out of Scope

Do not use this skill for:

- hub template listing, matching, or selection
- collecting template `image_env` values
- creating agents with `--from-template`

For those flows, use `agent-creator`. Never create a new worker with bare `participant create --bind create` from dispatch.

## Mandatory Dispatch Order

When a user asks for manager-led coordination (especially GitLab planning, issue queries, breakdown, assignment, or execution handoff), follow this order and do not skip steps:

1. Read this skill first before any domain execution.
2. Check existing workers first (`participant list`) and prefer reusing a capable available worker.
3. Ensure the selected worker is a member of the target room (add if missing).
4. If no suitable worker exists or the matching one is `unavailable`:
   - **New worker needed:** stop dispatch, follow `agent-creator` (template list → template get → `participant create --type agent --bind create --from-template` + `--env`), then use `basics` to add the worker to the room.
   - **Existing worker unavailable:** use `basics` / recreate paths for that participant only; do not bare-create a replacement.
5. Dispatch the task to the worker in-room after membership is confirmed.

Do not start with repo/code exploration, web fetch/search, or manager self-execution when the task is delegable to an existing or creatable worker.
Do not skip `participant list` by assuming worker availability from memory or prior turns.

## Fast Path

If the admin explicitly asks the manager to arrange or reuse workers such as `ux`, `dev`, and `qa`, do this directly:

1. Do not do the implementation work yourself.
2. Do not use `message` for progress chatter or to restate the request.
3. Do not use `spawn` or `subagent`.
4. Use the `basics` skill to inspect existing workers, reuse matching available workers, and use `agent-creator` only if a required capability is missing or the matching worker is `unavailable` and must be replaced with a new hub-template worker.
5. Use the `basics` skill to ensure every chosen worker has joined the target room and verify room membership.
6. Only after all required workers are present in the room, write `todo.json` under `~/.openclaw/workspace/projects/<slug>/todo.json`.
7. Only after that room-membership check passes, start tracking with `scripts/manager_worker_api.py start-tracking`.
8. Verify the first tracker dispatch message is actually delivered in-room (must include a real mention, i.e. `mention_id` targeting the assignee user; rendered as `<at user_id="...">...</at>` in message content).
9. Send at most one concise final room reply after dispatch delivery is verified.

If you already know this workflow and the relevant command surface is clear, do not reread this file just to paraphrase it back to the user.
Do not inspect or modify project implementation files before dispatch unless you need to choose the project slug or update `todo.json`.

## Workflow

1. Break the admin request into concrete deliverables.
2. Match each task to the needed capability; use the `basics` skill to inspect existing workers first (`participant list`) and reuse by matching `description`.
3. If a suitable worker does not exist or is `unavailable`, provision via `agent-creator` (new) or recreate the existing worker before dispatch.
4. Use the `basics` skill to ensure every required worker has joined the target room, then verify the full required worker set.
5. Dispatch the user task to selected workers in-room after membership checks pass.
6. Choose a suitable project directory under `~/.openclaw/workspace/projects`; create a short slug directory if none fits.
7. Write or overwrite `todo.json` in that directory as the only source of truth for the current dispatch plan, but only after the room-membership verification succeeds.
8. Start `scripts/manager_worker_api.py start-tracking` against that `todo.json`, but only after all required workers are confirmed present in the room.
9. Let the tracker own sequential handoff; workers must reply in-room with results or blockers, and neither the manager nor workers should manually assign the next worker while tracking is active.
10. After `start-tracking`, run an explicit dispatch-delivery check before claiming success: query recent room messages and confirm the tracker has posted a task message with a real mention (`mention_id` targeting the assignee participant; rendered as `<at user_id="...">...</at>`).

## Room Membership Gate

Room membership is a hard gate before dispatch.

- Determine the complete set of workers required by `todo.json` before starting tracking.
- Use the `basics` skill to add any missing worker into the room.
- Use the `basics` skill to verify every required worker ID or name appears in the target room.
- Do not write the final `todo.json`, run `start-tracking`, or send any assignment/progress message until every required worker is confirmed present in that room.
- If any required worker is still missing from the room, fix membership first; never rely on `start-tracking` to invite or discover missing workers.

Inside a manager/worker box, the shared project tree is `~/.openclaw/workspace/projects`.
On the host machine, that same mount is `~/.csgclaw/projects`.
When reporting a project path back to a human user, translate the in-box path to the host path. Example:

- in box: `~/.openclaw/workspace/projects/kanban-board`
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
- post an in-room mention message to manager with blocker summary if blocked

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

## Dependency on `basics`

Use the `basics` skill whenever this workflow needs any of these supporting operations:

- create the target room
- list workers or participants
- add a worker into the room
- verify room membership before tracking

When a **new** worker is required, use `agent-creator` instead of bare `participant create --bind create`. Use `basics` here only for recreate when an existing listed worker is `unavailable`.

## Tracking Script Usage

```bash
cd ~/.openclaw/workspace/skills/manager-worker-dispatch
python scripts/manager_worker_api.py start-tracking -h
python scripts/manager_worker_api.py stop-tracking -h
```

Start tracking todo:

```bash
python scripts/manager_worker_api.py start-tracking --channel <current_channel> --room-id room-123 --todo-path ~/.openclaw/workspace/projects/demo/todo.json
```

Dispatch delivery verification (mandatory, immediately after start):

```bash
csgclaw-cli --output json message list --room-id <target_room_id> --channel <current_channel>
```

Success criteria:

- There is a new tracker dispatch message from the manager user in the target room.
- The dispatch message includes mention to the selected assignee (`mention_id` equals the worker user ID). Do **not** rely on plain-text `@name`; CSGClaw renders mentions as `<at user_id="...">Name</at>`.
- Message content matches the task dispatch text pattern (task id/todo path context).

If verification does not pass:

1. Wait briefly and re-check message list up to 3 times.
2. If still missing, report dispatch failure with evidence.
3. Do **not** send a manual fallback assignment message while tracking is active.
4. Stop tracking if needed, fix root cause (assignee name mismatch, room membership mismatch, user ID mismatch), then restart tracking.

Stop the tracking:

```bash
python scripts/manager_worker_api.py stop-tracking --todo-path ~/.openclaw/workspace/projects/demo/todo.json
```

If you need to direct the human user to the project files on their Mac, point them to the host-side path such as `~/.csgclaw/projects/demo/todo.json`, not the in-box `/home/node/...` path.

## Single-worker handoff (one task, no tracking)

Use this when exactly one worker should act (for example install a registry skill via `skill-installer`, run one GitLab job) and you do **not** need multi-step `todo.json` sequencing.

1. Use the `basics` skill: `participant list`, confirm room membership, then `message create --mention-id <worker_participant_id>`.
2. Do **not** reply in the room with plain `@worker-name` after registry skill discovery or other tools — that does not wake workers under `mention_only`.
3. Verify with `csgclaw-cli message list` that the dispatch message contains `<at user_id="...">`.
4. Use `start-tracking` only when multiple workers or ordered handoff is required.

## Operating Rules

- Reuse available workers before creating new ones.
- If a matching worker is listed as `unavailable`, recreate that existing worker; do not bare-create a different replacement worker.
- Before writing the final `todo.json` or running `start-tracking`, use the `basics` skill to verify all required workers are already members of the target room.
- Treat missing room membership as a blocker: add the worker, verify again, and only then continue with `todo.json` and tracking.
- Keep `todo.json` aligned with the actual assignment being dispatched.
- Do not casually reorder tasks in the sequential flow.
- Let `start-tracking` drive dispatch from `todo.json`; do not duplicate that logic in manual room-message procedures.
- Never "补发" assignment messages manually after `start-tracking`; treat missing mention/`mention_id` dispatch as a verification failure to debug, not a prompt for manual dispatch.
- While tracking is active, do not manually tell the next worker to start in prose. The tracker is the only sequencer.
- When a worker finishes, they must reply in the shared room with a normal summary or blocker note; updating `todo.json` alone does not release the next task.
- Route all non-tracking room, participant, and member operations through the `basics` skill; do not use `scripts/manager_worker_api.py` for those operations.
- If `start-tracking` or `stop-tracking` response shape differs from expectations, patch the script instead of improvising around it.
