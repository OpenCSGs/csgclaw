# Subskill：测试模式（Test Flow）

**前置**：`project_path`、`mr_iid`（可从 MR URL 解析或会话继承）。

**认证**：通过 Connector lease 执行业务；遇到 401/403 时停止重试，并按父 skill「GitLab 认证」提示用户检查 Connector。

## T1：获取 MR 信息

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-test" && python3 scripts/fetch_mr.py \
    --project-path "{project_path}" --mr-iid {mr_iid}
```

## T2：双分支对比环境

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-test" && python3 scripts/setup_test.py \
    --project-path "{project_path}" \
    --source-branch "{source_branch}" --target-branch "{target_branch}"
```

## T3：技术栈检测

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-test" && python3 scripts/detect.py \
    --project-dir "./gitlab_projects/{project_name}/_source"
```

## T4：AI 代码审查（无脚本）

结合 T1/T2/T3 结果输出中文结构化审查 JSON（`overall_verdict`: `pass | partial | fail`）。
至少包含：
- `issue_type`
- `issue_summary`
- `review_items`（每个文件的 verdict/comment）
- `overall_verdict`
- `overall_comment`
- `suggestions`（可选）

判定规则：
- `pass`：完整满足需求且无明显风险
- `partial`：部分满足，仍有缺口
- `fail`：未满足需求或存在明显错误

如 diff 信息不足，需读取 `_source` 目录文件补上下文再给结论。

## T5：生成测试报告

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-test" && python3 scripts/report.py \
    --mr-info '{T1 的 fetch_mr JSON}' \
    --strategy '{T3 detect JSON}' \
    --code-review '{T4 审查 JSON}'
```

## T6：发布到 MR 评论（需用户确认）

先展示 `report_content` 给用户确认，再执行发布命令。

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/fullstack-test" && python3 scripts/comment.py \
    --project-id {project_id} --mr-iid {mr_iid} \
    --report-file "{report_path}"
```

失败恢复：
- `comment.py` 重跑会新增 note，属于预期行为
- 重跑前应确认 `project_id` 与 `mr_iid` 未错位
