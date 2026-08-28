# CSGClaw API 文档

本文基于当前代码中的实际 HTTP 路由整理，默认服务地址示例为 `http://127.0.0.1:18080`。

## 约定

- 除流式接口外，请求和响应均为 `application/json`
- 时间字段使用 RFC3339 / ISO8601
- 常规错误通常返回纯文本错误正文
- Agent session response 错误使用 OpenAI 风格的 JSON error envelope
- SSE 接口返回 `text/event-stream`
- 当前 API 主要分为 3 组：
  - 核心 API：`/api/v1/*`
  - Channel 与 participant API：`/api/v1/channels/*`
  - 健康检查：`/healthz`

CSGClaw skill 脚本可以通过 Runtime 中立的 [CSGClaw 结构化 Skill 输出协议](structured-output.zh.md) 输出资源链接和交互式问题。
该协议最终通过下方的 channel activity 接口提交问题响应。

## Activity API

### `POST /api/v1/channels/{channel}/activities/{activity_id}:respond`

回答一个待处理的结构化问题 activity。
请求正文是精确的 `RequestUserInputResponse` 对象：

```json
{
  "answers": {
    "verification": {
      "answers": ["Standard (Recommended)"]
    },
    "note": {
      "answers": []
    }
  }
}
```

空的外层 `answers` 对象会跳过整个请求。
空的内层 `answers` 数组会跳过单个问题。
服务端从已存储的 activity 中推导 room 和 responder，并拒绝未知字段。

## 认证

- 默认大多数 `/api/v1/*` 接口不要求认证
- 以下接口要求 `Authorization: Bearer <token>`，其中 token 为服务端 access token
  - `GET /api/v1/channels/{channel}/participants/{id}/events`
  - `POST /api/v1/channels/csgclaw/participants/{id}/messages`
  - `GET /api/v1/agents/{id}/llm/models`
  - `GET /api/v1/agents/{id}/llm/v1/models`
  - `POST /api/v1/agents/{id}/llm/chat/completions`
  - `POST /api/v1/agents/{id}/llm/v1/chat/completions`
  - `POST /api/v1/agents/{id}/llm/responses`
  - `POST /api/v1/agents/{id}/llm/v1/responses`
  - `GET /api/v1/agents/{id}/llm/responses`
  - `GET /api/v1/agents/{id}/llm/v1/responses`
- 若服务端开启 `no_auth`，上述鉴权会被跳过

## 健康检查

### `GET /healthz`

健康检查。

响应示例：

```text
ok
```

## 核心 API

### `GET /api/v1/version`

返回当前服务版本。

响应示例：

```json
{
  "version": "0.1.0"
}
```

### 升级

#### `GET /api/v1/upgrade/status`

返回升级状态。

响应字段：

- `current_version`
- `latest_version`
- `update_available`
- `checking`
- `upgrading`
- `last_checked_at`
- `last_error`

#### `POST /api/v1/upgrade/apply`

触发升级 helper。

成功时返回 `202 Accepted`：

```json
{
  "status": "accepted",
  "message": "upgrade helper started"
}
```

若升级管理器未配置，返回 `503 Service Unavailable`。

## Skill API

### `GET /api/v1/skills/remote`

从有效的 OpenCSG Hub 列出远端 Skill。浏览器只请求 CSGClaw；Server 按当前登录环境或显式配置的 official Hub URL 解析 Hub 地址。

可选查询参数：

- `page`：正整数页码，默认 `1`
- `per`：每页数量，范围 `1` 到 `100`，默认 `16`
- `search`：远端 catalog 搜索文本

Server 保持当前 Hub catalog 的排序，并返回归一化后的 Skill 摘要：

```json
{
  "items": [
    {
      "name": "agent-builder",
      "description": "Build agents",
      "readonly": true,
      "source": "official",
      "remote_path": "AIWizards/agent-builder",
      "remote_ref": "main",
      "remote_url": "https://opencsg.com/skills/AIWizards/agent-builder"
    }
  ],
  "page": 1,
  "per": 16,
  "total": 78,
  "next_page": 2
}
```

`remote_path` 是用于安装的稳定远端 Skill 标识。`remote_url` 是浏览器详情地址：未登录时使用 `https://opencsg.com`，登录后使用当前 OpenCSG 登录站点。Hub API 地址不作为网页地址使用。

### `POST /api/v1/skills:install`

从同一个有效 OpenCSG Hub 安装远端 Skill。Hub 支持时，Server 通过单个请求下载仓库压缩包；旧版 Hub 则兼容回退到 tree/blob API。设置 `replace` 可覆盖同名本地 Skill。

```json
{
  "remote_path": "AIWizards/agent-builder",
  "ref": "main",
  "replace": false
}
```

成功返回 `201 Created` 和已安装的本地 Skill 摘要；未设置 `replace` 时同名冲突返回 `409 Conflict`。

## Participant API

Participant 是 channel-scoped identity，用于房间、消息、mention、通知和 runtime bridge。Participant 可以表示 human、agent-backed channel identity 或 notification sender。

### `GET /api/v1/channels/{channel}/participants`

获取指定 channel 下的 participant 列表。

路径参数：

- `channel`：`csgclaw` 或 `feishu`

可选查询参数：

- `type`：`human`、`agent` 或 `notification`
- `agent_id`

响应字段：

- `id`
- `channel`
- `type`
- `name`
- `avatar`
- `channel_user_ref`
- `channel_user_kind`
- `channel_app_ref`
- `agent_id`
- `lifecycle_status`
- `presence`
- `mentionable`
- `metadata`
- `created_at`
- `updated_at`

示例：

