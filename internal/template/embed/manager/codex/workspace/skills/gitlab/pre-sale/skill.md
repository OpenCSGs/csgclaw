# Subskill：售前客户跟进（`product/pre-sale/customers`）

本目录统一覆盖售前侧两类能力：**只读周报统计**（Open issue + 时间窗内 note）与 **初次回访后录入线索并新建跟进 Issue**。默认项目均为 `product/pre-sale/customers`（与
`https://git-devops.opencsg.com/product/pre-sale/customers/-/issues` 对应；实际 host 以凭证 `gitlab_url` 为准）。

- `SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"`
- 脚本目录：`$SKILLS_BASE/gitlab/pre-sale/scripts/`（每条命令块内自行 `cd`）。
- **认证**：通过 Connector lease 执行业务；遇到 401/403 时停止重试，并按父 skill「GitLab 认证」提示用户检查 Connector。

**勿误入**：用户在 `product/agentichub/requirements` 提 bug/需求时说「创建 issue」→ 父级 `gitlab/SKILL.md` 路由到 **`issue-tracker`**，**不是**本目录。

## 功能分区与路由（强制）

| 用户意图 | 使用脚本 | GitLab 副作用 |
|----------|----------|---------------|
| 周报、统计、note、CRM 摘录、无跟进标记（**无**「新建/录入」意图） | `weekly_presale_report.py` | **只读** |
| 录入线索、新建**客户** issue、初次回访建档（须带**售前/customers**语境） | `create_presale_customer_issue.py` | **创建 Issue** |

路由优先级与父级 `gitlab/SKILL.md` 中「售前歧义」一致：若同时命中录入类关键词，**先走新建 issue**，再走周报。

---

## A. 周报（GitLab → CRM 摘录，只读）

面向该仓库中 **Open** 的客户跟进 issue：按固定自然周窗口统计是否有**新增 note**，并标出 **本周窗口内没有任何新增 note** 的客户行，便于汇总到 CRM。

### A.1 语义约定（必须）

| 字段 | 约定 |
|------|------|
| 客户 | 使用该 issue 的 **title** 作为客户标识（一条 issue 对应一个客户跟进入口） |
| 归属 | 归属只按 **issue 标题（客户名）** 统计，不依赖 assignee |
| Open | 仅 `state=opened` 的 issue |
| 周报时间窗 | **Asia/Shanghai**：**上周二 00:00** 起，至 **本周二 00:00** 止（实现为左闭右开 `[上周二00:00, 本周二00:00)`） |
| 报告锚点 | 以「运行时刻」或 `--report-at` 为锚点，取 **不晚于该时刻的最近一个周二 00:00（本地）** 作为 `end`，再向前推 7 天为 `start` |
| 本周无跟进 | 在时间窗内，**没有任何**一条非 `system` 的新增 note |
| 更新时间 | 表格中的“更新时间”取该 issue **全量最新评论时间**（非 system，**不受统计窗口限制**） |

### A.2 执行纪律（只读）

- **只读** GitLab；不向 issue 写评论、不改状态。
- 禁止在回复中打印 `access_token` 或含 token 的 URL。
- 统计与「是否有更新」以 **脚本 stdout** 为准，禁止未跑脚本就宣称结果。
- 面向用户展示客户列表时：**一张 Markdown 表**；需要完整 note 正文时再附加 JSON 或折叠块。

### A.3 推荐命令

默认输出 `both`（统计概览 + Markdown 明细表）。

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/pre-sale" && python3 scripts/weekly_presale_report.py \
  --project-path "product/pre-sale/customers" \
  --format both
```

指定锚点（可选）：

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/pre-sale" && python3 scripts/weekly_presale_report.py \
  --project-path "product/pre-sale/customers" \
  --timezone "Asia/Shanghai" \
  --report-at "2026-04-21T09:30:00+08:00" \
  --format both
```

### A.4 给智能体的回执模板（周报）

```markdown
## 售前周报（GitLab → CRM 摘录）
- project_path: product/pre-sale/customers
- window: {start} .. {end}（Asia/Shanghai，左闭右开）
- open_issues_count: N
- no_tracking_in_window_count: M
- execute: 已执行 weekly_presale_report.py
- verify: 以脚本打印的 JSON/markdown 为准
```

### A.5 输出格式（强制）

为避免模型自由发挥，面向用户输出时必须遵守以下规则：

1. **先执行脚本，再输出**；禁止未执行就口头汇总。
2. 输出仅允许以下两部分，且顺序固定：`统计概览`（固定字段）→ `客户跟进表`（单张 Markdown 表）。
3. **禁止**输出脚本结果之外的推断性内容，如「CRM 建议」「重点关注」「占比解读」「下一步行动建议」等。
4. **禁止**改写统计窗口、数量口径、客户名单；只能使用脚本 stdout 中的实际字段值。
5. 若需要补充说明，仅允许一句「数据来源：weekly_presale_report.py 脚本输出」。

