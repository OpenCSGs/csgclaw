# Subskill：开发模式（Dev Flow）

**前置**：已从用户或会话取得 `project_path`、`issue_iid`（及可选 `branch_description`）。

**Security**：禁止打印 `access_token` 或含 token 的 clone URL；Git 操作经 `load_credentials()` + `git_repo_url`（无 token 嵌入 URL）。

**认证**：通过 Connector lease 执行业务；遇到 401/403 时停止重试，并按父 skill「GitLab 认证」提示用户检查 Connector。

## D1：读取 Issue + 准备仓库

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/get_issue.py \
    --project-path "{project_path}" --issue-iid {issue_iid} --with-notes
```

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/setup_dev.py \
    "{project_path}" --branch "{branch_description}"
```

## D2：实现变更

在克隆目录内按 Issue 与项目惯例改代码；持续在同一 `active_branch` 上提交。

## D3：本地测试

```bash
cd ./gitlab_projects/{project_name} && python -m pytest -v
```

若导入失败可按项目实际安装依赖；自动修复尝试不宜超过 3 次，超限后应向用户说明阻塞点。

## D4：用户确认（必须）

向用户展示：`git diff --stat`、拟定 commit message、MR 标题与描述、**填好参数的** `finalize.py` 整行命令。
**MR 文案语言（强制）**：`--mr-description` 的正文须使用**英文**（可保留 Issue 编号、`Closes #…` 等 GitLab 惯用英文行）；若 Issue 需求为中文，先提炼为英文摘要再写入 MR 描述。`--mr-title` 建议使用英文，便于评审与检索。
用户明确同意前**不得**执行 D5，且只询问一次确认。
`finalize.py` 会执行 commit/push/创建 MR，因此在 D4 未获授权时，禁止以任何形式提前执行这些动作。

## D5：提交并开 MR

用户已在 **D4** 明确同意推送并创建 MR 后，**必须先执行**下方命令（或由用户在终端执行并把 **完整 stdout** 贴回），**再**根据输出写总结。禁止跳过命令、用自然语言直接写「MR 已创建」「任务完成」。

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-dev" && python3 scripts/finalize.py \
    --project-id {project_id} --project-name "{project_name}" \
    --commit-message "{commit_message}" \
    --default-branch "{default_branch}" \
    --issue-iid {issue_iid} \
    --mr-title "{mr_title}" --mr-description "{mr_description}"
```

`--mr-description` 须符合上文「MR 文案语言」：英文描述为主。

**成功回执标准**：向用户展示的内容须与脚本打印的 JSON（或 stderr 中的明确错误）一致；`mr_iid`、`mr_url`、`branch`、`commit` 等**不得凭记忆或推测填写**。若执行失败，说明 `step` / 错误信息并给出下一步，不得宣称成功。

若接下来要走测试模式，把 `mr_iid`、`project_path` 交给 `$SKILLS_BASE/gitlab/fullstack-test/skill.md`。
默认在 D5 结束后先停下并询问用户“是否继续进入测试模式（T1→T6）”，用户同意后再继续。

失败恢复：
- `finalize.py` 为幂等设计，可重试
- 若部分成功（如已 push / 已创建 MR），重跑前先核对远端状态避免重复操作
