# GitLab Connector

GitLab Connector 首期使用 GitLab 实例 Base URL 与 Personal Access Token（PAT）连接，支持 GitLab.com 和自托管 GitLab。连接保存时会请求 `GET <base_url>/api/v4/user` 校验地址、Token 与账号身份。

## 创建或更新

```bash
curl -X PUT "$CSGCLAW_BASE_URL/api/v1/connectors/gitlab/config" \
  -H "Authorization: Bearer $CSGCLAW_ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  --data '{"base_url":"https://gitlab.example.com","access_token":"<gitlab-pat>"}'
```

`base_url` 必须是无认证信息、query 和 fragment 的绝对 HTTP(S) URL。更新 Base URL 时可省略 `access_token` 以复用已保存 Token。响应只返回 `access_token_set`，不会回传 Token。

建议按任务授予最小 PAT scope：只读查询使用 `read_api`，需要创建或更新 Merge Request、Issue、Pipeline 等资源时使用 `api`。实际权限仍受 GitLab 用户在目标项目中的角色限制。

## 状态与断开

```bash
curl "$CSGCLAW_BASE_URL/api/v1/connectors/gitlab"
curl -X POST "$CSGCLAW_BASE_URL/api/v1/connectors/gitlab/disconnect"
```

断开会清除保存的 Token 和账号信息，但保留 Base URL，方便重新连接。

## Manager 使用方式

只有内置 Manager 可以通过托管凭据接口获取短时 lease：

```text
POST /api/v1/agents/agent-manager/connectors/gitlab/credential
```

请求必须同时携带共享服务认证 `Authorization: Bearer $CSGCLAW_ACCESS_TOKEN` 和仅注入 Manager runtime 的 `X-CSGClaw-Connector-Capability: $CSGCLAW_CONNECTOR_CAPABILITY`。仅有共享服务 Token、伪造 `agent-manager` 路径或从 Worker 发起请求都无法取得凭据。

lease 包含 `base_url`、`access_token`、`token_type=private-token`、账号信息和过期时间。分配给 Manager 的 GitLab skill 可在 GitLab 任务开始前动态请求 lease，通过 GitLab API v4 工作，并禁止将 Token 写入文件、日志、Prompt、消息或 Git credential store。GitLab skill 可从 SkillHub 独立安装，不属于 Manager 内置模板；Worker 默认无权获取该凭据。
