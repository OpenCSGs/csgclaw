# CSGClaw Channel 集成

[English](csgclaw.md) | 中文

本文档说明前端与 runtime 自动化使用的 CSGClaw Channel 侧交互约定，作为按通道文档（例如 `docs/tech/channel/feishu.md`）的补充说明。

## `csgclaw.action_card` 结构

某些流程（尤其是 Feishu 管理员初始化）会在最终消息中返回一段结构化卡片，而不是普通文本。前端识别该格式并渲染交互按钮。

```json
{
  "type": "csgclaw.action_card",
  "status": "manager_recreate_pending",
  "bot_id": "u-manager",
  "agent_id": "u-manager",
  "title": "Manager Feishu 配置已完成",
  "subtitle": "u-manager",
  "badge": "需在窗口点击",
  "summary": "飞书配置已写入并重新加载。Manager 需要重建后才能把新配置注入运行环境。请直接点击下方按钮，由浏览器发起安全的 Manager bootstrap replace。",
  "actions": [
    {
      "id": "rebuild-manager",
      "label": "重建 Manager",
      "style": "danger",
      "method": "manager-bootstrap-replace",
      "confirm": "重建 Manager 会中断当前 Manager，会话可能需要刷新。确认继续？"
    }
  ]
}
```

### 必填行为

- `type` 必须是 `csgclaw.action_card`。
- `bot_id` 是现有 setup helper 沿用的旧 payload 字段；其值表示目标 agent ID，不是 participant ID。
- `agent_id` 与 `bot_id` 保持一致，供 participant 语义的新调用方使用；旧客户端仍只认识 legacy 名称时，两者都要保留。
- `actions[0].id` 必须是 `rebuild-manager`。
- `actions[0].method` 必须是 `manager-bootstrap-replace`。
- 前端必须将该 payload 直接作为完整聊天内容渲染（不允许附加普通文本、markdown 表格或 markdown 代码块）。

### Manager 重建执行约定

`rebuild-manager` 动作必须由浏览器/Web UI 发起，调用：

```bash
POST /api/v1/agents
Content-Type: application/json

{"id":"u-manager","replace":true}
```

请勿调用 `POST /api/v1/agents/u-manager/recreate` 来完成该流程。

### 安全说明

- 不要返回或记录敏感凭证（如 `app_secret`、API key、token）。
- 若敏感值出现在日志中，应使用掩码形式（例如 `present`）。

## Notification participant（通知参与者）

通知发送者是 `type=notification` 的 CSGClaw participant，不创建 backing worker agent；投递配置保存在 participant `metadata` 中。默认 participant id 为 `n-{name}`（与 worker agent 的 `u-{name}` 区分）；创建时也可显式指定 `id`，但不得与同 channel 下已有 participant 冲突。

- 列表：`GET /api/v1/channels/csgclaw/participants?type=notification`
- 创建：`POST /api/v1/channels/csgclaw/participants`，请求体含 `"type":"notification"` 与 `metadata`
- 更新：`PATCH /api/v1/channels/csgclaw/participants/{id}`
- 删除：`DELETE /api/v1/channels/csgclaw/participants/{id}`
- 推送（webhook）：`POST /api/v1/channels/csgclaw/participants/{id}/notifications`，请求头 `Authorization: Bearer <webhook_token>`

实现：`internal/channel/csgclaw/notification/`。

## `csgclaw.notify_card` 结构

通知投递（GitLab/GitHub webhook 等）到 CSGClaw Web IM 时使用该类型：**整条消息的 `content` 即一段 JSON**，由服务端 `internal/channel/csgclaw/notification` 生成，Web 前端按 `type` 渲染为结构化卡片（标题、徽章、元数据行、可选链接与折叠原始 JSON）。

```json
{
  "type": "csgclaw.notify_card",
  "schema_version": 1,
  "provider": "gitlab",
  "event": "merge_request",
  "title": "GitLab · Merge request",
  "subtitle": "acme/app",
  "badge": "open",
  "summary": "",
  "link": "https://gitlab.example/acme/app/-/merge_requests/1",
  "meta": [
    { "label": "标题", "value": "Fix bug" },
    { "label": "分支", "value": "fix → main" }
  ],
  "raw": ""
}
```

### 字段说明

- `type`：必须为 `csgclaw.notify_card`。
- `schema_version`：当前为 `1`。
- `provider`：`gitlab`、`github` 或 `generic`（未识别 JSON 等）。
- `event`：规范化事件名（如 `push`、`merge_request`、`issue`、`pull_request`、`ping`）；`generic` 时可为 `json`、`text`、`empty` 等。
- `title` / `subtitle` / `badge` / `summary`：展示用；`summary` 可与 `meta` 二选一或并存。
- `link`：可选 HTTP(S) 链接；前端仅允许 `http:` / `https:`。
- `meta`：可选 `{ "label", "value" }` 列表。
- `raw`：可选，通常为未匹配 webhook 时的缩进 JSON 片段（已截断）；前端放在折叠区。
- 与 `action_card` 相同：**整条聊天 `content` 应为裸 JSON**，不要外包 Markdown 或代码块。

### 相关代码路径

- 前端解析与渲染：`web/app/src/components/business/MessageContent/MessageContent.tsx`、`web/app/src/components/business/MessageContent/structuredMessages.ts`
- Action card 与 Notifier card 单测：`web/app/tests/legacy-contract.test.ts`、`web/app/tests/components/MessageContent/structuredMessages.test.ts`
- 通知卡片生成：`internal/channel/csgclaw/notification/notify_card.go`、`internal/channel/csgclaw/notification/notify_webhooks.go`
- Feishu setup 命令输出：`internal/templates/embed/runtimes/picoclaw/manager/workspace/skills/feishu/scripts/feishu_setup/csgclaw.py`