- `GET /api/v1/channels/csgclaw/participants`
- `GET /api/v1/channels/csgclaw/participants?type=notification`
- `GET /api/v1/channels/feishu/participants?agent_id=u-worker`

### `POST /api/v1/channels/{channel}/participants`

在指定 channel 下创建 participant。

路径参数：

- `channel`：`csgclaw` 或 `feishu`

请求体示例：

```json
{
  "id": "qa",
  "type": "agent",
  "name": "QA",
  "channel_user": {
    "ref": "u-qa",
    "kind": "local_user_id"
  },
  "agent_binding": {
    "mode": "create",
    "agent": {
      "name": "QA",
      "role": "worker",
      "runtime_kind": "picoclaw_sandbox",
      "from_template": "builtin.openclaw-worker"
    }
  }
}
```

说明：

- `type` 必填，且只能是 `human`、`agent` 或 `notification`
- `name` 必填
- 实际 channel 由路由路径决定，而不是由请求体决定
- `agent` participant 可通过 `agent_binding` 创建或复用 Agent
- `human` 与 `notification` participant 不创建 runtime agent
- 上面的示例中，`qa` 是 participant ID；`u-qa` 只作为本地 channel user ref 和生成的 backing agent ID。
- 对 `csgclaw` 来说，`channel_user.ref` 是本地 IM user ID
- 对 `feishu` 来说，`channel_user.ref` 是 channel-native open ID

示例：

- `POST /api/v1/channels/csgclaw/participants`
- `POST /api/v1/channels/feishu/participants`

### `GET /api/v1/channels/{channel}/participants/{id}`

获取单个 participant。

### `PATCH /api/v1/channels/{channel}/participants/{id}`

更新 `name`、`avatar`、`mentionable`、`metadata` 等可编辑 participant 字段。

### `DELETE /api/v1/channels/{channel}/participants/{id}`

删除指定 channel 下的 participant。

成功返回 `204 No Content`。

示例：

- `DELETE /api/v1/channels/csgclaw/participants/qa`
- `DELETE /api/v1/channels/feishu/participants/qa`

## Agent API

### Agent 响应结构

