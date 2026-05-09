# CSGClaw Channel 集成

[English](csgclaw.md) | 中文

本文档说明前端与 runtime 自动化使用的 CSGClaw Channel 侧交互约定，作为按通道文档（例如 `docs/channel/feishu.md`）的补充说明。

## `csgclaw.action_card` 结构

某些流程（尤其是 Feishu 管理员初始化）会在最终消息中返回一段结构化卡片，而不是普通文本。前端识别该格式并渲染交互按钮。

```json
{
  "type": "csgclaw.action_card",
  "status": "manager_recreate_pending",
  "bot_id": "u-manager",
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

### 相关代码路径

- 前端解析与渲染：`web/static/app.js`
- Action card 单测：`web/static/app_action_card.test.cjs`
- Feishu setup 命令输出：`runtimes/picoclaw/manager/workspace/skills/feishu/scripts/feishu_setup/csgclaw.py`
