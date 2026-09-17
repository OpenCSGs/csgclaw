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

## OpenCSG 平台鉴权

所有 HTTP App，包括 GitLab、飞书和 llm-wiki，均支持通用的 OpenCSG 平台凭据来源。
新增 OpenCSG HTTPS 地址时，默认“复用当前 OpenCSG 登录”，每次请求读取最新凭据，不复制 Token 到 App。
用户明确选择手动模式或提供手动 Bearer Token 时，保留该选择。
业务凭据可通过 `PRIVATE-TOKEN`、`X-API-Key` 等独立 Header 同时发送；如果与平台凭据同时占用 `Authorization`，会明确提示冲突。
第三方地址、本地地址和 stdio 进程不会自动携带 OpenCSG 登录凭据。
仅向与当前登录环境匹配的 OpenCSG HTTPS 地址发送该凭据；正式环境与 staging、外部域名之间不会混用。
也可选择手动填写平台 Token；已过期的手动 Token 会明确提示更新或切换登录引用。
退出登录后引用连接需要重新授权，登录后会刷新未停用、未手动断开的引用连接。
前端分别显示平台 401/403、未登录、环境不匹配、飞书 Token 获取失败、MCP 地址错误或后端休眠/不可用，并保留 HTTP 状态信息。
## 飞书远端 Passthrough 服务

HTTP 飞书 App 选择“飞书应用凭据”，填写 MCP URL，并选择当前 Agent 的飞书 Channel 或手动填写 App ID/App Secret。
Connector 复用飞书 Token 获取实现，按连接实例缓存 tenant access token，并在有效期结束前按需重新获取。
调用 MCP 时自动注入 `lark-access-token` 和 `X-Lark-Token-Type: tenant_access_token`，平台 Token 单独放入 `Authorization: Bearer ...`。
不需要填写 Header 名称或临时飞书 Token，App Secret 不会发给 passthrough MCP。
只有上游明确返回 `lark_token_invalid` 时才刷新 Token 并重试一次；权限不足、网络错误或其他业务错误不自动重试。
重复拒绝刷新后的 Token 会将连接标记为需要重新授权。
Channel 凭据变化后，引用连接重新建立并使用新的 Token 缓存；断开、停用和移除遵守原有生命周期规则。
stdio 模式继续将应用凭据注入本地 MCP 进程的环境变量。
用户身份的 UAT 和浏览器 OAuth 不属于该应用身份流程，当前仍不支持自动用户授权；已有 UAT 可通过 Bearer/自定义 Header 模式手动配置。

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
