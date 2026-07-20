---
name: gitlab
description: "GitLab 全流程增强 skill，源自 gitlab-csgclaw：(A) Issue→MR 开发、MR 测试与报告、Dev+Test 完整链路、Story 拆解与逐个实现、无 MR 的部署侧/API 测试；(B) 管理 Issue、里程碑和工时，在 requirements 新建 Issue 时强制读取模板、渲染、确认后创建；(C) 售前 customers 周报和客户线索建档。Use when the Manager needs to fix or develop a GitLab issue, review or test an MR, create or manage issues, break down stories, inspect milestones, run GitLab API tests, or handle GitLab pre-sale workflows."
---

# GitLab Fullstack Pro（薄入口 + 子文档）

本 Skill **对外仍是一个包**；详细步骤在各子目录的 `skill.md` 中。模型必须先 **`read_file`** 对应子文档，再执行其中的 `bash` / `glab` 指令。

## 路径约定

- 统一变量（Codex Manager 默认）：`SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"`
- 说明：`<subskill>` 是占位符，指本 skill 包内的子目录名（例如 `issue-tracker` / `fullstack-dev` / `fullstack-test` / `fullstack-api-test` / `fullstack-breakdown` / `pre-sale`）。
- 本子文档：`$SKILLS_BASE/gitlab/<subskill>/skill.md`
- 子 skill 本地脚本：`cd "$SKILLS_BASE/gitlab/<subskill>" && python3 scripts/...`
- 各子目录 `scripts/` 存放可独立运行的源码，不依赖外部 skill 目录。
- 父级公共脚本：`cd "$SKILLS_BASE/gitlab" && python3 scripts/...`（凭据 wrapper **`run_with_gitlab_auth.py`**；业务如 `get_issue.py`、`setup_dev.py`、`create_issue.py` 等）。
- 克隆后的业务代码：`cd ./gitlab_projects/{project_name}/...`

Each `execute` call has **no** shared shell state：每条命令块内自行 `cd`。

---

## 子 Skill 索引（必读其一再干活）

| 子文档 | 适用场景 |
|--------|----------|
| `fullstack-dev/skill.md` | 从 Issue 开发到 MR（D1→D5） |
| `fullstack-test/skill.md` | 已有 MR：拉取、双分支、审查、报告、评论（T1→T6） |
| `fullstack-full/skill.md` | 开发完成后立刻测 MR（D1→D5 再 T1→T6） |
| `fullstack-breakdown/skill.md` | Story 拆解子 Issue、 checklist、逐个实现与 `track_progress` |
| `fullstack-api-test/skill.md` | 无 MR 的「全量/冒烟/API 测试」、AgenticHub-test.yaml、api_report |
| `issue-tracker/skill.md` | glab / 脚本管理 Issue：列表、筛选、评论、标签、里程碑、工时、**里程碑健康状况**；**新建** issue 见「创建 Issue」 |
| `pre-sale/skill.md` | 售前 `customers` 仓库：周报统计（只读）+ 线索录入新建 issue（写）；见文档内 A/B 分区 |

---

## 模式识别（先判定，再 read_file）

| 用户意图 | 先读子文档 |
|----------|------------|
| 修复 issue、实现功能、帮我开发（代码+MR） | `fullstack-dev/skill.md` |
| 测试 MR、出测试报告、验证 MR 修复 | `fullstack-test/skill.md` |
| 修复并测试、从 issue 到测 MR 全流程 | `fullstack-full/skill.md` |
| 拆解 issue、分解任务、拆解需求；逐个实现子任务；继续下一个子任务 | `fullstack-breakdown/skill.md` |
| 冒烟/契约/场景/API 测试、P0；**全量/全部/完整测试且无 MR 上下文** | `fullstack-api-test/skill.md` |
| 列出/查看/评论/更新 Issue；站会；我的 issue；里程碑版本；改「开发中」等标签；工时 | `issue-tracker/skill.md` |
| **版本/release 健康状况**、里程碑进度、某版本还有多少未关闭 issue | `issue-tracker/skill.md` → **「里程碑 / Release 健康状况」** |
| **创建** Issue / 提 bug / 新建需求 / 登记问题（`requirements` 等，**非**售前 customers） | `issue-tracker/skill.md` → **「创建 Issue」**；`get_issue_template.py` + **LLM 渲染** + 用户确认 + `create_issue.py` |
| 售前（周报 / 统计 / CRM / note / customers）或（录入线索 / 回访建档 / 新建客户 issue） | `pre-sale/skill.md`（文档内 **A=周报只读**、**B=新建 issue**） |
| Story **拆解**后批量建子 Issue | `fullstack-breakdown/skill.md` → `breakdown_issue.py`（**非** issue-tracker 模板流） |

