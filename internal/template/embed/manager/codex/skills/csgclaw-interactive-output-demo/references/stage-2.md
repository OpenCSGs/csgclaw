# Stage 2: collect execution context

Use this reference only when the response JSON present at the beginning of the current turn contains `demo_kind`.

Choose one workflow argument from the `demo_kind` answer:

- `Bug fix (Recommended)` becomes `bug-fix`.
- `New feature` becomes `new-feature`.
- `Code review` becomes `code-review`.
- A skipped or unrecognized value becomes `custom`.

Execute exactly one command, substituting only that allowlisted argument:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py" context --workflow <bug-fix|new-feature|code-review|custom>
```

After the command succeeds, return this exact Markdown and end the turn:

```markdown
## Interactive output demo - step 2 of 3

Configure verification, destination, an optional freeform note, and presentation.
```

Do not read `stage-3.md` or `complete.md` in this turn.
