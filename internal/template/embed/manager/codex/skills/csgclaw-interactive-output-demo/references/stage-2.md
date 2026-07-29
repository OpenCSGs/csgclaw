# Stage 2: collect execution context

Use this reference only when the response JSON present at the beginning of the current turn contains `demo_kind`.

Choose one workflow argument from the `demo_kind` answer:

- `Bug fix (Recommended)` or `修复缺陷 (Recommended)` becomes `bug-fix`.
- `New feature` or `新功能` becomes `new-feature`.
- `Code review` or `代码审查` becomes `code-review`.
- A skipped or unrecognized value becomes `custom`.

Keep the initial `en` or `zh` language selection.
Execute exactly one command, substituting only those allowlisted arguments:

```bash
python3 "$CODEX_HOME/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py" context --workflow <bug-fix|new-feature|code-review|custom> --language <en|zh>
```

After the command succeeds, return the matching exact Markdown and end the turn.

English:

```markdown
## Interactive output demo - step 2 of 3

Configure verification, destination, an optional freeform note, and presentation.
```

Chinese:

```markdown
## 交互式输出演示 - 第 2/3 步

请配置验证方式、目标位置、可选备注和展示方式。
```

Do not read `stage-3.md` or `complete.md` in this turn.
