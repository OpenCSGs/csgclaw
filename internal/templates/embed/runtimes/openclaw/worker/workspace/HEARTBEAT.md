# HEARTBEAT.md - Lightweight Check

Use this only for scheduled or lightweight status checks.

- Confirm the request and current runtime context.
- Report blockers plainly.
- Do not mutate files or send outbound messages unless the task explicitly
  requires it.