**统计概览（固定模板）**

```markdown
## 售前周报（GitLab → CRM 摘录）
- 统计周期: <payload.period.display>（例如：2026年4月14日-2026年4月20日）
- Open Issue总数: <payload.open_issues_count>
- 周期内未跟进客户数: <payload.no_tracking_in_window_count>
```

**客户跟进表（固定表头）**

表头必须与脚本 markdown 输出一致，不得增删改字段：

```markdown
| 客户名称 | 跟进状态 | 更新时间 | Issue IID | 周期内跟进评论数 |
|---|---|---|---:|---:|
```

排序规则（强制）：先展示 `已跟进`，再展示 `本周未跟进`；同组内按「周期内跟进评论数」从高到低，相同则按「客户名称」升序。

字段映射（强制）：`客户名称`←`customer_title`；`跟进状态`←`has_tracking_in_window`（true→已跟进，false→本周未跟进）；`更新时间`←`latest_note_time`（空为 `-`）；`Issue IID`←`[iid](web_url)`；`周期内跟进评论数`←`tracking_note_count_in_window`。

**禁止示例（不得出现）**：「本周已跟进占比 xx%」；「CRM 摘录建议/重点关注/后续动作」；未在脚本输出中的二次分类统计（除非用户明确要求）。

### A.6 失败恢复

- **401/403** / `缺少 GitLab 凭证`：停止重试并使用父 skill 的缺失凭证模板。
- **404 project**：核对 `--project-path` 与 GitLab 实例是否一致。
- **耗时**：issue 多时会逐条拉 notes。

---

## B. 初次回访 — 新建客户跟进 Issue（写）

初次电话回访后，将结构化线索写入同一项目，**新建 Open issue**；**标题**默认 **公司名称**（空则用联系人）；**描述**为固定 Markdown 模板。

### B.1 语义约定

| 项 | 约定 |
|----|------|
| Issue **标题** | 默认 **公司名称**；未填则 **联系人姓名**；或 `--issue-title` / `--title-from contact` |
| Issue **描述** | 模板列出下列字段，空项为「—」 |
| 项目路径 | 默认 `product/pre-sale/customers` |

描述字段（顺序固定）：公司名称、联系人姓名、联系电话、职位、是否为决策者、需求描述、关注的产品、是否有预算（及金额）、项目周期、潜在用户数量、线索来源、下一步计划、CRM线索链接。

### B.2 凭证与安全（写操作）

- 创建前核对信息；禁止在回复中打印 token。
- 必须真实执行脚本后再宣称成功；以 stdout JSON 为据。

### B.3 推荐命令

```bash
SKILLS_BASE="${SKILLS_BASE:-$CODEX_HOME/skills}"
cd "$SKILLS_BASE/gitlab/pre-sale" && python3 scripts/create_presale_customer_issue.py \
  --company-name "示例科技有限公司" \
  --contact-name "张三" \
  --phone "13800138000" \
  --job-title "技术总监" \
  --is-decision-maker "是" \
  --requirements "希望私有化部署模型推理服务" \
  --products-interest "CSGHub / 推理网关" \
  --budget "有，约 50 万/年" \
  --project-duration "计划 Q3 上线" \
  --potential-users "约 200 名研发" \
  --lead-source "官网留资" \
  --next-step "约下周三演示 POC" \
  --crm-link "https://crm.example.com/leads/12345"
```

标题使用联系人姓名：

```bash
cd "$SKILLS_BASE/gitlab/pre-sale" && python3 scripts/create_presale_customer_issue.py \
  --title-from contact \
  --company-name "示例科技" --contact-name "李四" --phone "13900001111"
```

stdin JSON（键可用英文字段名或中文标签）：

```bash
cd "$SKILLS_BASE/gitlab/pre-sale" && echo '{
  "公司名称": "某某公司",
  "联系人姓名": "王五",
  "联系电话": "13700000000",
  "CRM线索链接": "https://..."
}' | python3 scripts/create_presale_customer_issue.py --stdin-json
```

### B.4 给智能体的回执模板（新建）

面向用户时 **仅**需简短说明已成功创建，并给出 **一条可点击的 Markdown 超链接**（链接目标取脚本 stdout JSON 中的 `web_url`）。**不要**单独列出 `iid`、`web_url` 字段，**不要**写「验证」类说明。

示例（链接文案可微调，须保留完整 URL）：

```markdown
客户信息已录入 GitLab 售前客户跟进系统，跟进 Issue 已创建成功。

[在 GitLab 中打开客户跟进 Issue](<payload.web_url>)
```

（模型内部仍应以脚本 stdout JSON 核对成功；对用户展示以上口径即可。）
