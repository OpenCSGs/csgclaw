# Subskill：拆解模式 + 逐个实现

其中 `SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"`。

**认证**：通过 Connector lease 执行业务；遇到 401/403 时停止重试，并按父 skill「GitLab 认证」提示用户检查 Connector。

## 拆解（P1→P4）+ 校验（P5）

### P1：读取 Story Issue + `project_id`

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/get_issue.py \
    --project-path "{project_path}" --issue-iid {issue_iid} --with-notes
cd "$SKILLS_BASE/gitlab" && python3 scripts/get_project.py "{project_path}"
```

### P2：LLM 拆解（无脚本）

根据 P1 的 Issue 正文与 notes，将需求拆成可独立交付的小任务；每个任务足够小，适合一次 **D1→D5** 迭代。产出 JSON 数组，元素形如：

```json
{
  "title": "[子任务] 简短标题",
  "description": "做什么、涉及模块/文件、验收标准",
  "labels": ["backend"]
}
```

标题建议前缀 `[子任务]`；任务按依赖排序（无依赖在前）。

### P3：用户确认拆解方案（须执行）

向用户展示完整任务列表（标题+描述）。用户可：批准 → 进入 P4；要求修改 → 调整列表后重新确认；取消 → 不创建子 Issue。
若用户明确「仅分析 / 先别创建 / 只给方案」，则停在 P3。

### P4：批量创建子 Issue

**与 issue-tracker「用户口述 + requirements 模板」建单无关**：子 Issue 描述为 P2 任务 JSON + `Parent story: #…` 前缀，由 `breakdown_issue.py` 调用 `create_issue.py`，**不要**走 `get_issue_template.py` 与 LLM 渲染 requirements 模板流程。

用户批准 P3 后：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-breakdown" && python3 scripts/breakdown_issue.py \
    --project-id {project_id} --parent-iid {issue_iid} \
    --tasks '{P2 的 JSON 数组}'
cd "$SKILLS_BASE/gitlab/fullstack-breakdown" && python3 scripts/list_sub_issues.py \
    --project-id {project_id} --parent-iid {issue_iid}
```

**Milestone 规则（强制）**：

- 子 Issue 的 milestone 必须与父 Issue 保持一致（脚本会自动继承父 Issue 的 milestone_id）。
- 禁止 LLM 或人工在拆解任务 JSON 中捏造 milestone 名称/ID。
- 若父 Issue 无 milestone，则子 Issue 也不设置 milestone。

### P5：结果校验（强制）

校验完成条件：

- `created_count > 0`
- `sub_issues_count > 0`
- 父 issue checklist 与子任务数量一致

未满足时不得宣称「拆解完成」。

---

## 与子 Skill `fullstack-dev` 的衔接（逐个实现）

对每个子任务执行 **开发模式** 时：

1. **必须先 `read_file`** `$SKILLS_BASE/gitlab/fullstack-dev/skill.md`，遵守其中 **D2/D3/D4/D5**（含 D4 确认、`finalize.py` 授权规则）。本子文档只约定 **D1 分支策略** 与 **D5 文案** 在父 Story 场景下的差异。
2. 开发流中的 `issue_iid` 使用 **当前子 Issue** 的 `iid`（不是父 Story）。
3. **D1：读子 Issue + 建分支**（`setup_dev.py` 在父目录 `gitlab/scripts/`，与 `fullstack-dev` 一致）

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/get_issue.py \
    --project-path "{project_path}" --issue-iid {sub_issue.iid} --with-notes
```

**无依赖**（首个子任务，或判断不依赖上一子任务）：`previous_branch` 为空或不用作 base。

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/setup_dev.py \
    "{project_path}" --branch "issue-{sub_issue.iid}-{kebab-description}"
```

**有依赖**（子 Issue 描述提及前序能力、或与前序子任务强相关同一批文件）：在上一子任务完成后，`previous_branch` 为其 **最终开发分支名**（与 `setup_dev` 返回的 `branch` / `active_branch` 一致），D1 必须：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab" && python3 scripts/setup_dev.py \
    "{project_path}" --branch "issue-{sub_issue.iid}-{kebab-description}" \
    --base-branch "{previous_branch}"
```

依赖判定：阅读子 Issue 的 `description`；若明确依赖前序子任务或修改重叠，则视为有依赖。

4. **D5 `finalize.py`**：建议 `--issue-iid` 为子 Issue；`--mr-description` 含 `Closes #{sub_issue.iid}` 与 `Parent story: #{parent_iid}`（与旧版 gitlab-fullstack 一致），**除上述行外，MR 描述正文须为英文**（与 `fullstack-dev/skill.md` 一致）。`--commit-message` / `--mr-title` 可带 `(#{sub_issue.iid})` 便于追溯，标题建议英文。命令路径见 `fullstack-dev/skill.md`（`fullstack-dev/scripts/finalize.py`）。

5. 单个子任务的 **D5 完成后**，若用户还要测 MR，可再 `read_file` `fullstack-test/skill.md`；默认在父 Story 逐个实现流程中 **不自动**进入测试，除非用户明确要求。

---

## 逐个实现（循环）

开始逐个实现前，必须先恢复父 issue 与子任务列表：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-breakdown" && python3 scripts/list_sub_issues.py \
    --project-id {project_id} --parent-iid {parent_iid}
```

规则：

- 若 `sub_issues_count > 0`，沿用该列表，禁止重新拆解。
- 仅当 `sub_issues_count == 0` 才回退 P2 → P4。

维护 `previous_branch`（上一子任务完成后更新）；分支命名：`issue-{iid}-{kebab-description}`。

每个子任务顺序：

1. 通知用户开始：`#子任务 {sub_issue.iid} — {title}`。
2. 按上文 **「与子 Skill fullstack-dev 的衔接」** 执行 D1→D5。
3. 完成后更新父 issue checklist、关闭子任务：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-breakdown" && python3 scripts/track_progress.py \
    --project-id {project_id} \
    --parent-iid {parent_iid} \
    --sub-issue-iid {sub_issue.iid}
```

4. 将 `previous_branch` 设为本子任务 **D1/D5 所使用分支名**（以 `setup_dev` 输出为准），询问是否继续下一子任务。

5. **全部完成后**可输出汇总表（子 Issue / 分支 / MR / 状态）。

失败策略：

- 某子任务失败时先报告错误并询问「重试/跳过」，不要静默进入下一个。
- `track_progress.py` 幂等，可安全重跑。
