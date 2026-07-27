# Agent Profile Skills Management

## 目标

在 Agent Profile 页面的 skills 列表上，支持：

- 新增 skill
- 删除 skill

这里的 skills 必须是 **agent 作用域**，即直接读写该 agent runtime layout 对应的 `SkillsRoot`，而不是当前 Hub 页面使用的全局 `~/.csgclaw/skills`。

其中操作模型需要明确区分：

- 新增：从全局 skills 里选择一个候选 skill，再拷贝到当前 agent 的 runtime `SkillsRoot`
- 删除：直接从当前 agent 的 runtime `SkillsRoot` 删除该 skill 目录

## 当前问题

现状里 Agent Profile 的 skills 仅支持只读展示：

- 前端通过 `fetchAgentSkills()` / `fetchAgentSkillsFile()` 读取 `GET /api/v1/agents/{id}/skills`
- 后端通过 `Handler.handleAgentSkills()` 读取 `svc.SkillsRoot(agent.Name)`

但新增/删除能力目前只存在于全局 skillhub：

- `POST /api/v1/skills:upload`
- `DELETE /api/v1/skills/{name}`
- 后端固定使用 `skillhub.SkillsRoot()`

因此如果直接在 Agent Profile 复用这套写接口，会有两个问题：

- 新增会变成“往全局目录上传/安装新 skill”，而不是“从全局候选复制到 agent 目录”
- 删除会误删全局 skills，而不是删除当前 agent runtime 下的 skills

## 后端修复方案

### 0. 分层边界

这块能力按下面的边界落：

- `api` 层负责暴露 agent skills 的新增/删除接口
- `agent.Service` 负责实现“从全局 skills 拷贝到某个 agent runtime”以及“从某个 agent runtime 删除 skill”的编排
- `runtime` 层通过 `Layout(agentHome)` 提供该 runtime 的目录布局，尤其是 `SkillsRoot`

`agent.Service` 建议补充类似能力：

- `AddSkill(agentID, skillName string) error`
- `DeleteSkill(agentID, skillName string) error`

由它内部完成：

- 解析全局 skills root
- 解析目标 agent runtime 的 `SkillsRoot`
- 执行目录校验、复制和删除

### 1. 为 agent skills 增加独立写接口

建议新增：

- `POST /api/v1/agents/{id}/skills:batchAdd`
- `DELETE /api/v1/agents/{id}/skills/{name}`

保留现有只读接口：

- `GET /api/v1/agents/{id}/skills`
- `GET /api/v1/agents/{id}/skills/file`

### 2. 新增走“从全局 root 复制到 agent root”，删除走“从 agent root 删除”

当前全局 skillhub 已有两类可复用能力：

- 读全局候选：`skillhub.SkillsRoot()` + `skillhub.List(root)`
- `skillhub.Delete(root, name)`

后端需要补一段“目录级复制”能力：

- source root: `skillhub.SkillsRoot()`
- target root: `h.agentSkillsRoot(id)`
- source skill dir: `<global root>/<skill name>`
- target skill dir: `<agent skills root>/<skill name>`

建议做法：

- 新增一个 agent skill batch add handler，请求体使用 `names`
- handler 调用 `agent.Service.AddSkill(...)`
- `POST /api/v1/agents/{id}/skills:batchAdd` 的 body 使用 `{ "names": ["<global-skill-name>"] }`
- 逐个校验 `names` 里的 skill 在全局 skills 中存在，且包含合法 `SKILL.md`
- 若 agent runtime 下已存在同名 skill，返回冲突
- 将全局 skill 目录完整拷贝到 agent runtime 的 `SkillsRoot`
- 删除时调用 `agent.Service.DeleteSkill(...)`

### 3. 错误处理保持与全局 skills 一致

新增建议使用接近现有 skillhub 的错误语义：

- 全局候选不存在返回 `404 Not Found`
- agent runtime 下已存在同名 skill 返回 `409 Conflict`
- 全局 skill 目录不合法 / 缺少 `SKILL.md` 返回 `400 Bad Request`
- agent 不存在返回 `404 Not Found`

删除建议继续复用：

- skill 不存在返回 `404 Not Found`
- 非法 skill 名返回 `400 Bad Request`

### 4. 测试补齐

建议新增/补齐：

