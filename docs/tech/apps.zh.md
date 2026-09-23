# 连接器

[English](apps.md) | 中文

界面统一使用“连接器（Connector）”；内部 Plugin 包结构、CLI 命令和现有 API 路径保持不变。

在 **资源 > 连接器** 中统一配置 GitLab、飞书和 llm-wiki 连接。
每个全局实例有独立名称、服务地址和受保护的凭据或凭据引用，同一种 App 可以配置多个账户或服务实例。

## 配置资源与添加到 Agent

连接器表单底部展示默认折叠的工具列表。
测试连接后显示当前表单对应的工具；修改表单后需要重新测试以更新列表。
填写全局资源配置，点击 **测试连接** 验证当前表单，通过后按需点击 **保存配置**，再在 Agent 的 **连接器** 页选择 **从资源添加**。
Agent 绑定仅保存资源引用和自己的启停、断开意图，不复制秘密凭据。
每个绑定保留独立的 MCP 会话、本地进程目录和 Agent 工具名称。
修改全局资源会撤下旧工具，并重连之前已连接且启用的绑定；已手动断开或停用的绑定不会被恢复。
全局停用影响所有绑定，Agent 停用只影响自己。
移除绑定或删除 Agent 不会删除全局资源或其他 Agent 的绑定。
删除全局资源时展示受影响的 Agent，并移除该资源的所有绑定。

飞书连接器独立保存 App ID/App Secret，不再引用任意 Agent 的渠道凭据。
所有绑定的 Agent 使用该全局连接器的身份；修改渠道不影响连接器凭据。
测试只使用当前表单和未修改的已存凭据，不写入资源配置或新凭据。
GitLab 测试会先通过实例的 `/api/v4/user` 验证填写的 PAT，再检查 MCP 连接和工具发现。
保存接口会先验证候选凭据、MCP 初始化和工具发现，通过后才写入新连接设置。
验证失败时保留原配置与现有连接；上游不可用时仍可停用或移除连接器。
验证期间配置被其他操作修改时，本次保存会被拒绝，需要重新验证。
App 连接已有的 HTTP 或 stdio MCP 服务，CSGClaw 不负责部署该服务。
浏览器 OAuth2 仍未实现。
已有本地安装记录会一次性转换为全局资源与绑定，保留绑定 ID、连接意图和工具名称。

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

HTTP 飞书连接器填写 MCP URL 及其独立的 App ID/App Secret。
Connector 复用飞书 Token 获取实现，按连接实例缓存 tenant access token，并在有效期结束前按需重新获取。
调用 MCP 时自动注入 `lark-access-token` 和 `X-Lark-Token-Type: tenant_access_token`，平台 Token 单独放入 `Authorization: Bearer ...`。
不需要填写 Header 名称或临时飞书 Token，App Secret 不会发给 passthrough MCP。
只有上游明确返回 `lark_token_invalid` 时才刷新 Token 并重试一次；权限不足、网络错误或其他业务错误不自动重试。
重复拒绝刷新后的 Token 会将连接标记为需要重新授权。
连接器凭据变化后，连接重新建立并使用新的 Token 缓存；断开、停用和移除遵守原有生命周期规则。
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
csgclaw app list --global
csgclaw app add --global --file resource.json
csgclaw app update --global --id RESOURCE_ID --file changes.json
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

创建全局 App 的 `resource.json` 示例：

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
  "connect": false
}
```

`probe` 请求去掉 `name` 和 `connect`，只保留 `app_id`、`config`、`credentials` 和可选的 `installation_id`。
`update` 只提供待修改字段，例如 `{"enabled":false}`，不包含 `app_id` 和 `connect`。
Agent 添加请求使用 `{"resource_id":"RESOURCE_ID","connect":true}`；连接失败时保留绑定及诊断状态，供用户修复全局配置或该 Agent 的身份。
Agent 运行环境中的 CLI 只允许目录、列表和详情读取，并提供 App 设置链接；修改连接设置与凭据请使用 App 页面。

## API 与运行行为

| 接口 | 用途 |
|---|---|
| `GET /api/v1/connectors/catalog` | 内置 App 类型目录 |
| `GET /api/v1/connectors/catalog/{app_id}` | 定义和连接默认值 |
| `GET/POST /api/v1/connectors/resources` | 列出或创建全局资源 |
| `GET/PATCH/DELETE /api/v1/connectors/resources/{resource_id}` | 管理全局资源并查看受影响的 Agent |
| `POST /api/v1/connectors/resources:probe` | 使用独立凭据测试全局资源 |
| `GET/POST /api/v1/agents/{agent_id}/connectors` | 列出或添加资源绑定 |
| `GET/PATCH/DELETE /api/v1/agents/{agent_id}/connectors/{installation_id}` | 读取、启停或移除一个绑定 |
| `POST /api/v1/agents/{agent_id}/connectors:probe` | 使用 Agent 身份测试连接 |
| `POST .../{installation_id}/connect` 或 `/disconnect` | 连接或断开一个绑定 |

Codex 通过该 Agent 的受管 `/api/v1/agents/{agent_id}/mcp` 入口使用 App 工具。
CSGClaw 内置 MCP 入口被标记为必需服务，Codex 在启动及工具刷新后等待工具清单就绪再生成回答。
工具加载较慢时，不应使用不完整的可选服务清单将已连接的连接器误判为不可用。
已有手动 MCP 继续独立工作，由 App 管理的记录跳转到 App 设置。
停用、断开或移除后立即禁止后续调用，旧会话保留的工具名也不能绕过。
只读 Agent 只能发现和调用标记为只读的工具。
工具搜索由 Codex 提供，含工具定义或工具历史的请求不会静默降级到纯文本 Chat Completions。

内部包使用根目录 `plugin.json` 和 `apps.json`，三个内置 App 不需要 `mcp.json`。
凭据保存在私有本地状态中，每个 stdio 安装实例拥有独立的 `HOME`、`PLUGIN_DATA` 和 `PLUGIN_ROOT` 目录。
插件市场、任意包导入、Skills/Hooks 执行和浏览器 OAuth2 不属于当前版本的 App 流程。

## 安装默认值与设置表单

新增 App 表单读取目录中 `config_schema.properties.*.default` 声明的非秘密连接默认值。
GitLab 预填 `https://u-agentichub-gitlab-mcp-1qq.public.opencsg.com/mcp`；飞书预填 `https://u-agentichub-lark-mcp-passthrough-1qr.public.opencsg.com/mcp`。
连接时使用与服务地址匹配的 OpenCSG 登录环境。
llm-wiki 暂时预填本地测试服务 `http://127.0.0.1:19093/mcp`，可以手动替换地址或通过知识库选择器填写。
本机地址指运行 CSGClaw 的机器。
默认值不包含凭据，已安装实例继续使用已保存的配置。
表单分为服务连接、MCP 服务鉴权、飞书应用身份；超时和附加 Header/环境变量放在高级设置中。

Agent 添加接口使用 `{"resource_id":"RESOURCE_ID","connect":true}`，Agent 更新接口仅接受 `enabled`。
全局资源通过 `/api/v1/connectors/resources` 管理，通过 `/api/v1/connectors/resources:probe` 测试。

对话输入区仅保留附件上传，旧 GitHub/GitLab 连接器入口及其前端 OAuth 轮询已移除。
