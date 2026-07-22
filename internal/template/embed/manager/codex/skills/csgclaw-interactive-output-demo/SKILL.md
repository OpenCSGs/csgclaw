---
name: csgclaw-interactive-output-demo
description: Run the complete CSGClaw structured-output acceptance demo when the user explicitly invokes $csgclaw-interactive-output-demo. Exercise resource links, five Codex-style questions, freeform and secret inputs, and automatic continuation.
---

# CSGClaw Interactive Output Demo

Use this built-in Manager skill as the executable reference for the CSGClaw structured skill output protocol.

On the initial explicit invocation, execute this command exactly once:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py"
```

Do not quote, summarize, or restate the emitted control records.
If the command prints `Structured output unavailable:`, the running CSGClaw runtime did not advertise structured-output protocol version 1.
Report that runtime incompatibility clearly and do not claim that the interactive demo is ready.
After the command succeeds, return this exact Markdown, which is also stored as `INITIAL_RESPONSE_MARKDOWN` in `emit_demo.py`:

```markdown
## Interactive output demo

The complete interactive output demo is ready.
```

If the current prompt says that the user answered a prior `request_user_input` and provides the exact response JSON, do not run the emitter again.
Continue automatically by returning the redacted structured response JSON and then appending its suggested human-facing Markdown presentation, using the shape demonstrated by `render_answer_markdown()` and `ANSWER_RESPONSE_MARKDOWN_EXAMPLE` in `emit_demo.py`.
Preserve every question ID and every non-secret answer exactly.
Replace every answer for `test_secret` with the literal string `<redacted>` before returning it.
Return only these two sections: the `## Submitted \`RequestUserInputResponse\`` heading with its fenced `json` block, followed by the `## Suggested Markdown presentation` heading with its concise answer list.
In the suggested Markdown list, preserve the original question order, remove one leading `user_note: ` prefix from every answer string before display, render empty arrays as `Skipped`, and render a non-empty `test_secret` answer as `Secret recorded`.
For example, display `user_note: Human-readable destination` as `Human-readable destination`, including when the question also offered options.
Never repeat a secret answer value in Markdown, prose, logs, or tool input.