- `internal/api/handler_test.go`
  - agent skill add 成功
  - agent skill add 时全局候选不存在
  - agent skill add 重名失败
  - agent skill delete 成功
  - agent 不存在返回 404
- 覆盖至少一种非默认 runtime layout
  - 重点确认 codex agent 会写到 `.codex/home/skills`
  - 而不是误写到 workspace 或全局 `~/.csgclaw/skills`
- 额外确认 copy 是从全局 skill 内容完整复制，而不是只复制 `SKILL.md`

## 前端修复方案

### 1. 在 agent API client 增加写接口

建议在 `web/app/src/api/agents.ts` 增加：

- `batchAddAgentSkillsRequest(agentID, skillNames)`
- `deleteAgentSkillRequest(agentID, skillName)`

不要直接在 Agent Profile 调用 `web/app/src/api/skills.ts` 的全局接口，否则会写错目录。

### 2. 在 `useAgentController` 中接管上传/删除状态

当前 agent skills 数据已经在 `useAgentController.ts` 里通过：

- `workspaceQueryKeys.agentSkills(agentID)`

进行加载。新增/删除也放在这里统一管理。

建议增加：

- `agentSkillAddBusy`
- `agentSkillAddError`
- `agentSkillDeleteBusy`
- `agentSkillDeleteError`
- `batchAddAgentSkills(skillNames)`
- `deleteAgentSkill(skill)`

成功后至少要：

- `invalidateQueries({ queryKey: workspaceQueryKeys.agentSkills(agentID) })`

如果新增动作需要弹出候选列表，controller 里还应额外加载全局 skills 作为候选源，建议直接复用：

- `fetchSkills()`
- `workspaceQueryKeys.skills()`

如果页面后续会缓存某个 skill 文件内容，再额外清理对应 `agentSkillsFile` 查询。

### 3. Agent Detail Pane 增加新增/删除交互

当前 `AgentDetailPane.tsx` 已经有 skills 区块，只差操作入口。

建议：

- 在 skills section 标题右侧增加 `Add` 按钮
- 每个 skill item 增加 `Delete` 按钮
- 删除走确认弹窗，不用浏览器原生 `confirm`
- 点击 `Add` 后弹出候选选择，而不是 zip 上传

候选列表规则建议为：

- 数据源是全局 `/api/v1/skills`
- 默认隐藏当前 agent 已经拥有的 skill
- 展示 `name` + `description`
- 支持多选后调用 `batchAddAgentSkillsRequest(agentID, skillNames)`

### 4. 文案与状态

建议新增 agent profile 专用文案，而不是直接复用 Hub 文案：

- `agentSkillAdd`
- `agentSkillAddSubtitle`
- `agentSkillAddFailed`
- `agentSkillAddEmpty`
- `agentDeleteSkill`
- `agentDeleteSkillConfirmMessage`
- `agentSkillDeleteFailed`

其中 subtitle 要明确：

- 候选来自全局 skills
- 选择后会复制到当前 agent 的 runtime skills 目录

## 推荐改动文件

后端：

- `internal/api/router.go`
- `internal/api/agent_workspace.go` 或新增独立 `internal/api/agent_skills.go`
- `internal/agent/` 下新增或补充 agent skills 管理逻辑
- `internal/api/handler_test.go`

前端：

- `web/app/src/api/agents.ts`
- `web/app/src/hooks/workspace/useAgentController.ts`
- `web/app/src/pages/AgentPage/components/AgentDetailPane/AgentDetailPane.tsx`
- `web/app/src/pages/AgentPage/components/AgentDetailPane/AgentDetailPane.css`
- `web/app/src/shared/i18n/messages.ts`
- 相关前端测试文件

## 实施顺序

1. 先补后端 agent-scoped upload/delete API 与测试。
2. 再补前端 API client 和 `useAgentController` mutation。
3. 最后在 Agent Profile skills 区块接入候选选择弹窗、删除确认和查询刷新。

## 关键注意点

- Agent skills 的真实存储位置必须以 runtime `Layout(...).SkillsRoot` 为准。
- Agent Profile 的新增不是上传 zip，而是从全局 skills 复制到 agent runtime skills。
- 不能把 Agent Profile 的新增/删除误接到全局 `/api/v1/skills` 写接口。
- 不需要把 skills 写入 agent profile/state；它们本质上是文件系统内容，不是 profile 字段。