### 「售前」与「创建 issue」歧义优先级（强制）

- **走 pre-sale（B）**：同时满足 **(1) 售前语境**（售前 / customers 仓库 / 线索 / 回访 / CRM 客户录入）与 **(2) 录入或新建意图**（录入 / 新建 / 建档 / 初次回访 / 线索录入）。此时用 `create_presale_customer_issue.py`，**不要**用 `issue-tracker` 的 requirements 模板流。
- **走 pre-sale（A）**：售前语境 + 周报/统计/跟进/note，且**无**录入/新建意图 → `weekly_presale_report.py`（只读）。
- **走 issue-tracker 创建**：在 `product/agentichub/requirements`（或用户指定的非 customers 项目）提 bug、记需求、优化项等，即使用户说了「创建 issue」，**只要无售前/customers 语境**，一律 `issue-tracker/skill.md`，**禁止**误入 `pre-sale`。
- **走 fullstack-breakdown**：用户要**拆解 Story / 分子任务 / 批量子 Issue** → `fullstack-breakdown/skill.md`，**禁止**用 issue-tracker 单条用户口述建单流程。
- 若用户意图包含 **售前** 且包含任一关键词：`录入` / `新建` / `建档` / `创建 issue` / `初次回访` / `线索录入`，**必须优先**读 `pre-sale/skill.md` 并走 **B**（`create_presale_customer_issue.py`，写操作）。
- 若用户意图包含 **售前** 且包含任一关键词：`跟进` / `统计` / `周报` / `CRM` / `note` / `customers`（且**无**上一条的新建/录入意图），**必须优先**读 `pre-sale/skill.md` 并走 **A**（`weekly_presale_report.py`，只读）。
- 即使用户句子中出现了「issue 列表/查看/open issue」，只要满足上一条周报统计条件，也**不得**路由到 `issue-tracker/skill.md`。
- 仅当用户目标是对 issue 做运维操作（如评论、改标签、改里程碑、关闭/reopen、工时）且不涉及售前子 skill 的 A/B 场景时，才走 `issue-tracker/skill.md`。

## 执行与回执（强制）

- 任何涉及外部副作用的动作（评论/更新/关闭/reopen/创建等）必须先执行真实命令，再返回结果。
- **创建 Issue 例外（强制）**：**第一步必须** `get_issue_template.py` 读 `issues_template`（禁止跳过、禁止臆造模板）→ LLM 在对话中渲染 → 用户确认 → `create_issue.py --description`。**禁止**用户原话直传 `--description`。细则见 `issue-tracker/skill.md`「脚本速查」「创建 Issue」。售前 B、拆解子任务除外。
- 禁止“未调用命令就宣称成功”。若命令未执行或失败，必须明确说明失败原因与下一步。
- 对于评论和状态更新类动作，必须执行“动作命令 + 验证命令”两步，并依据验证结果回复。
- 回复中必须包含可核对字段：`REPO`、`iid`、核心命令是否执行、验证是否通过（通过/失败）。
- 对于 issue 评论动作，必须包含来自验证命令的证据（如 `glab issue view --comments` 命中片段）；无验证证据时视为未完成。
- 若仅给了计划或推理，必须显式标注“尚未执行”。
- 在开发流中，`finalize.py`（commit/push/MR）属于高风险副作用：未获用户明确授权不得执行。
- **创建 MR 的文案**：`--mr-description` 使用**英文**；Issue 为中文时须在 D4/D5 中改写为英文说明（保留 `Closes` 等英文惯用行）。MR 标题建议英文。
- **拆解子任务的 milestone**：创建子 Issue 时必须继承父 Issue 的 milestone；禁止模型捏造不存在的 milestone。
- **D5 防幻觉（强制）**：用户已同意执行 D4 中展示的 `finalize.py` 后，**下一轮必须先真实执行该命令**（或由用户在本机执行并粘贴输出）。**禁止**在未调用工具/未拿到终端输出的情况下，叙述「已成功推送」「MR 已创建」或编造 `mr_iid`、MR URL、`commit` 哈希、分支名。若当前环境无法执行命令，须明确写「尚未执行 finalize」，并给出用户可复制的一行命令，不得假装已完成。
- 宣称 D5 成功时，回复中须含 **可核对证据**：来自 `finalize.py` **stdout 的 JSON**（或等价的 `mr_iid`、`mr_url`、`branch`、`commit` 字段），或用户提供的同结构输出；无则只能写「待执行」或「执行失败」。
- 在完整流中，D5 完成后必须再次询问是否进入测试流（T1→T6）；禁止自动衔接到测试。

### GitLab 认证（强制）

