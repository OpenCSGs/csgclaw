# GitLab Connector

GitLab Connector 统一使用 Personal Access Token（PAT），不再通过 OAuth 获取 GitLab Token，也不再把 MCP 地址固定为 GitLab 官方的 `/api/v4/mcp`。

## 配置职责

- GitLab Connector 是全局状态；所有 Agent 共享同一个 GitLab 实例根地址和 PAT，并通过 `GET <base_url>/api/v4/user` 校验凭据。
- Agent 的 GitLab App 保存实际使用的 MCP 服务地址和 GitLab 实例根地址。
- App 发起 MCP 请求时，Connector 默认注入 `PRIVATE-TOKEN` 和 `X-GitLab-Base-URL`；用户可按 MCP 服务约定覆盖或增加 Header。
- MCP 服务位于 OpenCSG 域名时，还可选择复用当前 OpenCSG 登录凭据作为入口 `Authorization`；GitLab PAT 与平台 Token 相互独立。

因此，不同 Agent 可以各自配置不同的上游 GitLab MCP 服务，但使用同一份全局 GitLab 实例地址和 PAT。

## PAT 配置

在 App 页面填写：

1. MCP 服务的完整地址，例如 `https://service.public.opencsg.com/mcp`。
2. GitLab 实例根地址，例如 `https://gitlab.example.com`。
3. 具有所需权限的 GitLab PAT。只读调用可使用 `read_api`，需要写操作时使用 `api`。

也可直接调用 Connector API：

```bash
curl -X PUT "$CSGCLAW_BASE_URL/api/v1/connectors/gitlab/config" \
  -H "Authorization: Bearer $CSGCLAW_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"base_url":"https://gitlab.example.com","access_token":"<gitlab-pat>","auth_method":"personal_access_token"}'
```

PAT 只保存在全局 Connector 状态中，不会写入 App 配置或状态响应。编辑已有 App 时 Token 输入框留空会继续使用当前全局 PAT。“测试连接”只使用表单中的临时 PAT，不会覆盖已保存的 Connector；“保存配置”或“保存并连接”才会持久化。

## 请求头

连接 App 时，请求会直接发送到 App 配置的 MCP 地址：

```text
PRIVATE-TOKEN: <connector-managed GitLab PAT>
X-GitLab-Base-URL: https://gitlab.example.com
Authorization: Bearer <current OpenCSG login token>  # 仅选择复用 OpenCSG 登录时
```

Connector 会校验 App 中的 GitLab 实例地址与当前 Connector 配置一致，避免把 PAT 用到错误实例。HTTP 客户端也会限制凭据只发送到配置的 MCP origin，并拒绝携带凭据跟随跨地址重定向。

`PRIVATE-TOKEN` 和 `X-GitLab-Base-URL` 只是内置默认值，不是保留 Header。自建 MCP 可以在 App 高级设置中覆盖它们或使用完全不同的 Header。选择复用 OpenCSG 登录时，`Authorization` 用于 OpenCSG 网关认证，因此最终由当前平台登录凭据设置。

## 当前实现边界

GitLab Connector 负责保存和校验全局 GitLab PAT；每个 Agent 的 GitLab App installation 仍独立保存 MCP 服务地址和非托管 Header。App 运行时由 `internal/apps` 创建 Streamable HTTP MCP client，并在建立连接时解析 Connector 凭据、构造受限 HTTP transport。当前没有独立的通用 Connector MCP 反向代理层。

全局 PAT 更新后，所有引用 GitLab Connector 的 Agent App session 都会关闭并使用新凭据重新连接；断开 Connector 时则关闭这些 session 并从 Agent 工具目录移除相应工具，避免已建立的 client 继续使用旧 PAT。

## 状态与断开

```bash
curl "$CSGCLAW_BASE_URL/api/v1/connectors/gitlab"
curl -X POST "$CSGCLAW_BASE_URL/api/v1/connectors/gitlab/disconnect"
```

状态响应的 `auth_method` 为 `personal_access_token`。断开会清除 PAT 和账号信息，但保留 GitLab Base URL。

旧版本的 `auth.gitlab_agents` 数据会在首次读取时迁移到 `auth.gitlab`；优先采用 Manager 的配置，否则按 Agent ID 选择第一份配置。

旧的 GitLab App `auth_mode = "oauth2"` 配置在加载或更新时会归一化为 `connector`，用户需要在 App 页面补充 MCP 地址、GitLab 实例地址和 PAT 后重新测试连接。
