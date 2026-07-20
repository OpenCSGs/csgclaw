# Subskill：GitLab Issue 追踪与管理（glab）

默认仓库：`REPO="${REPO:-product/agentichub/requirements}"`，组路径：`GROUP="${REPO%/*}"`。
`SKILLS_BASE` 与路径回退见父级 `SKILL.md`「路径约定」。

## 里程碑 / Release 健康状况（强制）

当用户说「**0.6.1 release 健康状况**」「版本健康」「里程碑进度」「还有多少没关」等，含义是 **组级里程碑下的 Issue 完成度**，**不是** GitLab `/releases` 对象，**不是**去猜 `agentichub-server` 等发布仓库。

**禁止**：调用 `glab release list`、`/projects/:id/releases` 回答里程碑健康状况（除非用户明确问 GitLab Release 页面）；在脚本已有结果后继续 `glab api projects?search=...` 盲搜。

**必须**：优先 **一条命令** 完成查询；版本号规则见下文「列表与筛选」。

### 查某版本整体健康状况

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/issue-tracker" && python3 scripts/check_milestone_health.py \
  --milestone "0.6.1" \
  --format markdown
```

解读 `health_verdict`：`closed_clean`（全部关闭且 due date 齐全）｜`closed_with_data_gaps`（全部关闭但有缺 due date）｜`in_progress` / `in_progress_with_data_gaps`（仍有 opened 或数据缺口）。

缺 due date 明细见下文「查询版本里没填 due date 的 issue」；`--all-states` 时用 `list_missing_due_date_issues.py`。

### 查「我的」某版本未关闭 issue

与下文「查询我某版本还没完成的 issue」相同，优先：

```bash
cd "$SKILLS_BASE/gitlab/issue-tracker" && python3 scripts/check_milestone_health.py \
  --milestone "0.6.1" --my-issues --format markdown
