# Task workspace contracts

Task conversations use the existing IM room and message APIs. Starting a task creates a direct room with the selected Agent participant, sends the first message to that room, and selects the same room in the workspace. The routed room owns displayed messages and outgoing drafts.

## Session configuration

`POST /api/v1/rooms` and `PATCH /api/v1/rooms/{id}` accept an optional `configuration` object:

```json
{
  "configuration": {
    "model_provider_id": "opencsg",
    "model_id": "example-model",
    "reasoning_effort": "auto",
    "workspace_dir": "/absolute/project/path",
    "execution_mode": "read_only"
  }
}
```

The server validates catalog references and runtime choices. Credentials are resolved from the current provider catalog and are never persisted in the room. Empty model, reasoning, and execution-mode choices follow Agent defaults. The working directory is captured on creation and cannot change afterward. Starting a new task from a conversation inherits its choices, which can be edited before sending.

Host Codex and DSH support independent session settings. Teams and sandbox runtimes keep their existing execution contract; the toolbar explains the unsupported scope. Configuration changes are rejected while the room is working. Codex uses `standard` / `read_only` and honors the server's read-only policy. DSH uses `read-only` / `workspace-write` / `danger-full-access`.

## Execution and recovery

The Room remains the conversation record. Execution reuses the existing `(Agent ID, ConversationKey)` mapping to Codex thread IDs and DSH session IDs. A group room therefore has a separate native session for each participating Agent; it is not assigned to a single Agent. Native runtime files stay in the existing Agent runtime store. No additional task identity or per-room runtime home is created.

The Agent runtime remains alive between turns. Before execution, the adapter resolves the room's effective settings without changing the saved Agent defaults. Identical settings reuse the process. A changed process configuration (including the room-specific model gateway) restarts that Agent's process and resumes the same native session. Host Codex and DSH turns for the same Agent are queued while a turn is active, so a settings change cannot interrupt another room. Different Agents can execute concurrently. Canceled queued requests never change the running configuration. Read-only Codex execution disables skill discovery through runtime configuration while retaining the Agent's skill files; standard mode restores the saved skill settings.

Codex engine threads register the same host tool declarations in both execution modes, because native `thread/resume` cannot change them. The host checks the current mode on every image-generation, video-generation, and upload call, so read-only execution remains restricted while switching back to standard mode retains the native thread and its complete history.

Native history, including tool calls and compacted context, is retained across these restarts and server restarts. Room message text is never replayed as a substitute for native history. Explicit reset removes only the existing mapping for that conversation. The unpublished `task-sessions` layout is imported into the existing runtime store once, preserving the original files as backups; reset conversations are not imported again.

Configured execution uses `/api/v1/agents/{id}/tasks/{room_id}/llm` for model requests. This existing route uses the Room ID, not a separate task ID. The bridge validates both the Agent credential and room membership, then resolves the room's model profile. Credentials are resolved from the provider configuration rather than copied into the Room. Agent-wide mutation still uses the existing lifecycle coordinator.

## Regression coverage

Tests cover routed message/draft ownership, target selection at creation, directory locking, persisted room configuration, concurrent model routing, unauthorized room access, process reuse, native resume after configuration/server changes, multi-Agent room mappings, canceled queued turns, legacy native-history import, server read-only enforcement, and the toolbar's save/error behavior.

## Natural-language scheduled tasks

The Scheduled tasks page offers **Create manually** and **Use a sentence**. The latter focuses the start-panel composer; Enter inserts a newline and `@` selects an executor, including Manager. Sending creates a new Manager direct task conversation through the existing room/message APIs. Failed sends retain the input and reuse the room/request/message IDs on retry. Manager model and authentication checks run before room creation.

The page uses the demo's full-width header, compact schedule list, centered composer, and three usage hints. List badges show disabled, enabled, or running based on the schedule and its generated tasks, including schedules that are not selected. Narrow workspace panes stack the list above the independently scrolling detail/composer pane.

Manager uses its configured model and embedded `scheduled-tasks` skill to parse requests in new or existing conversations. It reads `csgclaw-cli scheduled-task context` for the current server time, real executor IDs and supported recurrences, shows the parsed time/content/executor in the conversation, and lets the user correct them before saving. Ambiguous times, missing executors and unsupported schedules require clarification. A successful CLI response is required before reporting success. Demo-only keyword parsing is not used.

Both `csgclaw` and `csgclaw-cli` expose `scheduled-task context|list|create|update`. These call the existing `/api/v1/scheduled-tasks` CRUD API; `GET /api/v1/scheduled-tasks/context` returns only executor ID/name/role, current server time and supported recurrences. Model configuration and credentials are excluded. Schedule timestamps and recurrence calculations retain their existing behavior.

Manager runs these commands with its scoped Agent credential. The API explicitly permits Manager's context/list/create/update routes; workers cannot manage schedules, and read-only execution mode permits only context/list. This does not grant configuration administration, schedule deletion, or immediate execution. Regression coverage uses the CLI against the authenticated HTTP router through saving, editing, triggering, and Manager claim/completion.

Successful schedule creation, editing, and deletion publish `scheduled_task.changed` through the existing workspace event stream. The browser revalidates the shared schedule query so both the list and sidebar count update even while a Manager conversation is open. Event-stream reconnection and entering the Scheduled tasks page also revalidate the list to recover missed notifications.

The supported recurrence set remains `once`, `daily`, `weekly`, `monthly`. Unsupported recurrence values are rejected. The Manager workflow asks about workdays, multiple weekdays and end-of-month dates rather than silently changing them. The running server checks due tasks once a minute. Triggering creates the existing tracked Agent task and delivers its assignment event in the executor's direct room. Manager uses Agent ID `agent-manager` and participant ID `pt-manager`, claims the task, executes it and records completion/failure through the same task API.