**唯一凭据来源**：CSGClaw Manager GitLab connector lease。公共 Python 脚本会通过 `CSGCLAW_BASE_URL` 和 `CSGCLAW_ACCESS_TOKEN` 自动获取 lease，全程只在进程内使用。

#### 常态

- Python 业务脚本可直接运行，它们会动态获取 lease。
- 所有直接 `glab` 命令必须通过 `scripts/run_with_gitlab_auth.py glab ...` 运行，以便向单次子进程注入 lease。
- 禁止执行 `glab auth login`、读取 glab `config.yml`、把 token 写入环境配置或凭据文件。
- 禁止在任务开始前做认证预检查。

#### 认证失败时

业务命令出现 **400/401/403**、`Could not authenticate`、`未登录` 或 connector lease 错误时，停止重试并告诉用户在 CSGClaw 中检查 GitLab Connector 的 Base URL、Token、PAT scope 和项目权限。禁止要求用户在对话中发送 Token。

#### Connector 未配置时回复模板

```markdown
当前 GitLab Connector 未配置、Token 已失效或权限不足，无法继续执行。

请在 CSGClaw GitLab Connector 中检查 Base URL 和 Personal Access Token，然后让我重试。请不要在对话中发送 Token。PAT 创建步骤见：[GitLab Token 配置指南](https://opencsg.com/docs/agentichub/101/quickstart/gitlab-token)。
```

### 「全量测试」歧义（强制）

- **无 MR 上下文** → **只**走 `fullstack-api-test/skill.md`（A 流），**禁止**安装项目依赖、**禁止** `detect.py`。
- **有 MR 且处于测试流** → 在 `fullstack-test/skill.md` 的审查里覆盖全量变更即可。

### 上下文提取（两套件共用）

1. 会话中已有 `project_id`、`project_path`、`mr_iid`、`issue_iid` 等则复用。
2. MR URL → 解析 `project_path`、`mr_iid`。
3. Issue URL/编号 → 解析 `project_path`、`issue_iid`。
4. 不足则向用户追问。

---

## 组合场景

- **先追踪 Issue 再开发**：可先读 `issue-tracker/skill.md` 定位/更新 Issue，再读 `fullstack-dev/skill.md`。
- **拆解后开发**：入口为 `fullstack-breakdown/skill.md`；**进入「逐个实现」循环后，每个子任务开发前必须再 `read_file` `fullstack-dev/skill.md`**，并遵守 breakdown 文档中对 D1（含 `--base-branch`）的附加规则（见该子文档「与子 Skill fullstack-dev 的衔接」）。
- **依赖缺失**：优先以 `gitlab` 目录内文档与脚本为准，不读取外部 skill 文档。

---

## 多子 Skill 编排（流转摘要）

下列场景会涉及 **多个** `skill.md`，除父级本表外，以各子文档为准。

| 场景 | 阅读顺序（均须 `read_file`） | 关键衔接数据 |
|------|------------------------------|--------------|
| 完整链路 Dev→Test | `fullstack-full/skill.md` → 按需重读 `fullstack-dev`、`fullstack-test` | D5 的 `mr_iid`、`project_path`、`project_id` → T1；D5 后须用户确认再进入测试（见 `fullstack-full`） |
| 拆解 + 逐个实现 | `fullstack-breakdown/skill.md` → **每子任务** `fullstack-dev/skill.md` | 子任务的 `issue_iid` 即子 Issue 的 `iid`；`project_path`/`project_id`/`project_name`/`default_branch` 全链路复用；**有依赖时** D1 的 `setup_dev.py` 必须带 `--base-branch`（见 breakdown 子文档） |
| 仅开发单 Issue | `fullstack-dev/skill.md` | 无 |
| 仅测 MR | `fullstack-test/skill.md` | `project_path`、`mr_iid` |
| 无 MR 的部署/API 测试 | `fullstack-api-test/skill.md`（缺 yaml 时在同文件内走 **AG1→AG3**，再汇总进报告） | `project_name` 来自 `setup_dev`；`AgenticHub-test.yaml` 或 AG 产物 → `api_report.py` |
| Issue 管理后开发 | `issue-tracker/skill.md` → `fullstack-dev/skill.md` | 从列表/视图取得 `project_path`、`issue_iid` |

**纪律**：子文档之间 **没有** 共享 shell；跨阶段只传递 JSON/变量，每条命令块内自行 `cd`。

---

## 错误恢复（摘要）

Fullstack 各脚本多可安全重试；详情见脚本返回结果中的 `step` 字段。Issue 侧：评论/更新后必须用 wrapper 执行 `glab issue view … --comments` 或 `-F json` 验证后再对用户报成功。GitLab **认证失败 / 缺凭证**：停止重试并使用「Connector 未配置时回复模板」，不得空泛甩锅。