```

`opened=0` 即有效结论，**禁止**为验证空结果再去搜 release 仓库（见「失败恢复」）。

## glab 兼容（沙箱 glab 1.90.x）

沙箱 `glab` 可能较旧。遇到 `Unknown flag` 时：**停止**重复同类 flag（如 `-O`、`--json`、`--owned`），**改用本子目录 Python 脚本**（直连 REST API）。
`glab issue list` 可用：`-R`、`--assignee=@me`、`-m "<title>"`、`-F json`、`-P`、`-p`（**不是** `-O`）；**勿用** `--state`（1.90.x 不支持），opened 筛选交给 `list_issues.py` 或 `-m` 组合。

## 本 skill 边界（避免走错分支）

| 场景 | 应读文档 / 脚本 | 勿用 |
|------|-----------------|------|
| 在 **requirements** 等仓库**新建** bug/需求（用户口述问题） | 下文「创建 Issue」+ `get_issue_template.py` + `create_issue.py` | 售前 `pre-sale`；`glab issue create` 直建 |
| **售前** customers 线索录入 / 初次回访 | `pre-sale/skill.md` → `create_presale_customer_issue.py` | 本 skill 的 requirements 模板流程 |
| **Story 拆解**批量建子 Issue | `fullstack-breakdown/skill.md` → `breakdown_issue.py` | 本 skill 用户确认模板流 |
| **开发流**里 finalize 顺带建 Issue | `fullstack-dev/skill.md` → `finalize.py` | 本 skill |
| 列表 / 评论 / 改标签 / due date | 本 skill 其余章节 | `pre-sale` 周报 |

## 脚本速查（新建 vs 查看，勿混用）

| 脚本 | 何时用 | 得到什么 |
|------|--------|----------|
| **`get_issue_template.py`** | **新建** issue **之前（强制第一步）** | 项目默认模板 `issues_template`（Settings → General），供 LLM 渲染 |
| `get_issue.py` | **查看/引用**已有 issue | 该 issue 当前的 `description` 正文 |
| `create_issue.py` | 用户确认草稿 **之后** | 在 GitLab 创建 issue |

**新建 issue 时 LLM 必须意识到**：没有执行 `get_issue_template.py` 就没有合法模板骨架，**不得**凭记忆编造模板，**不得**把用户原话直接交给 `create_issue.py`。

**常见误用（禁止）**：

- **不得**对 `create_issue.py` 使用 `--list-issue-templates`、`--list-templates` 或任何「列模板」参数——**该脚本没有这些参数**（仅支持 `--project-id`、`--title`、`--description`、`--labels`、`--milestone-id`）。拉模板**只能**用 `get_issue_template.py`。
- **不得**把 GitLab 仓库模板 API（`GET .../projects/:id/templates/issues`，列 `.gitlab/issue_templates/*.md`）与项目 **`issues_template`**（General 默认描述）混为一谈；`requirements` 新建 issue 走后者，命令见「创建 Issue」第一步。
- **不得**用 `list_issues.py` 代替拉模板（那是列 **已有 issue**，不是读模板）。

## 仓库范围硬约束（强制）

- 在用户**未明确指定仓库**时，所有查询、统计、评论、更新、关闭/reopen 等操作，均只允许在 `product/agentichub/requirements` 执行。
- 仅当用户在当前请求中明确给出其他仓库（如完整 `group/project`、可解析的仓库 URL）时，才允许切换 `REPO`。
- 未经用户明确指定，禁止跨仓库统计或跨仓库聚合结果。
- 若执行结果出现 `web_url` 不属于当前 `REPO`，必须判定为串库并重试，禁止直接回传给用户。

## 执行纪律（必须）

- **认证**：Python 脚本直接从 CSGClaw connector 获取 lease；直接 `glab` 命令必须通过父 skill 的 `run_with_gitlab_auth.py`。**禁止**手写 `glab auth`、任务前预检查，或引用 `opencsg_credentials.py` / `csghub-cli`。
- 禁止在回复中打印 `GITLAB_TOKEN`、`access_token` 或完整 `export` 行。
- Issue 操作用 `-R "$REPO"`；标签/里程碑枚举用 `GROUP`（`glab label list -g "$GROUP"`、`glab milestone list --group "$GROUP"`）。
- 评论后必须二次验证：`glab issue view <iid> -R "$REPO" --comments`。
- 禁止“口头成功”：未执行命令或未通过验证时，不得输出“已完成/已成功”。
- 每次副作用操作都要给出最少回执字段：`repo`、`iid`、执行命令类型、验证结果。
- 对“只读查询类”任务（list/filter/search、里程碑健康状况），**优先脚本**；`glab` 仅在脚本不满足或写操作时使用；禁止在成功路径上做认证预检查。
- 写操作优先 `glab` wrapper；认证失败时停止重试并提示用户检查 CSGClaw GitLab Connector。
- 对“某版本缺少 due date 的 issue 统计”任务，**必须**优先使用 `list_missing_due_date_issues.py`，禁止让 LLM 对原始列表做二次人工计数。
- **输出收束（强制）**：凡是面向用户的 issue 列表/筛选结果，统一使用**一个 Markdown 表格**展示；禁止按标签、优先级、里程碑或进度再拆成多个表格。
- 单表内可按优先级/进度排序，但必须在同一表中完成；若无结果，返回“0 行结果”说明，不创建空分组表。
- **新建 issue（强制）**：在调用 `create_issue.py` 或 `glab issue create` 之前，**必须先**在同一轮或上一轮执行 `get_issue_template.py` 并取得 `issues_template`（见下文「创建 Issue」）；**禁止**跳过拉模板；**禁止**未读模板就在对话中假装已按模板排版。

## 凭证与认证

仅遵循父 skill `SKILL.md`「GitLab 认证」：Python 脚本自动获取 connector lease，直接 `glab` 使用 `run_with_gitlab_auth.py`，禁止描述或执行 `glab auth login`、禁止引用 `opencsg_credentials.py` / `csghub-cli`。

缺凭证时使用父 skill「Connector 未配置时回复模板」；禁止要求用户在对话中提供或导出 Token。

### glab 写操作（401/403 恢复）

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
export GLAB_TELEMETRY_DISABLED=1
REPO="${REPO:-product/agentichub/requirements}"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue note <iid> -R "$REPO" -m "正文"
```

401/403 时停止重试并提示用户检查 Connector、PAT scope 和项目权限。

## 常用任务

### 列表与筛选

当用户语句包含“我/我的/分配给我”时，查询**必须**包含 `--assignee=@me`（`glab`）或 `--my-issues --scope assigned_to_me`（脚本）。
当用户语句包含“未完成/还没完成/进行中”时，脚本用 `--state opened`；`glab` 1.90.x **勿用** `--state`，改用 `list_issues.py` 或 `check_milestone_health.py --my-issues`。
当用户语句包含版本号（如 `0.5.2` / `v0.5.2`）时，**必须**带 milestone 筛选。

版本号匹配规则（通用，仅此一处）：
- `0.5.2` 与 `v0.5.2` 等价；`list_issues.py` / `check_milestone_health.py` / `list_missing_due_date_issues.py` 会自动归一化匹配组级里程碑真实标题。
- 需要枚举里程碑标题时：`glab milestone list --group "$GROUP" --state active -F json -P 20`。
- **禁止**因前缀差异手工换成多个变体反复查询。

**默认 milestone 规则（强制）**：
- 当用户请求某个 issue 操作（list/filter/update/comment 等）但**未指定 milestone**时，默认使用组级 `active` 里程碑中的**最新一个版本号**作为目标 milestone。
- 获取方式：`glab milestone list --group "$GROUP" --state active -F json -P 20`，按版本号语义选择最新（如 `v0.5.3` > `v0.5.2`）。
- 若 active 里程碑为空或版本号无法判定，必须先向用户确认，不得臆造 milestone。
- 若用户已明确给出 milestone（文本/编号），以用户输入为准，不覆盖。

推荐查询模板（优先脚本，自动读凭证）：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/issue-tracker" && python3 scripts/list_issues.py \
  --my-issues \
  --scope assigned_to_me \
  --state opened \
  --milestone "<milestone_title>"
```

仅当脚本不满足场景时，再使用 `glab issue list`。

```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue list -R "$REPO" --assignee=@me -F json
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue list -R "$REPO" --assignee=@me -l "<label>" -F json
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue list -R "$REPO" --assignee=@me -m "<milestone_title>" -F json
```

输出给用户时每条至少包含：`repo`、`iid`、`title`、`state`、`labels`、`web_url`。

**单表输出模板（默认）**：

```markdown
| repo | iid | title | state | due_date | milestone | labels | web_url |
|---|---:|---|---|---|---|---|---|
| product/agentichub/requirements | 1234 | 示例标题 | opened | 2026-04-20 | v0.5.2 | backend,dev-in-progress | https://... |
```

说明：
- 当用户问“哪些 issue 没填 due date”时，仍按上表输出，`due_date` 为空（或 `null`）的行保留在同一张表里。
- 除用户明确要求“导出 JSON/CSV”外，不要改用多段列表或多表分组（详见执行纪律“输出收束”）。

### 站会清单与分页策略（必须）

- 站会清单默认口径：`--my-issues --scope assigned_to_me --state opened`（脚本），或 `glab … --assignee=@me` 后按 `state` 过滤。
- 已有负责人+筛选条件时，先用 `-P 30 -p 1`；确认被截断再翻页。
- 宽查询（无负责人、无标签）禁止直接大页多翻，先收窄条件再查。
- 多页拉取规则：`-p 2,3...` 直到返回空数组再停止。

示例（脚本优先）：

```bash
cd "$SKILLS_BASE/gitlab/issue-tracker" && python3 scripts/list_issues.py \
  --my-issues --scope assigned_to_me --state opened
```

### 创建 Issue（拉模板 → LLM 渲染 → 用户确认 → 创建，强制）

适用：在 `REPO`（默认 `product/agentichub/requirements`）**新建** bug / 需求 / issue。
**不适用**：售前 customers（`pre-sale`）、Story 子任务（`fullstack-breakdown`）、开发流 `finalize.py` 内建建单——见上文「本 skill 边界」。

**第一步永远是拉模板（强制，不可跳过）**

只要用户意图是**新建** issue，在生成任何「待创建草稿」或调用 `create_issue.py` 之前，**必须先执行**：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
REPO="${REPO:-product/agentichub/requirements}"
python3 "$SKILLS_BASE/gitlab/scripts/get_issue_template.py" "$REPO"
```

- 从 stdout JSON 读取 **`issues_template`**（字符串）与 **`has_template`**（布尔）。
- `has_template=true`：后续 description **必须**以该字符串为骨架，在对话中由 LLM 渲染（见「LLM 渲染规则」）。
- `has_template=false`：与用户说明项目未配置默认模板，再协商是否用简易结构创建。
- **未执行上述命令前**，不得向用户展示「按模板填好的」草稿（禁止凭训练数据臆造 requirements 模板）。

**纪律（必须）**

- **禁止**跳过 `get_issue_template.py` 直接 `create_issue.py` / `glab issue create`。
- **禁止**在未展示草稿、未获用户明确确认前执行 `create_issue.py` 或 `glab issue create`。
- **禁止**将用户原话直接作为 `--description` 提交（会得到纯文本 issue，如 #1986）。
- **禁止**用脚本/代码合并模板（无 `--user-input`、`--draft-only` 等）；**模板渲染只能在对话中由 LLM 完成**。
- **禁止**省略 `description` 指望 GitLab 服务端自动套模板；确认后须把 **LLM 渲染后的完整 Markdown** 传入 `--description`。
- **禁止**口头成功：创建后给出 `iid`、`web_url`，并建议 `glab issue view <iid> -R "$REPO" -F json` 验证。
- 用户修改意见后：在对话中改草稿并**再次确认**，不得在未确认时创建。

**流程（按序）**

| 步骤 | 动作 |
|------|------|
| 1 | 确定 `REPO`（默认 `product/agentichub/requirements`）。 |
| 2 | **（已强制）** `get_issue_template.py` → 读取 `issues_template`。 |
| 3 | **LLM 渲染（强制）**：以步骤 2 的 `issues_template` 为骨架（非 `get_issue.py`），结合用户叙述生成完整 description。 |
| 4 | **用户确认**：展示建议标题 + 渲染后的完整 Markdown。 |
| 5 | **创建**：`create_issue.py --title ... --description "<确认后的正文>"`。 |
| 6 | **回执**：`iid`、`web_url`。 |

**步骤 2 输出字段说明**

| 字段 | 含义 |
|------|------|
| `issues_template` | 项目级默认 Markdown 模板全文（与 UI「Default description template for issues」一致） |
| `has_template` | 是否配置了非空模板 |

**LLM 渲染规则（强制）**

1. **保留结构**：不得删除模板中的 `### …` 章节标题、`---` 分隔、checkbox 列表框架（如环境、影响范围）。
2. **填入内容**：把用户叙述写入合适章节（通常 `### 描述`、`### 实际结果`、`### 操作步骤`）；无信息处写「待补充」或保留占位，**不得捏造**日志、截图、复现细节。
3. **标题**：根据用户叙述归纳一行中文标题，与 description 一并供确认。
4. **自检**：渲染结果须仍包含至少 `### 相关人`、`### 描述`、`### 环境` 等章节（与模板一致）；若 `has_template=false`，与用户说明后按协商结构创建。
5. **唯一写入路径**：用户确认的正文 → 仅通过 `create_issue.py --description` 提交，勿用 `glab issue create` 绕过。

**步骤 5：创建（须在用户确认后）**

```bash
python3 "$SKILLS_BASE/gitlab/scripts/create_issue.py" \
  --project-id "$REPO" \
  --title "<用户确认后的标题>" \
  --description "$(cat <<'EOF'
<LLM 渲染且用户确认后的完整 Markdown>
EOF
)"
```

可选：`--labels`、`--milestone-id`（须真实存在）。

**确认话术示例（给用户）**

```markdown
## 待创建的 Issue 草稿

**标题：** …

**描述：**
…（完整 Markdown）…

请确认：回复「确认创建」将提交到 GitLab；或说明需要修改的字段。
```

**标准回执**（创建成功后，`action` 使用 `issue_create`）：

```markdown
## 操作结果
- action: issue_create
- repo: product/agentichub/requirements
- iid: <iid>
- web_url: <url>
- execute: 已执行 create_issue.py
- verify: 通过 | 失败
```

### 评论（必须验证）

使用稳定路径：`glab issue note` + `glab issue view --comments`。

```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue note <iid> -R "$REPO" -m "正文"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue view <iid> -R "$REPO" --comments
```

通过标准：`--comments` 输出中可见本次评论正文（或可唯一识别的片段）；未看到则判定失败。

### 状态管理（防幻觉）

- 系统状态只有 `opened` / `closed`（`close` / `reopen`）。
- “开发中/测试中/阻塞/待验证”属于业务进度，一律用标签管理。
- 先列组级 labels，再语义匹配目标进度标签，更新后再次 `issue view -F json` 验证。

### 更新 Issue（标题/描述/里程碑/标签）模板

先读取当前 issue：

```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue view <iid> -R "$REPO" -F json
```

常见更新命令：

```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue update <iid> -R "$REPO" --title "新标题"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue update <iid> -R "$REPO" --description "新描述"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue update <iid> -R "$REPO" --due-date 2026-04-20
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue update <iid> -R "$REPO" -l "<TARGET_LABEL>"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue update <iid> -R "$REPO" --unlabel "<OLD_LABEL>"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue close <iid> -R "$REPO"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue reopen <iid> -R "$REPO"
```

标签更新规则：
- 业务进度标签只改“进度类”，不移除无关标签。
- 更新后必须二次验证：`glab issue view <iid> -R "$REPO" -F json`。

### 修改截止日期（due date）

当用户说“修改/设置/调整截止日期、due date、到期时间”时，按以下流程执行：

0. 先查询当前系统日期（必须）：
```bash
date -u +"%Y-%m-%d"
```
禁止模型凭记忆推断“今天/明天/本周五”的具体日期；凡是相对日期，都必须先通过命令拿到“今天”再换算。

1. 先读取当前 issue（确认 `iid`、`repo`）：
```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue view <iid> -R "$REPO" -F json
```
2. 设置截止日期（格式必须是 `YYYY-MM-DD`）：
```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue update <iid> -R "$REPO" --due-date 2026-04-20
```
3. 执行后必须验证：
```bash
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue view <iid> -R "$REPO" -F json
```

验证标准：
- 设置场景：返回 JSON 中 `due_date` 与目标日期一致。
- `web_url` 所属项目路径与 `REPO` 一致（避免串库）。

相对日期处理规则（必须）：
- 用户说“今天”：`TARGET_DATE=当前系统日期`。
- 用户说“明天/后天/3天后”等：基于系统日期计算 `TARGET_DATE` 后再执行 `--due-date`。
- 若运行环境不支持 `date -d`，可改用 `python3 -c` 计算，但仍必须先获取当前系统日期而非凭空假设。

清空截止日期注意事项（必须）：
- 在当前环境中，`glab issue update ... --due-date ""` 可能触发 GitLab `400 at least one parameter must be provided`，不要使用该示例。
- 当用户要求“清空截止日期”时，优先走 GitLab API（`due_date: null`），并在同一轮完成更新与验证；不要只给口头说明。

清空截止日期（API 标准模板，优先 `glab api`；401/403 时停止重试并提示检查 Connector）：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
REPO="${REPO:-product/agentichub/requirements}"
PROJECT_ID="$(python3 -c "import urllib.parse; print(urllib.parse.quote('$REPO', safe=''))")"
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab api "projects/$PROJECT_ID/issues/<iid>" \
  --method PUT \
  --header "Content-Type: application/json" \
  --input <(printf '%s' '{"due_date": null}')
python3 "$SKILLS_BASE/gitlab/scripts/run_with_gitlab_auth.py" glab issue view <iid> -R "$REPO" -F json
```

API 清空验证标准：
- 返回或二次查询结果中 `due_date` 为 `null`（或空值）。
- `web_url` 所属项目路径与 `REPO` 一致（避免串库）。

### “当前版本我的 issue” / “查询我某版本还没完成的 issue”

用户指定版本（如 0.5.2）且问「我的未完成 issue」时，**优先脚本**：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/issue-tracker" && python3 scripts/list_issues.py \
  --my-issues --scope assigned_to_me --state opened --milestone "0.5.2"
```

整体健康状况（含 opened/closed 统计）用 `check_milestone_health.py`（见上文）。
用户只说「当前版本」未给号时：先 `glab milestone list --group "$GROUP" --state active -F json -P 20` 取最新版本标题，再执行上式。
`glab` 备选：`-R "$REPO" --assignee=@me -m "v0.5.2" -F json -P 30 -p 1`（**禁止**省略 `--assignee=@me`）。

### “查询某版本里没填 due date 的 issue”

优先模板（必须）：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/issue-tracker" && python3 scripts/list_missing_due_date_issues.py \
  --milestone "0.5.2" \
  --state opened \
  --format markdown
```

说明：`--milestone` 支持 `0.5.2` / `v0.5.2`；统计以脚本输出字段为准；需含已关闭 issue 时用 `--all-states`。

## 脚本补位

本子目录脚本：`check_milestone_health.py`（版本健康状况）、`list_issues.py`（列表筛选）、`list_missing_due_date_issues.py`（缺 due date 统计）、`time_tracking.py`（工时）。
`--help` 见各脚本；选用场景见上文各节，不在此重复列举。

## 参考补充（内置）

当遇到 REST 细节问题（查询参数、`labels` 行为、分页、notes 接口、URL 编码、`state_event`、`milestone_id` 等）：
- 先以本文件规则执行
- 不足时通过 `glab issue --help` / `glab --help` 与脚本 `--help` 获取当前环境的权威参数

## 失败恢复（必须）

- 认证失败（400/401/403 / connector lease 错误）：停止重试并使用父 skill「Connector 未配置时回复模板」。
- 仓库或 issue 不存在（404）：核对 `REPO` 与 `iid`，并与 issue URL 的项目路径对齐。
- **同类命令连续失败**（相同错误类型 ≥2 次）：停止盲搜；改用 `check_milestone_health.py` / `list_issues.py`，或基于已成功命令的输出回复用户。
- **查询结果为空**：若 `check_milestone_health.py` 或 `list_issues.py` 返回 `opened=0` / `[]`，必须当作有效结论回复，**禁止**为“验证空结果”再去搜 release 仓库或 `projects?search=...`。
- 评论后未命中验证：不要报成功；回传失败并建议重试 `note + view --comments`。
- 更新后验证不一致：重新读取 issue JSON，明确说明哪些字段未生效。
- 多页查询异常：先降到单页 `-P 30 -p 1` 验证，再逐页扩展定位问题页。

## 标准回执模板（必须）

执行完成后按如下结构回复（字段可精简，但语义必须齐全）：

```markdown
## 操作结果
- action: issue_comment | issue_update | issue_close | issue_reopen | issue_create | list_issues | milestone_health
- repo: product/agentichub/requirements
- iid: 1159（列表/健康状况类可省略）
- execute: 已执行（命令级）
- verify: 通过 | 失败
- details: 验证依据（例如 comments 中命中片段）
```

`milestone_health` 额外字段：`milestone_query`、`resolved_milestone_title`、`opened`、`closed`、`missing_due_date`、`health_verdict`。

若是评论动作，`details` 需包含验证依据（例如 `--comments` 输出命中的正文片段）。

如果 `verify=失败`，必须追加：
- 失败原因
- 建议重试命令或下一步修复动作
