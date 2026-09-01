# Agent 模板清单字段

本文档记录 CSGClaw Agent 模板中 `agent.toml` 的字段含义、必填要求，以及后续是否计划保留。

知识库 MCP 随 Agent 发布和从社区模板恢复的过程，见 [AgenticHub 知识库 MCP 流程](knowledge-base-mcp.zh.md)。

> 注意：表格记录字段精简决策。标记为 ❌ 的字段已经从当前实现中移除；旧清单中残留的这些字段会被忽略。
>
> `[runtime_options]` 当前仅适用于 Codex Worker 模板，支持 `execution_mode` 和 `memory_mode`。

| 字段名 | 含义 | 是否必须 | 是否计划保留 |
|---|---|---|---|
| `schema_version` | `agent.toml` 的格式版本。目前发布模板时会写入 `agentfile/v1`，但读取时尚未校验 | 否 | ✅ |
| `name` | 模板名称，用于模板展示；创建 Agent 时可作为默认名称 | 是 | ✅ |
| `description` | 模板说明；创建 Agent 时可作为 Agent 的默认描述 | 否 | ✅ |
| `role` | 模板角色，旧格式只支持 `manager` 或 `worker` | 否，已移除 | ❌ |
| `runtime_kind` | Agent 运行时类型，只支持 `codex`、`picoclaw` 或 `openclaw` | 是 | ✅ |
| `version` | 模板版本，用于模板展示及判断沙箱镜像是否需要升级 | 否 | ✅ |
| `updated_at` | 模板更新时间，必须是 RFC 3339 时间格式，用于 Hub 展示和排序 | 否 | ✅ |
| `image.ref` | 沙箱运行时使用的容器镜像地址 | `picoclaw`、`openclaw` 必须；`codex` 不必须 | ✅ |
| `image.digest` | 容器镜像摘要 | 否，已移除 | ❌ |
| `image.platforms` | 镜像支持的平台列表 | 否，已移除 | ❌ |
| `image.env` | 容器镜像所需的环境变量定义列表 | 否 | ✅ |
| `image.env[].name` | 环境变量名称；同一模板内不允许出现大小写不同但名称相同的重复项 | 存在 `image.env` 条目时必须 | ✅ |
| `image.env[].required` | 是否要求用户在创建 Agent 时填写该环境变量，默认值为 `false` | 否 | ✅ |
| `image.env[].secret` | 是否为敏感信息。设置为 `true` 时不允许同时设置 `default` | 否 | ✅ |
| `image.env[].default` | 环境变量默认值；创建 Agent 时自动填入。敏感变量不能设置默认值 | 否 | ✅ |
| `image.env[].description` | 环境变量说明 | 否，已移除 | ❌ |
| `image.env[].choices` | 环境变量可选值列表 | 否，已移除 | ❌ |
| `image.env[].pattern` | 环境变量值的正则表达式约束 | 否，已移除 | ❌ |
| `image.env[].example` | 环境变量示例值 | 否，已移除 | ❌ |
| `image.env[].placeholder` | 环境变量输入提示 | 否，已移除 | ❌ |
| `runtime_options` | Agent 运行时选项。目前只支持 Codex Worker 模板，并且只允许包含 `execution_mode` 和 `memory_mode` | 否 | ✅ |
| `runtime_options.execution_mode` | Codex Worker 的执行模式；`standard` 表示标准模式，`read_only` 表示只读模式 | 否，未设置时按 `standard` 模式运行 | ✅ |
| `runtime_options.memory_mode` | Codex Worker 的记忆模式；`enabled` 表示启用，`disabled` 表示不启用 | 否，未设置时按 `enabled` 模式运行 | ✅ |

## 当前约束

- 内置模板的角色由内置模板注册表决定；本地模板和社区模板统一作为 Worker。
- 从 Manager 保存或发布的模板仍允许创建，但保存后会作为普通 Worker 模板使用。
- `runtime_kind` 当前只接受 `codex`、`picoclaw` 或 `openclaw`。
- `image.ref` 对沙箱运行时 `picoclaw` 和 `openclaw` 是必填字段。
- `runtime_options` 当前仅适用于 Codex Worker 模板。
- `runtime_options.execution_mode` 当前只接受 `standard` 或 `read_only`。
- `runtime_options.memory_mode` 当前只接受 `enabled` 或 `disabled`。
- `image.env[].secret = true` 时不能同时设置非空的 `default`。
