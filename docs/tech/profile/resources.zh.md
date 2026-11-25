# Profile 的 Skill 与 MCP 启用管理

Profile 的 Skill 与 MCP 使用资源页面的 `ResourceListCard`：宽屏两列，每项横向排列图标、名称、说明与操作按钮；窄屏使用一列。点击内容打开详情弹框，点击启用或禁用按钮提交配置。Skill 详情按需读取完整目录和文件，MCP 详情隐藏凭据值。来源更新、删除确认和 App 设置沿用现有功能。

## 添加资源

- Skills 支持选择全局技能、上传 ZIP、搜索市场并安装启用，弹窗提供全局管理入口。
- Connectors 分别提供 App 和 MCP 添加入口。App 使用现有全局资源绑定流程；MCP 支持全局资源、手动配置和市场安装。
- Knowledge bases 使用独立页签，支持启用已连接的知识库，以及连接并启用有权限的 AgenticHub 知识库。知识库继续使用现有 MCP 配置和来源同步流程。

`AgentResourcePicker` 与 `useAgentResourcePicker` 共用资源查询、安装和绑定流程，复用 `SkillUploadDialog`、`MCPServerCreateDialog` 及现有 API。全局安装成功后，将服务端返回的资源名称交给 Agent controller 添加；绑定失败时，弹窗保留安装结果，重试继续添加资源。MCP 添加请求通过 `enabled: true` 在同一次 Agent 更新中启用资源，全局资源状态保持原值。

## 数据与模块

- `AgentSpec.SkillStates`、Agent 持久记录的 `skill_states` 保存每个已安装 Skill 的状态。未配置状态时默认启用。删除 Skill 同时移除状态，禁用保留文件。
- `mcp_servers.<name>.enabled` 保存 MCP 状态，缺省为启用。来源同步保留 Agent 的启用选择。
- HTTP handler 负责请求验证与响应；所有修改经过 `Engine.Agents().Update` 和现有 Agent 生命周期控制，使用 `ResourceVersion` 检查并发修改。
- Agent controller 负责保存目标配置、协调 runtime 配置写入与重启；runtime adapter 负责各自配置格式。`SkillsReconciler` 仅表示配置写入能力。
- `useAgentResourceEnablement` 统一请求状态、失败重试与查询失效。Profile 和聊天 `/` 候选使用同一个 `agentSkills` 查询；候选过滤禁用的 Skill。
- App 管理的 MCP 使用现有 App enabled API 与 App runtime 刷新机制；详情弹框显示工具列表并提供 App 设置入口。

## 接口

| 方法与 URI | 用途 |
| --- | --- |
| `GET /api/v1/agents/{id}/skill-summaries` | 返回 Skill 摘要数组，每项包含 `name`、`description`、`enabled`；响应头提供 `ETag` |
| `GET /api/v1/agents/{id}/mcp-servers` | 保持现有响应格式，配置包含 `enabled`；响应头提供 `ETag` |
| `PUT /api/v1/agents/{id}/skills/{name}/enabled` | 设置已安装 Skill 的状态 |
| `PUT /api/v1/agents/{id}/mcp-servers/{name}/enabled` | 设置已安装 MCP 的状态 |

两个 PUT 接口共用请求格式。`enabled` 必须是 boolean；`resource_version` 必须是查询响应 `ETag` 的字符串值，去除 HTTP 引号。

```json
{"enabled":false,"resource_version":"当前版本"}
```

成功响应：

```json
{
  "agent_id":"agent-example",
  "resource_version":"更新后的版本",
  "name":"reviewer",
  "enabled":false,
  "runtime_kind":"codex",
  "runtime_state":"running",
  "restart_required":false
}
```

`runtime_kind` 支持 `codex`、`dsh`。停止中的 Agent 写入配置并保持停止，返回 `runtime_state: "stopped"` 与 `restart_required: true`，下次启动读取配置。

| HTTP 状态 | 错误代码 | 含义 |
| --- | --- | --- |
| 400 | `invalid_request` | 参数缺失、类型错误或无效 Skill 名称 |
| 404 | `skill_not_found` / `mcp_server_not_found` | 目标资源未安装 |
| 409 | `resource_version_conflict` | Agent 已被其他请求修改 |
| 422 | `runtime_capability_unsupported` | runtime 不支持此操作 |
| 500 | `runtime_config_apply_failed` | 配置写入或重启失败 |

## 配置流程

