# Stage 3: choose the final action

Use this reference only when the response JSON present at the beginning of the current turn contains `verification`, `destination`, `freeform_note`, and `presentation`.

Choose one destination argument from the `destination` answer:

- `Current room` becomes `current-room`.
- `Thread: QA / 验收` becomes `qa-thread`.
- A `user_note: ` value becomes `custom`.
- A skipped or unrecognized value becomes `unspecified`.

Choose one verification argument from the `verification` answer:

- `Standard` becomes `standard`.
- `Strict + Unicode 中文` becomes `strict`.
- `Fast, focused` becomes `fast`.
- A skipped or unrecognized value becomes `unspecified`.

Choose one presentation argument from the `presentation` answer:

- `Concise (Recommended)` becomes `concise`.
- `Detailed` becomes `detailed`.
- `Bilingual 中文 + English` becomes `bilingual`.
- A skipped or unrecognized value becomes `unspecified`.

Preserve the allowlisted workflow chosen during stage 2.
Execute exactly one command using only allowlisted branch selectors:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py" confirm --workflow <bug-fix|new-feature|code-review|custom> --destination <current-room|qa-thread|custom|unspecified> --verification <standard|strict|fast|unspecified> --presentation <concise|detailed|bilingual|unspecified>
```

After the command succeeds, return this exact Markdown and end the turn:

```markdown
## Interactive output demo - step 3 of 3

Choose the final action and optionally enter a disposable secret test value.
```

Do not read `complete.md` in this turn.