`/api/v1/agents*` 返回的 agent 主要字段如下：

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "runtime_id": "codex",
  "runtime_kind": "codex",
  "image": "example/image:latest",
  "box_id": "codex-session-alice",
  "role": "worker",
  "status": "running",
  "created_at": "2026-05-16T08:00:00Z",
  "profile": "api.gpt-5.4",
  "runtime_options": {},
  "mcpServers": {
    "workspace-filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/workspace"]
    }
  },
  "agent_profile": {
    "provider": "api",
    "base_url": "https://api.example.com/v1",
    "api_key_set": true,
    "api_key_preview": "sk-1...",
    "model_id": "gpt-5.4",
    "reasoning_effort": "medium",
    "profile_complete": true
  },
  "profile_complete": true,
  "detection_results": []
}
```

说明：

- `agent_profile` 中不会返回真实 `api_key`
- `runtime_options` 会经过 API 侧脱敏处理
- `mcpServers` 是 server 名称到 server 配置的直接映射；通用 Agent 响应会
  脱敏各 server 的 `env` 与 `headers` 中的密钥值
- `profile` 是服务端归一化后的选择器，例如 `api.gpt-5.4`
- `detection_results` 用于展示默认 profile 探测结果

### `GET /api/v1/agents`

列出全部 agent。

服务端会先执行 reload，再返回最新状态。

### `POST /api/v1/agents`

创建 agent。

请求体字段：

- `id`
- `name`
- `description`
- `image`
- `runtime_kind`
- `from_template`
- `replace`
- `field_mask`
- `role`
- `status`
- `created_at`
- `profile`
- `runtime_options`
- `mcpServers`
- `agent_profile`

`from_template` 应使用模板列表 API 返回的原始 ID。内置和本地模板使用 `builtin.codex-worker`、`local.my-worker` 等格式；远端模板使用 `<namespace>/<name>` 格式。

请求体示例：

```json
{
  "id": "u-alice",
  "name": "alice",
  "description": "frontend dev",
  "runtime_kind": "codex",
  "profile": "api.gpt-5.4",
  "agent_profile": {
    "provider": "api",
    "base_url": "https://api.example.com/v1",
    "api_key": "sk-xxx",
    "model_id": "gpt-5.4",
    "reasoning_effort": "medium"
  }
}
```

补充说明：

- `name` 必填
- `replace=true` 时会走替换逻辑
- `field_mask` 用于替换时只覆盖指定字段
- `agent_profile.api_key` 只在写入时使用，读取时会被脱敏

OpenClaw、PicoClaw 和 Codex CLI agent 通过顶层 `mcpServers` 字段配置 MCP server。该字段的值是所有已支持 runtime 共用的、从 server 名称到 server 配置的直接映射：

```json
{
  "name": "alice",
  "runtime_kind": "openclaw_sandbox",
  "mcpServers": {
    "workspace-filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/home/node/.openclaw/workspace"
      ]
    }
  },
  "profile": "api.gpt-5.4"
}
```

各 runtime adapter 的映射关系：

- OpenClaw：`mcpServers` -> `openclaw.json` 的 `mcp.servers`
- PicoClaw：`mcpServers` -> PicoClaw 配置的 `tools.mcp.servers`
- Codex CLI：`mcpServers` -> 隔离 Codex home `config.toml` 中由 CSGClaw 管理的 `[mcp_servers."<name>"]` 块

注意：MCP command 在目标 runtime 环境内执行，filesystem server 的目录参数也必须是该 runtime 可见路径。`runtime_options` 中的 MCP 形态值（包括 `mcp` 与 `mcpServers`）会被拒绝；请使用顶层 `mcpServers` 字段。

### `GET /api/v1/agents/{id}`

获取单个 agent。

不存在时返回 `404`。

### `PATCH /api/v1/agents/{id}`

更新 agent 基本信息。

可更新字段：

- `name`
- `description`
- `image`
- `runtime_options`
- `mcpServers`
- `agent_profile`

请求体示例：

```json
{
  "description": "updated description",
  "runtime_options": {
    "execution_mode": "read_only"
  }
}
```

说明：

- 省略的字段不会修改
- `runtime_options` 一旦提交就是整体替换
- Codex Worker 支持 `runtime_options.execution_mode`，取值为 `standard` 或 `read_only`；字段省略或为空时默认使用 `standard`
- 正在运行的 Codex Worker 切换运行模式后会自动重启，使新的命令、沙盒和审批策略生效
- `mcpServers` 一旦提交就是整个映射替换；传 `null` 可清除 CSGClaw 托管的 server 集合
- OpenClaw、PicoClaw 或 Codex CLI agent 的 MCP server 变更可能触发该 agent runtime recreate，使原生配置生效
- `agent_profile.api_key` 如果传空，服务端会保留原有密钥
- 如果 `agent_profile.env` 发生变化，响应中的 `env_restart_required` 可能为 `true`

### Agent memory 接口

当选中的 Runtime 实现 CSGClaw memory capability 时，Agent 响应中的 `memory_supported` 为 `true`。
Profile 页面只为这些 Agent 展示 Memory Tab。

#### `GET /api/v1/agents/{id}/memory`

返回 Runtime 的只读主记忆文档和启用状态。
Runtime 尚未生成文档时，`ready` 为 `false`。

```json
{
  "enabled": true,
  "ready": true,
  "name": "memory_summary.md",
  "location": "$CODEX_HOME/memories/memory_summary.md",
  "content": "# Durable memory\n"
}
```

`location` 是 Runtime 提供、用于展示和诊断的逻辑文件位置；Codex 当前返回 `$CODEX_HOME/memories/memory_summary.md`。

#### `PUT /api/v1/agents/{id}/memory`

接收 `{ "enabled": true }` 或 `{ "enabled": false }`，并返回相同的 memory document 结构。
具体配置格式以及应用设置时是否需要重启由 Runtime 自己负责。
该接口只读展示自动生成的 memory 内容，不提供人工编辑能力。

### Agent MCP server 接口

#### `GET /api/v1/agents/{id}/mcp-servers`

返回 Agent 的 MCP server 配置。`servers` 是从 server 名称到 server 配置的直接原始映射。与通用 Agent 响应不同，这个接口不会脱敏已配置的 token。

```json
{
  "agent_id": "u-alice",
  "runtime_kind": "openclaw_sandbox",
  "servers": {
    "context7": {
      "command": "uvx",
      "args": ["context7-mcp"],
      "env": { "CONTEXT7_API_KEY": "secret" }
    }
  }
}
```

#### `POST /api/v1/agents/{id}/mcp-servers:batchAdd`

接收 `{ "names": ["..."] }`，将 MCP catalog 中同名 server 的定义合并到该 Agent 托管的 MCP server。响应与 `GET` 一样。

#### `POST /api/v1/agents/{id}/mcp-servers:batchDelete`

接收 `{ "names": ["..."] }`，从该 Agent 托管的 MCP server 中移除同名 server。响应与 `GET` 一样。

### MCP server catalog

可复用的 MCP server catalog 保持在 `/api/v1/mcp-servers`：

- `GET /api/v1/mcp-servers`：列出 catalog server
- `POST /api/v1/mcp-servers`：创建 catalog server
- `PUT /api/v1/mcp-servers/{name}`：替换一个 catalog server
- `DELETE /api/v1/mcp-servers/{name}`：删除一个 catalog server

catalog 的列表和变更响应以 `mcpServers` 作为直接 server 映射。创建或替换单个
catalog 条目时使用 `{ "name": "...", "config": { ... } }`，其中 `config`
就是该条目的 server 配置。

#### `GET /api/v1/mcp-servers/remote`

通过 CSGClaw 列出可安装的远端 MCP server 摘要。浏览器只请求 CSGClaw；服务端根据
当前登录环境或显式配置的 official Hub 解析有效 OpenCSG Hub，并转发当前用户的
Hub 凭据。查询参数与远端 Skill 列表保持一致：`page`（默认 `1`）、`per`（默认
`12`，最大 `100`）和 `search`。

每个条目只包含 Hub 的 `id`、名称和展示元数据，不包含安装配置或 headers。
`description` 是 CSGClaw 用于说明 MCP 用途的 catalog 元数据；向 Agent runtime
添加条目时会移除该字段。

```json
{
  "items": [
    {
      "id": "builtin:calendar",
      "name": "calendar",
      "description": "Calendar tools",
      "url": "https://mcp.example.test/calendar",
      "protocol": "streamable-http"
    }
  ],
  "page": 1,
  "per": 12,
  "total": 13,
  "next_page": 2
}
```

#### `POST /api/v1/mcp-servers/remote/{id}/install`

安装或刷新一个远端 MCP 条目。CSGClaw 会在服务端请求 Hub 详情接口，将详情中的
`protocol`、`url` 和 `headers` 转换为 catalog 配置，并使用详情响应的 `name` 作为
catalog key。新远端条目默认写入 `enabled: true`、`startup_timeout_sec: 30` 和
`tool_timeout_sec: 60`。响应只返回已安装的名称：

```json
{ "name": "calendar" }
```

### `DELETE /api/v1/agents/{id}`

删除 agent。

成功返回 `204 No Content`。

### `POST /api/v1/agents/{id}/start`

启动 agent，返回更新后的 agent 对象。

### `POST /api/v1/agents/{id}/stop`

停止 agent，返回更新后的 agent 对象。

### `GET /api/v1/agents/{id}/logs`

获取 agent 日志。

查询参数：

- `lines`：默认 `20`
- `follow`：`1/true/yes/on` 表示持续跟随输出

返回类型：`text/plain; charset=utf-8`

说明：

- `follow=false` 时，错误会直接以 HTTP 错误返回
- `follow=true` 时，若流式过程中出错，错误文本会被写入响应体

### `GET /api/v1/agents/{id}/profile`

获取单个 agent 的脱敏 profile。

### `PUT /api/v1/agents/{id}/profile`

整体更新单个 agent 的 profile。

请求体为 `agent_profile` 结构，例如：

```json
{
  "provider": "api",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx",
  "model_id": "gpt-5.4",
  "reasoning_effort": "medium",
  "headers": {
    "x-org": "demo"
  },
  "env": {
    "FOO": "bar"
  }
}
```

说明：

- 与 `PATCH /api/v1/agents/{id}` 不同，这里语义上是“用新的 profile 覆盖当前 profile”
- 若 `api_key` 为空，服务端会保留现有密钥

### `POST /api/v1/agents/{id}/recreate`

按当前配置重建 agent，返回新的 agent 状态。

常见失败：

- `404`：agent 不存在
- `400`：profile 不完整或运行时不允许重建

## Agent Profile 辅助 API

### `POST /api/v1/agent-profiles/models`

根据给定 provider 配置获取可选模型列表。

请求体字段：

- `agent_id`
- `provider`
- `base_url`
- `api_key`
- `headers`

请求体示例：

```json
{
  "provider": "api",
  "base_url": "https://api.example.com/v1",
  "api_key": "sk-xxx"
}
```

响应示例：

```json
{
  "provider": "api",
  "models": ["gpt-5.4", "gpt-5.4-mini"]
}
```

说明：

- `provider=codex` 或 `claude_code` 时会通过 CLIProxy 获取模型选项
- `provider=api` 时会调用目标 OpenAI-compatible `/models`
- 若提供了 `agent_id` 且当前请求未显式传 `api_key`，服务端可能复用该 agent 已保存的密钥
- 若未提供 `agent_id` 且当前请求未显式传 `api_key`，仅当 `provider=api` 且 `base_url` 匹配当前默认 profile 时，服务端才可能复用已保存的默认 API key

### `GET /api/v1/agent-profile-defaults`

获取服务当前默认 agent profile 的脱敏视图。

常用于前端初始化默认 provider / model 展示。

## Hub Template API

模板使用以下目录结构：

```text
<template>/
  agent.toml
  instructions/AGENTS.md
  skills/<skill>/...
  mcps/mcp.json
  memories/memory_summary.md # 可选的 Codex 记忆快照
  memories/MEMORY.md         # 非 Codex Runtime 可选
