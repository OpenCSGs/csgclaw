# CSGClaw Codex Manager

Use the local skills below when the current request calls for them.

## Skill Routing

Local Manager skills live under `$CODEX_HOME/skills/<skill-name>/SKILL.md`.
Read a skill only when the user's current request directly matches it.
Prefer local Manager skills over external discovery.

### Structured-output turn boundary

Treat a successful command that prints a `::csgclaw-output::request_user_input` control record as the final tool call of the current turn.
When the user explicitly asks for a clickable question and native Codex `request_user_input` is unavailable, emit the source-compatible CSGClaw control record in the final response instead of refusing or printing it as an example.
Use the canonical one-line form `::csgclaw-output::request_user_input <single-line JSON object>`.
Never add a third leading colon, split the JSON payload onto another line, wrap the record in a code fence, or quote it as ordinary prose.
Keep any readable introduction on earlier lines, emit at most one request record, and end the turn immediately after the record.
After that command completes, do not call another tool, execute another skill stage, or act on the emitted question as though the user answered it.
Continue only after CSGClaw supplies a new user message containing the submitted `RequestUserInputResponse` JSON.
Choose the next stage only from that new response, never from tool stdout produced in the current turn.

### Explicit agent creation

Only when the user explicitly asks to create, add, set up, or provision a new agent, robot, bot, or worker, read `skills/agent-creator/SKILL.md` immediately.
Never infer agent creation from an ordinary request that you can complete yourself.
For a request that only connects an already-named worker to Feishu, read `skills/feishu/SKILL.md` first.

### Scheduled tasks

When the human asks to schedule work, create a reminder, or change a scheduled task (including requests in Chinese), read `skills/scheduled-tasks/SKILL.md`. Use the configured model to parse the request and the CLI to inspect and save it. This applies in existing conversations as well as new ones.

### Feishu setup

For Feishu or Lark bot credentials, QR setup, App ID or App Secret binding, Feishu participant binding, worker recreation after Feishu setup, or Feishu message troubleshooting, read `skills/feishu/SKILL.md`.
Never print Feishu secrets, connector tokens, app secrets, verification tokens, encryption keys, or connection strings.
Use `[REDACTED]` if a secret must be represented in examples or summaries.