```mermaid
sequenceDiagram
    participant UI as Profile
    participant API as HTTP handler
    participant Engine as Agent controller
    participant Runtime as Codex / DSH
    UI->>API: 查询列表，获取 ETag
    UI->>API: PUT enabled + resource_version
    API->>Engine: Update 指定字段
    Engine->>Engine: 获取生命周期控制权，校验版本并保存配置
    alt Agent 正在运行
        Engine->>Runtime: 写入配置，停止、初始化、启动
        Engine->>Engine: 清除 EnvRestartRequired
    else Agent 已停止
        Engine->>Runtime: 写入配置
    end
    Engine-->>API: 更新后的 Agent
    API-->>UI: 状态与新版本
    UI->>API: 刷新列表及 Agent 状态
```

配置修改沿用持久化的 `EnvRestartRequired`。配置写入或启动失败时保留目标状态与重试标记，页面展示错误和重试按钮；重试提交相同目标状态，继续应用配置。一次请求内的 Skill 与 MCP 修改统一由 controller 安排重启，保留 Agent 目录与会话记录。

## Worker 自助安装（Codex 和 DSH）

安装已有技能先读 `skill-installer`；从零编写技能读 `skill-creator`。来源未指定（包括裸 `namespace/name`）时，`skill_search` 依次查当前 worker 已安装资源（含禁用）、全局库、当前登录的 OpenCSG 站点、skills.sh，首次命中即停止。对 `installed` 候选直接用 `key` 和 `selection` 调用启用工具，不下载。命名空间按安装来源匹配，不将同名或另一个仓库当作命中。网络错误或未登录不会被当成“无结果”而自动转向公网。`CSGCLAW_DISABLE_PUBLIC_SKILLS=true` 禁用 skills.sh 检索与下载。

对选中的全局/站点候选调用 `skill_install_from_catalog`，原样传回 `source`、`key`，全局候选还须传 `selection`，站点候选传 `site`、`ref`。全局来源或技能包发生变化时需重新搜索。它只写本 worker 文件；站点凭据仅在 Server 使用。安装结果返回实际 `name`；读 `skill_status(name)` 可查询来源、已知版本、期望启用状态及磁盘是否同步，不触发修复或会话重载。

同名同源的安装/重装表示用户希望使用该技能，应继续 MCP 启用步骤，即使原先被禁用。同名异源（或无法确认旧来源）必须告知两个来源，询问是否覆盖。用户明确同意后，带 `replace:true` 和冲突返回的 `confirmation` 重试；CLI/脚本使用 `--force --confirmation <token>`。已安装副本变化后旧确认失效。

已有 Codex/DSH worker 普通启动时识别历史发布的完整 `skill-installer`、`skill-creator` 文件树，仅升级未改动的内置副本。用户改写、其他来源、主动删除和原有禁用状态均保留。

在 worker 内执行 `csgclaw-cli --output json skill install <slug> --registry opencsg` 只安装文件；显式重装使用 `--force`。skills.sh/GitHub 由 `skill-installer` 内置脚本在临时项目下载，再用 `skill import` 安装。两条路径都返回实际安装名 `Slug`，不需要 shell 持有 worker 的服务端凭据。

安装或重装到当前 worker 后，Agent 按 `skill-installer/SKILL.md` 调用内置 `csgclaw` MCP 工具 `skill_ensure_enabled({"name":"<Slug>"})`。服务端从已认证的 MCP 连接确定目标 worker；工具不接受 Agent ID 或目录。`disk_synced: true` 表示启用配置与磁盘已同步，不表示当前会话已加载。原先已启用的技能也会同步最新文件，全程不重启 worker。若启用失败，只重试 MCP 调用；普通终端安装或导出到其他目录不会触发 MCP。已有 Web/Portal 管理开关保持原行为。

## Codex

Skill 禁用配置写入 Agent 独立的 `.codex/home/config.toml`，使用带管理标记的 `[[skills.config]]` 段，`path` 指向对应 Skill 的 `SKILL.md` 文件，`enabled = false`。启用时移除该禁用项。配置重新生成时保留管理段。MCP 使用已有 `[mcp_servers.<name>]` 配置及原生 `enabled` 字段。

## DSH

`.dsh/home/skills` 保留完整 Skill 文件。每次应用配置与启动时，在 `.dsh/skill-view/skills` 生成启用项的完整副本，包含脚本、资源和文件权限。目录准备完成后替换当前目录，使用备份处理替换中断。

生成的 `csgclaw-context.patch.yml` 为 `skill-filesystem` 配置 `dshHome: <agent>/.dsh/skill-view`。DSH 自身的 home 与会话目录保持原位置。MCP 通过 ACP `session/new`、`session/resume` 参数加载，仅发送启用项；禁用项继续保存在 Agent 配置中。

这些开关管理 Profile 中已安装的资源。runtime 自行发现的项目目录资源遵循该 runtime 的发现规则。