```

发布时始终生成 `AGENTS.md` 和 `mcp.json`。
其他 instruction 文件可选。
当 `memory_mode` 启用时，Codex 模板会快照 `CODEX_HOME/memories/memory_summary.md`，并在创建新 Agent 时恢复到相同位置。`memory_mode = "disabled"` 的模板不会打包或恢复记忆摘要；模板也不会复制 Codex 的 SQLite memory 流水线状态，或把 memory 当作 workspace overlay。
非 Codex Runtime 的可选模板 memories 仍按对应 Runtime 的 workspace 约定叠加。
根据模板创建 Agent 时，skills 会安装到 `skills/`，`mcp.json` 中的 MCP server 会自动应用；如果创建请求显式传入 `mcpServers`，则以请求内容为准。

Codex Worker 模板可在 `agent.toml` 中保存运行模式：

```toml
[runtime_options]
execution_mode = "read_only"
memory_mode = "disabled"
```

发布时只保存适合模板复用的运行时选项。
Codex Worker 目前保存 `execution_mode` 和 `memory_mode`，不会保存 `local_workspace_dir` 等本机选项。
创建请求显式提供的运行时选项优先于模板值。
缺少 `execution_mode` 时默认为 `standard`。
缺少 `memory_mode` 时默认为 `enabled`。

### `GET /api/v1/hub/templates`

列出可读 registry 中的全部模板。

响应字段：

- `id`
- `name`
- `description`
- `runtime_kind`
- `image`
- `runtime_options`
- `updated_at`
- `source.name`
- `source.kind`
- `workspace.kind`

### `POST /api/v1/hub/templates`

将现有 agent 的 workspace 发布到 hub。

请求体：

```json
{
  "agent_id": "u-alice",
  "registry": "local",
  "include_memory": false
}
```

说明：

- `agent_id` 必填
- `registry` 省略时使用默认 publish registry
- 只有显式设置 `include_memory: true` 才会发布 Codex 记忆摘要，包括通过 `template_id` 再发布已有本地模板时；由于记忆可能包含从对话中提取的隐私信息，该字段默认是 `false`
- 发布成功返回 `201 Created`

### `GET /api/v1/hub/templates/{id}`

获取模板详情。

在列表接口的基础上，还会返回：

- `workspace.entries`

`workspace.entries` 字段示例：

```json
{
  "workspace": {
    "kind": "dir",
    "entries": [
      {"path":"agent.toml","name":"agent.toml","type":"file","depth":0,"size":128},
      {"path":"instructions","name":"instructions","type":"dir","depth":0,"size":0}
    ]
  }
}
```

### `GET /api/v1/hub/templates/{id}/workspace/file?path=...`

读取模板 workspace 中的单个文件预览。

查询参数：

- `path`：必填，相对路径

响应字段：

- `path`
- `content`
- `size`
- `truncated`
- `binary`

说明：

- 非 UTF-8 文件会返回 `binary=true`
- 超过 `256 KiB` 的文本内容会被截断，并返回 `truncated=true`
- 不允许绝对路径或 `..` 越界路径

## CLIProxy Auth API

### `GET /api/v1/cliproxy/auth/status?provider=...`

查询 provider 的本地鉴权状态。

`provider` 必填。

响应内容由 CLIProxy 返回，通常包含：

- `provider`
- `authenticated`
- `login_required`
- `message`
- `supports_login`

### `POST /api/v1/cliproxy/auth/login`

触发 provider 登录。

请求体：

```json
{
  "provider": "codex",
  "no_browser": true
}
```

成功返回 provider 当前鉴权状态。

说明：

- 缺少 `provider` 返回 `400`
- 登录失败返回 `502 Bad Gateway`

## Bootstrap Config API

### `GET /api/v1/config/bootstrap`

获取 bootstrap 配置视图。

响应字段：

- `default_manager_template`
- `default_worker_template`
- `runtime_kind`
- `effective_manager_image`
- `supported_runtime_kinds`
- `runtime_default_images`

### `PUT /api/v1/config/bootstrap`

更新 bootstrap 默认模板。

请求体：

```json
{
  "default_manager_template": "builtin.manager",
  "default_worker_template": "local.review-bot"
}
```

说明：

- 两个字段都可选
- 更新后会做 bootstrap 配置校验
- 如果默认模板变化且 agent service 已挂载，会同步更新 gateway runtime

## 本地 IM API

这组接口对应 CSGClaw 本地 IM 数据。

Thread 模型、不变量、隐藏上下文行为和 bridge 规则见
[im-threads.zh.md](im/im-threads.zh.md)。

### `GET /api/v1/bootstrap`

获取 IM bootstrap 数据。

响应字段：

- `current_user_id`
- `users`
- `rooms`
- `invite_draft_user_ids`

bootstrap 响应中的 room 消息列表遵循默认时间线契约：只包含顶层消息，
不包含 thread reply。

### `GET /api/v1/events`

订阅本地 IM 事件流。

返回 `text/event-stream`，建立连接后先写入：

```text
: connected
```

随后按 SSE `data:` 帧推送 JSON 事件；心跳为：

```text
: ping
```

当前实际可能出现的事件类型包括：

- `message.created`
- `room.created`
- `room.members_added`
- `thread.created`
- `thread.updated`
- `upgrade.status_changed`

事件 JSON 结构：

- `type`
- `room_id`
- `room`
- `user`
- `message`
- `thread`
- `sender`
- `upgrade`

### `GET /api/v1/users`

列出本地 IM 用户。

### `POST /api/v1/users`

创建本地 IM 用户。

请求体：

```json
{
  "id": "alice",
  "name": "Alice",
  "role": "worker"
}
```

说明：

- `id` 必填
- `name` 必填
- 对于 `worker/agent` 角色，如果 participant service 与 agent service 已启用，应优先使用 participant API 创建 agent-backed 身份

### `DELETE /api/v1/users/{id}`

删除本地 IM 用户。

删除用户后会基于剩余 room 消息重建 thread state。被删除用户发送的 thread
root 会被移除，隐藏上下文快照会重新生成且不包含被删除用户的消息；能保留的
thread 创建时间会尽量保留。

常见返回：

- `204`：删除成功
- `404`：用户不存在
- `409`：尝试删除当前用户

### `GET /api/v1/rooms`

列出本地 IM 房间。

room 消息列表默认不包含 thread reply；当 thread 存在时，root message 仍会
带有 thread summary。

### `POST /api/v1/rooms`

创建房间。

请求体：

```json
{
  "title": "Launch",
  "description": "coordination",
  "creator_id": "manager",
  "member_ids": ["alice", "bob"],
  "locale": "en"
}
```

兼容字段：

- 旧请求中的 `participant_ids` 仍可被识别并映射到 `member_ids`

### `PATCH /api/v1/rooms/{id}`

更新本地房间行为。

请求体：

```json
{
  "notify_all_agents": true
}
```

群组房间默认仅通知被提及的 Agent。
启用 `notify_all_agents` 后，每条用户消息都会唤醒房间内的所有 Agent，包括带有显式提及的消息。
Agent 回复仍只会唤醒被显式提及的 Agent，以防止回复循环。
直接消息房间始终会通知对应 Agent，因此会拒绝此更新。

### `DELETE /api/v1/rooms/{id}`

删除房间，成功返回 `204`。

### `GET /api/v1/rooms/{id}/members`

列出房间成员。

### `POST /api/v1/rooms/{id}/members`

向指定房间加人。

请求体：

```json
{
  "inviter_id": "manager",
  "user_ids": ["bob"],
  "locale": "en"
}
```

说明：

- 路径中的 `{id}` 会作为 `room_id`
- 若 body 中也传了 `room_id`，必须与路径一致

### `POST /api/v1/rooms/invite`

按 room 维度添加成员，语义与 `POST /api/v1/rooms/{id}/members` 基本一致。

请求体：

```json
{
  "room_id": "room-1",
  "inviter_id": "manager",
  "user_ids": ["bob"],
  "locale": "en"
}
```

### `GET /api/v1/messages?room_id=...`

获取指定房间消息列表。

`room_id` 必填。

默认不返回 thread reply，因此房间主时间线保持顶层消息。添加
`include_thread_replies=true` 可把 thread reply 一起返回。

### `POST /api/v1/messages`

发送消息。

请求体：

```json
{
  "room_id": "room-1",
  "sender_id": "manager",
  "content": "hello @alice",
  "mention_id": "alice"
}
```

说明：

- `room_id` 必填
- 成功返回 `201 Created`
- 发送成功后会向 `/api/v1/events` 发布 `message.created`
- 发送 thread reply 时传入 `relates_to: {"rel_type":"m.thread","event_id":"<root_message_id>"}`
- `relates_to.rel_type` 当前支持 `m.thread`；root 必须是同一 room 内的顶层消息
- thread reply 还会发布 `thread.updated`
- 发送附件时使用 `multipart/form-data`，其中 `payload` JSON part 包含同样字段，`files` part 可以出现一次或多次。
- 至少带有一个文件时允许发送纯附件消息。
- 返回的 message 可以包含 `attachments`，字段包括 `id`、`name`、`kind`、`media_type`、`size_bytes`、`sha256`、`created_at`、`download_url`、可选的 `preview_url`、可选图片尺寸，以及面向 agent 投递时可选的 `workspace_path`。

Multipart 示例：

```text
payload={"room_id":"room-1","sender_id":"manager","content":""}
files=@diagram.png;type=image/png
```

### `GET /api/v1/attachments/{id}`

按 attachment ID 下载已存储的聊天附件。

附件元数据中的 `download_url` 包含附件级 capability token，浏览器和 agent 可以直接使用。

调用方也可以使用配置的服务端 Bearer token 请求不带 capability token 的基础路径。

该接口会使用存储的 media type 返回原始字节，并设置 `X-Content-Type-Options: nosniff`。

请将 capability URL 视为敏感信息，不要在 room 上下文之外共享。

图片附件以内联方式返回。

其他文件附件以下载方式返回。

### `POST /api/v1/rooms/{id}/threads`

从已有顶层消息开启一个 thread。thread 的规范 ID 就是 root message ID，
对应 Matrix `m.thread` 关系语义，但不占用原始 `/_matrix` namespace。

请求体：

```json
{
  "root_message_id": "msg-root"
}
```

响应：

- `201 Created`：创建了新的 thread state
- `200 OK`：thread 已存在，幂等返回

响应体是 `ThreadView`：

```json
{
  "room_id": "room-1",
  "root": { "id": "msg-root" },
  "context": [],
  "replies": [],
  "summary": {
    "root_id": "msg-root",
    "reply_count": 0,
    "participants": [],
    "current_user_participated": true,
    "context_summary": {
      "root_excerpt": "root text",
      "message_count": 1,
      "before_count": 0,
      "after_count": 0
    }
  }
}
```

`ThreadView.root` 是可见 root message，`context` 是给 LLM 上下文使用的隐藏
快照，`replies` 是可见 thread reply 列表，`summary` 是主时间线和 thread
列表使用的 root-level thread summary。

thread 开启时会固定一份上下文快照：root 之前最多 5 条顶层消息、root
消息本身，以及 root 之后最多 2 条顶层消息，并受 payload 大小限制。这份
context 不会被渲染成 thread 内消息；它是给 LLM-backed agent 使用的隐藏
上下文，让 thread 能以干净的新会话开始，同时理解它从哪里开启。

### `GET /api/v1/rooms/{id}/threads?include=all|participated&limit=&from=`

列出房间 threads。`include` 默认是 `all`；`participated` 只返回当前用户
作为 root 发送者或 reply 参与者的 threads。`limit` 与 `from` 是 offset
风格分页。

### `GET /api/v1/rooms/{id}/threads/{root_message_id}`

返回一个 `ThreadView`，包含 root message、隐藏上下文窗口、replies 和 summary。

### `GET /api/v1/rooms/{id}/relations/{event_id}/m.thread`

返回 Matrix 风格的 thread 子事件：

```json
{
  "chunk": []
}
```

## Channel API

## `csgclaw` channel

`/api/v1/channels/csgclaw/*` 基本是本地 IM 的镜像接口。

### 用户

- `GET /api/v1/channels/csgclaw/users`
- `POST /api/v1/channels/csgclaw/users`
- `DELETE /api/v1/channels/csgclaw/users/{id}`

说明：

- `GET` / `POST` 复用本地 IM 用户逻辑
- `DELETE` 走 channel 专用删除逻辑，但语义仍是删除本地用户

### 房间

- `GET /api/v1/channels/csgclaw/rooms`
- `POST /api/v1/channels/csgclaw/rooms`
- `DELETE /api/v1/channels/csgclaw/rooms/{id}`
- `GET /api/v1/channels/csgclaw/rooms/{id}/members`
- `POST /api/v1/channels/csgclaw/rooms/{id}/members`
- `POST /api/v1/channels/csgclaw/rooms/{id}/threads`
- `GET /api/v1/channels/csgclaw/rooms/{id}/threads`
- `GET /api/v1/channels/csgclaw/rooms/{id}/threads/{root_message_id}`
- `GET /api/v1/channels/csgclaw/rooms/{id}/relations/{event_id}/m.thread`

### 消息

- `GET /api/v1/channels/csgclaw/messages?room_id=...`
- `POST /api/v1/channels/csgclaw/messages`

## `feishu` channel

### 飞书凭证配置

`/api/v1/channels/feishu/config` 已移除。飞书凭证现在写在 Feishu
participant 的 `channel_app_config` 中，通过 `participant bind` 或 participant
更新接口来管理。

在 participant 响应中，`app_secret` 会脱敏为 `present`。

### Participant 事件

#### `GET /api/v1/channels/feishu/participants/{id}/events`

订阅指定 participant 在飞书中的被提及消息事件。

特点：

- 需要 Bearer Token
- 返回 `text/event-stream`
- 只转发“消息里 mention 到该 participant open_id”的事件
- 建立连接后先输出 `: connected`

### 用户

- `GET /api/v1/channels/feishu/users`
- `POST /api/v1/channels/feishu/users`
- `DELETE /api/v1/channels/feishu/users/{id}`

`POST` 请求体示例：

```json
{
  "id": "ou_xxx",
  "name": "Alice",
  "role": "member",
  "avatar": "AL"
}
```

### 房间

- `GET /api/v1/channels/feishu/rooms`
- `POST /api/v1/channels/feishu/rooms`
- `DELETE /api/v1/channels/feishu/rooms/{id}`
- `GET /api/v1/channels/feishu/rooms/{id}/members`
- `POST /api/v1/channels/feishu/rooms/{id}/members`

创建房间和加人时，请求体与本地 IM 基本一致，仍使用：

- `title`
- `description`
- `creator_id`
- `member_ids`
- `locale`

加人接口请求体：

```json
{
  "inviter_id": "manager",
  "user_ids": ["dev"],
  "locale": "zh-CN"
}
```

### 消息

- `GET /api/v1/channels/feishu/messages?room_id=...`
- `POST /api/v1/channels/feishu/messages`

发送消息请求体：

```json
{
  "room_id": "oc_xxx",
  "sender_id": "manager",
  "content": "hello",
  "mention_id": "worker"
}
```

## Runtime Bridge API

Runtime client 使用 participant-scoped 路由处理 channel 消息，使用 agent-scoped 路由处理 LLM provider 流量。旧的 `/api/bots/*` 路由不再注册。

Runtime 和 Codex bridge 使用的 thread/session 隔离规则见
[im-threads.zh.md](im/im-threads.zh.md)。

### `GET /api/v1/channels/{channel}/participants/{id}/events`

订阅 participant 事件流。

特点：

- 需要 Bearer Token
- 返回 `text/event-stream`
- 建立连接后先输出 `: connected`
- 心跳注释为 `: heartbeat`
- 事件名为 `message`
- 若客户端带 `Last-Event-ID`，服务端会按 replay 规则尝试补发最近消息

单条事件示例：

```text
id: msg-1
event: message
data: {"message_id":"msg-1","room_id":"room-1","channel":"csgclaw","chat_id":"room-1","sender_id":"admin","text":"hello","thread_root_id":"msg-root","context":{"channel":"csgclaw","chat_id":"room-1","chat_type":"direct","topic_id":"msg-root","sender_id":"admin","message_id":"msg-1"},"thread_context":{"root_message_id":"msg-root","context":[{"id":"msg-root","sender_id":"admin","content":"root text"}],"summary":{"root_excerpt":"root text","message_count":1,"before_count":0,"after_count":0}}}
```

对于 thread replies，`thread_root_id` 是 root message ID，`thread_context`
携带 thread 开启时记录的确定性隐藏上下文。Runtime/LLM bridge 会把它作为
prompt context 使用；它不是 thread reply 列表。PicoClaw 原生 client 可以把
`context.topic_id` 当作同一个 thread/session 标识。

事件可以包含 `attachments` 数组，字段与 message API 返回的附件元数据一致。

对于 CSGClaw agents，服务端还会尝试把每个附件复制到目标 agent workspace，并在复制成功时设置 `workspace_path`。

### `POST /api/v1/channels/csgclaw/participants/{id}/messages`

以指定本地 CSGClaw participant 身份发送消息。

请求体示例：

```json
{
  "room_id": "room-1",
  "text": "hello",
  "thread_root_id": "msg-root"
}
```

`thread_root_id`、`topic_id` 和 `context.topic_id` 都是可选的 thread/topic
标识；传入任一字段时 participant 响应会发送到该 IM thread 中。全部省略时，
响应会作为 room/DM 顶层消息发送；服务端不会根据 participant 在房间中最近收到的
事件推断 thread。

该接口也接受与 `POST /api/v1/messages` 相同的 multipart 附件格式。

使用 `payload` JSON part 传 participant 消息字段，并用一个或多个 `files` part 传生成的文件。

也接受 PicoClaw outbound message 形态：

```json
{
  "chat_id": "room-1",
  "content": "hello",
  "context": {
    "channel": "csgclaw",
    "chat_id": "room-1",
    "topic_id": "msg-root"
  }
}
```

### `POST /api/v1/agents/{agent}/sessions/{session_id}/responses`

通过选定 Agent 及其真实 CSGClaw runtime 执行一次 turn。
`{agent}` selector 可以使用 Agent ID，也可以使用不区分大小写的唯一 Agent 名称。
该接口与 `/llm/responses` 相互独立，后者只代理模型流量，不执行 Agent。

该接口不要求 Bearer token。
因为 admin 目前是唯一用户，Anonymous 复用现有 `user-admin` 身份表示。
服务端不接受调用方指定身份，每条输入消息都以 `sender_id: "user-admin"` 持久化。

客户端提供一个全局 `session_id`，长度为 1-128 位，只能包含 path-safe ASCII 字符。
第一次请求创建一个 non-direct room，后续请求复用该 room。
Room 中只能包含 admin 和所选 Agent，服务端会保持 `notify_all_agents` 开启。
Room title 创建后不再变化，并使用以下便于审计的格式：

```text
Anonymous Session: <session_id> | Agent: <agent_name> (<agent_id>)
```

最简请求使用字符串 input：

```json
{
  "input": "Review this patch."
}
```

也可以使用仅包含文本的 user message items：

```json
{
  "input": [
    {
      "type": "message",
      "role": "user",
      "content": [
        {
          "type": "input_text",
          "text": "Review this patch."
        }
      ]
    }
  ],
  "stream": false
}
```

省略 `stream` 或设置为 `false` 时，接口返回包含最终 assistant 文本和 session
元数据的 Responses 风格 JSON。设置 `stream: true` 时，接口返回
`text/event-stream`，格式为与 CSGBot Genius 兼容的 Anthropic 风格事件流。
纯文本流的事件顺序为：

```text
message_start
content_block_start       (text)
content_block_delta       (text_delta)
content_block_stop
message_delta             (stop_reason: end_turn)
message_stop
```

每个 SSE block 同时包含 `event: <type>` 和 JSON `data:` payload；payload
中的 `type` 与事件名一致。Codex 工具活动以 `tool_use` content block 和
`input_json_delta` 返回，随后以自定义的 `tool_result` content block 和
`tool_result_delta` 返回工具结果，与 Genius chat 协议一致。`tool_use` 会保留
runtime 提供的具体工具名和结构化输入，敏感值在返回前会被脱敏。响应空闲期间，
服务端约每 15 秒发送一次 SSE `: heartbeat` comment 以保持连接。对于 Codex 智能体，
Codex app-server 发布的 final-answer `item/agentMessage/delta` 会即时转发为
`text_delta`；commentary 和无法分类的 agent message 不会计入回答。完成事件不再
重复携带全量文本，前端应通过追加 delta 构建可见回答。只发布 final message 的
runtime 会降级为一个文本 delta。Agent 完成前，连接已建立并先 flush
`message_start`。流式失败依次返回 `error`、带 `stop_reason: error` 的
`message_delta` 和 `message_stop`。

同一个 session 同时只允许一个 turn。
不同 session ID 可以并发执行。
同一个全局 session 改用其它 Agent 时返回 `409 session_agent_conflict`。
服务端最多等待五分钟，之后返回 `504 response_timeout`。

### `POST /api/v1/agents/{agent}/sessions/{session_id}/responses/cancel`

取消所选 Agent 与 Session 当前正在执行的 response。
只有 Runtime 取消和 turn cleanup 都完成后，该请求才返回 `204 No Content`，此时 client 可以安全发送下一个 turn。
没有 active response 时重复调用也是幂等的，并返回 `204 No Content`。

错误使用以下 JSON 格式：

```json
{
  "error": {
    "message": "another response is already running for this session",
    "type": "conflict_error",
    "param": "session_id",
    "code": "session_busy"
  }
}
```

V1 只接受文本并返回最终文本。
流式输出、tools、instructions、非 user role、attachments 和未知请求字段都会被拒绝。
内置 live demo 和可替换的前端 mock 边界见 [Session API Demo 前端指南](web/session-api-demo.zh.md)。

### `GET /api/v1/agents/{id}/llm/models`

### `GET /api/v1/agents/{id}/llm/v1/models`

转发模型列表请求到 LLM bridge。

说明：

- 需要 Bearer Token
- 返回内容类型和响应体由上游 bridge 决定

### `POST /api/v1/agents/{id}/llm/chat/completions`

### `POST /api/v1/agents/{id}/llm/v1/chat/completions`

转发聊天补全请求到 LLM bridge。

说明：

- 需要 Bearer Token
- 请求体会原样读取并转发
- 单次读取上限为 `10 MiB`
- 失败时可能返回普通文本错误，也可能返回：

```json
{
  "error": {
    "code": "unauthorized",
    "message": "upstream auth failed",
    "provider": "openai"
  }
}
```

### `POST /api/v1/agents/{id}/llm/responses`

### `POST /api/v1/agents/{id}/llm/v1/responses`

### `GET /api/v1/agents/{id}/llm/responses`

### `GET /api/v1/agents/{id}/llm/v1/responses`

转发 OpenAI-compatible Responses API 请求到 LLM bridge。Codex runtime 使用这个入口发送 provider 流量。如果所选上游 provider 返回不支持 Responses endpoint 的状态，bridge 会回退到上游 chat completions，并把结果包装成 Codex 可消费的 Responses-compatible response。

`GET` 形式是 Responses API session 的 websocket upgrade 入口。

请求体示例：

```json
{
  "model": "ignored-by-server",
  "input": "Review this patch.",
  "stream": true
}
```

说明：

- 需要 Bearer Token
- 请求会先转发到所选 profile 的 `base_url + /responses`
- 若上游 `/responses` 返回 `404` 或 `405`，bridge 会改为请求 `base_url + /chat/completions`
- `model` 字段会被覆盖为 agent 已解析出的 `model_id`
- Responses 转发不会注入 chat-only 的顶层 `reasoning_effort`
- 上游 Responses 的 headers、status 和 body 会被透传，包括 `text/event-stream` 这类流式响应

## 兼容性说明

- `CreateRoomRequest.participant_ids` 仍兼容旧字段，会映射到 `member_ids`
- `Message.mentions` 兼容旧格式：
  - 新格式：`[{ "id": "alice", "name": "Alice" }]`
  - 旧格式：`["u-alice"]`
- 本地 `csgclaw` channel 路由本质上是 `/api/v1/users|rooms|messages` 的镜像入口

## 当前未暴露的旧接口

以下旧文档中常见路径，当前路由里已不存在，不应再作为对外 API 使用：

- `/api/v1/notify/{agent_id}`
- `/api/v1/channels/{channel}/bots`
- `/api/v1/channels/{channel}/bots/{id}`
- `/api/v1/channels/feishu/bots/{id}/events`
- `/api/bots/{id}/events`
- `/api/bots/{id}/messages/send`
- `/api/bots/{id}/llm/*`
- 任何未在 `internal/api/router.go` 中注册的旧路径
