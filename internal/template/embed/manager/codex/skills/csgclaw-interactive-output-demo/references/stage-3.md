# Stage 3: choose the final action

Use this reference only when the response JSON present at the beginning of the current turn contains `verification`, `destination`, `freeform_note`, and `presentation`.

Choose one destination argument from the `destination` answer:

- `Current room` or `当前房间` becomes `current-room`.
- `Thread: QA / 验收` or `线程：QA / 验收` becomes `qa-thread`.
- A `user_note: ` value becomes `custom`.
- A skipped or unrecognized value becomes `unspecified`.

Choose one verification argument from the `verification` answer:

- `Standard` or `标准` becomes `standard`.
- `Strict + Unicode 中文` or `严格 + Unicode 中文` becomes `strict`.
- `Fast, focused` or `快速、聚焦` becomes `fast`.
- A skipped or unrecognized value becomes `unspecified`.

Choose one presentation argument from the `presentation` answer:

- `Concise (Recommended)` or `简洁 (Recommended)` becomes `concise`.
- `Detailed` or `详细` becomes `detailed`.
- `Bilingual 中文 + English` or `双语 中文 + English` becomes `bilingual`.
- A skipped or unrecognized value becomes `unspecified`.

Preserve the allowlisted workflow chosen during stage 2.
Keep the initial `en` or `zh` language selection.
Execute exactly one command using only allowlisted branch selectors:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py" confirm --workflow <bug-fix|new-feature|code-review|custom> --destination <current-room|qa-thread|custom|unspecified> --verification <standard|strict|fast|unspecified> --presentation <concise|detailed|bilingual|unspecified> --language <en|zh>
```

After the command succeeds, return the matching exact Markdown and end the turn.

English:

```markdown
## Interactive output demo - step 3 of 3

Choose the final action and optionally enter a disposable secret test value.
```

Chinese:

```markdown
## 交互式输出演示 - 第 3/3 步

请选择最终操作，并可选择输入一次性秘密测试值。
```

Do not read `complete.md` in this turn.
