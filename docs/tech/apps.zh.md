# Agent Apps

[English](apps.md) | 中文

每个 Agent 独立管理自己的 GitLab、飞书和 llm-wiki App。
同一个 App 可以添加多份，各自保存名称、服务地址和凭据。

## 添加与连接

在 Agent 的 **Apps** 页签点击 **添加 App**，填写服务配置、测试连接，再完成添加。
App 连接已有的 HTTP 或 stdio MCP 服务，CSGClaw 不负责部署上游服务。
HTTP 连接需要填写 MCP URL 和该服务要求的鉴权方式。
stdio 连接需要填写命令、参数、工作目录及必要环境变量。

GitLab 使用上游要求的 Token/PAT 或自定义鉴权 Header。
飞书可以引用当前 Agent 的渠道 App ID 和 App Secret，也可以单独填写凭据。
渠道凭据在连接时解析，并在变更后刷新，不复制渠道 Secret 到安装记录。
llm-wiki 使用知识库 MCP 地址和 Token，知识库选择器可以辅助填写地址。
当前版本不支持浏览器 OAuth2 授权。

**停用**保留配置和凭据，但禁止工具调用。
**断开**清除安装实例托管的凭据，保留名称和普通配置。
手动断开后，渠道更新和服务重启都不会自动重连，只有显式连接才恢复。
**移除**删除安装记录和私有数据，不删除引用的飞书渠道。

## 访问方式

App 管理与 CSGClaw 个人助理页面一样，可直接打开使用，无需填写 CSGClaw 服务 Token。
GitLab、飞书和知识库需要的凭据仍在各自 App 中配置。
Agent MCP 继续使用系统自动配置的专属 Token，按 Agent 校验工具访问权限。
App CLI 无需额外登录，服务地址通过 `CSGCLAW_BASE_URL` 指定。
原有受保护 API 的服务 Token 校验保持独立，`server.no_auth` 不作为页面登录开关。

## 飞书远端 Passthrough 服务

对于入口使用 OpenCSG Token、业务调用透传飞书 Token 的 MCP，选择 HTTP 和 Bearer Token 鉴权。
Token 字段填写 OpenCSG Access Token，在高级 Header 中填写 `lark-access-token`（原始飞书访问 Token，不带 `Bearer`）和 `X-Lark-Token-Type`（`user_access_token` 或 `tenant_access_token`）。
这些 Header 随上游 MCP 请求发送，并按实例作为凭据保存，不会回显原值。
该模式的上游服务不接收 App Secret，因此不要选择把 App ID/App Secret 直接映射到 Header 的鉴权方式。
当前支持手动提供已有的飞书 Token，尚未实现此模式下的 Token 换取、自动刷新或用户 OAuth 授权。
App ID/App Secret 换取的应用凭据通常是 tenant access token；user access token 还需要用户授权。

## 对话中选择 App

用户可以直接提出“列出 GitLab 的 Issue”等请求，无需指定 App 实例名称。
只有一个匹配且已连接的 App 时，Agent 直接使用它。
有多个匹配 App 时，优先根据明确的账号、资源链接、项目标识或当前任务中已确认的选择确定实例；仍有歧义时才列出 App 名称询问用户。
没有可用 App 时，引导用户添加或重新连接。
工具描述包含所属 App 的名称和服务类型，重命名 App 后同步更新工具目录。

## CLI

`csgclaw` 和 `csgclaw-cli` 均支持以下命令：

```sh
csgclaw app catalog
csgclaw app list --agent agent-dev
csgclaw app get --agent agent-dev --id INSTALLATION_ID
csgclaw app probe --agent agent-dev --file probe.json
csgclaw app add --agent agent-dev --file request.json
csgclaw app update --agent agent-dev --id INSTALLATION_ID --file changes.json
csgclaw app connect --agent agent-dev --id INSTALLATION_ID
csgclaw app disconnect --agent agent-dev --id INSTALLATION_ID
csgclaw app remove --agent agent-dev --id INSTALLATION_ID
```

`add`、`update` 和 `probe` 从 `--file` 读取 JSON 请求，`--file -` 表示从标准输入读取。
App 凭据不提供命令行参数。
凭据文件应使用 `0600` 等私有权限，并排除在版本控制之外。
CLI 只输出 API 已脱敏的结果，不回显请求文件及其凭据。
需要完整响应字段时，在 `app` 前加全局参数 `--output json`。

添加 App 的 `request.json` 示例：

```json
{
  "app_id": "gitlab",
  "name": "公司 GitLab",
  "config": {
    "transport": "http",
    "url": "https://mcp.example.com/mcp",
    "auth_mode": "bearer"
  },
  "credentials": {"token": "<上游服务Token>"},
  "connect": true
}
```

`probe` 请求去掉 `name` 和 `connect`，只保留 `app_id`、`config`、`credentials` 和可选的 `installation_id`。
`update` 只提供待修改字段，例如 `{"enabled":false}`，不包含 `app_id` 和 `connect`。
添加后连接失败时，接口仍返回已创建的安装记录及错误状态，方便继续修改配置。
Agent 运行环境中的 CLI 只允许目录、列表和详情读取，并提供 App 设置链接；修改连接设置与凭据请使用 App 页面。

## API 与运行行为

| 接口 | 用途 |
|---|---|
| `GET /api/v1/apps` | 内置目录 |
| `GET /api/v1/apps/{app_id}` | App 定义和配置字段 |
| `GET/POST /api/v1/agents/{agent_id}/apps` | 列出或添加实例 |
| `GET/PATCH/DELETE /api/v1/agents/{agent_id}/apps/{installation_id}` | 读取、修改或移除实例 |
| `POST /api/v1/agents/{agent_id}/apps:probe` | 测试未保存的连接配置 |
| `POST .../{installation_id}/connect` 或 `/disconnect` | 连接或显式断开 |

Codex 通过该 Agent 的受管 `/api/v1/agents/{agent_id}/mcp` 入口使用 App 工具。
已有手动 MCP 继续独立工作，由 App 管理的记录跳转到 App 设置。
停用、断开或移除后立即禁止后续调用，旧会话保留的工具名也不能绕过。
只读 Agent 只能发现和调用标记为只读的工具。
工具搜索由 Codex 提供，含工具定义或工具历史的请求不会静默降级到纯文本 Chat Completions。

内部包使用根目录 `plugin.json` 和 `apps.json`，三个内置 App 不需要 `mcp.json`。
凭据保存在私有本地状态中，每个 stdio 安装实例拥有独立的 `HOME`、`PLUGIN_DATA` 和 `PLUGIN_ROOT` 目录。
插件市场、任意包导入、Skills/Hooks 执行和浏览器 OAuth2 不属于当前版本的 App 流程。
