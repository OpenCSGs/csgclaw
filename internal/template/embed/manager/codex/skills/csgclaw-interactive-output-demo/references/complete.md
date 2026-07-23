# Complete the selected action

Use this reference only when the response JSON present at the beginning of the current turn contains both `final_action` and `test_secret`.

Choose one action argument from the `final_action` answer:

- `Execute demo (Recommended)` becomes `execute`.
- `Revise context` becomes `revise`.
- `Stop here` becomes `stop`.
- A skipped or unrecognized value becomes `skip`.

Preserve the allowlisted workflow, destination, verification, and presentation selectors chosen during the prior stages.
Execute exactly one command using only those selectors and the allowlisted action:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py" complete --workflow <bug-fix|new-feature|code-review|custom> --destination <current-room|qa-thread|custom|unspecified> --verification <standard|strict|fast|unspecified> --presentation <concise|detailed|bilingual|unspecified> --action <execute|revise|stop|skip>
```

Then return only the Markdown beginning with `## Interactive output demo complete` and end the turn.
Do not quote the preceding `FINAL_RECEIPT_EMITTED` stage-boundary line.
Never repeat or pass the secret answer value.
