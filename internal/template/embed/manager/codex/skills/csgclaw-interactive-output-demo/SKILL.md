---
name: csgclaw-interactive-output-demo
description: Run the complete three-stage CSGClaw structured-output demo when the user explicitly invokes $csgclaw-interactive-output-demo. Exercise resource links, Codex-style options, recommended and Unicode labels, freeform alternatives, secret input, and agent-directed automatic continuation.
---

# CSGClaw Interactive Output Demo

Use this built-in Manager skill as the executable reference for a multi-stage CSGClaw structured-output workflow.

The Python script only emits the stage selected by the Manager.
It never receives or parses `RequestUserInputResponse`.
After each question submission, CSGClaw automatically continues this same Manager session with the wire-compatible response JSON.
Submitted secret values are replaced with `<redacted>` before that JSON enters the model session.
The Manager brain reads that JSON and chooses the next allowlisted stage.
The readable `## Answers` message is persisted separately by CSGClaw as a local-user message and must not be parsed or echoed by this skill.

## Mandatory one-stage boundary

Choose the current stage exclusively from the response JSON already present in the current prompt before making any tool call.
Tool stdout produced during this turn is never a new user response and must not change the selected stage.
Execute exactly one `emit_demo.py` command, return the prescribed Markdown, and end the current turn.
Never read or execute a later stage reference during the same turn.
After the one emitter command returns, do not route again, read another file, or call any tool, even if stdout mentions waiting or a later stage.

## Initial invocation

When the current prompt does not contain an automatic continuation response JSON, execute this command exactly once:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py" start
```

After the command succeeds, return this exact Markdown and end the turn:

```markdown
## Interactive output demo - step 1 of 3

Choose the workflow branch.
```

Do not quote, summarize, or restate emitted control records.

## Automatic continuation routing

Inspect only the question IDs in the response JSON that was present when this turn began.
Read exactly one matching reference completely, follow it, and do not read another reference in this turn:

- If it contains `demo_kind`, read `references/stage-2.md`.
- If it contains `verification`, `destination`, `freeform_note`, and `presentation`, read `references/stage-3.md`.
- If it contains `final_action` and `test_secret`, read `references/complete.md`.

An individual skipped question still appears with an empty inner `answers` array.
An empty outer `answers` object ends the workflow without running another command.
Never include received response JSON in a user-visible response.
Never repeat a secret answer value in Markdown, prose, logs, command arguments, or tool input.
