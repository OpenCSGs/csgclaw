# Subskill：完整模式（Dev → Test）

**顺序**：先执行 `$SKILLS_BASE/gitlab/fullstack-dev/skill.md` 的 D1→D4（等待授权）→ D5，再询问是否继续测试；仅在用户确认后执行 `$SKILLS_BASE/gitlab/fullstack-test/skill.md` 的 T1→T6。其中 `SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"`。

**认证**：各阶段通过 Connector lease 直接执行业务；遇到 401/403 时停止重试，并按父 skill「GitLab 认证」提示用户检查 Connector。

## 衔接数据

- D5 产出 `mr_iid`、`project_path`、`project_id` → 直接作为 T1 输入。
- 无需用户重复提供 MR 编号（除非会话丢失，再追问）。

## 强制确认点（必须）

1. **确认点 A（开发发布前）**：严格遵循 Dev Flow 的 D4。未获用户明确同意，禁止执行 D5（commit/push/MR）。
2. **确认点 B（测试前）**：D5 成功后，必须先询问“是否进入测试流程 T1→T6（生成并可选发布测试报告）”。
   - 用户同意：继续 T1→T6。
   - 用户拒绝或未答复：停在开发结果汇报，不自动进入测试。
