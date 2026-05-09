# CSGClaw Channel Integration

English | [中文](csgclaw.zh.md)

This document describes CSGClaw channel-side interaction contracts used by frontend and runtime automation. It supplements the channel-specific documents (for example `docs/channel/feishu.md`).

## csgclaw.action_card payload

Some flows (notably Feishu manager setup) return a structured card object in the final message instead of plain text. The frontend recognizes this format and renders an interactive button.

```json
{
  "type": "csgclaw.action_card",
  "status": "manager_recreate_pending",
  "bot_id": "u-manager",
  "title": "Manager Feishu setup completed",
  "subtitle": "u-manager",
  "badge": "Click in window",
  "summary": "Feishu config has been written and reloaded. Manager must be rebuilt before the new configuration is injected into runtime. Click the button below to trigger a secure Manager bootstrap replace in the browser.",
  "actions": [
    {
      "id": "rebuild-manager",
      "label": "Rebuild Manager",
      "style": "danger",
      "method": "manager-bootstrap-replace",
      "confirm": "Rebuilding Manager will interrupt the current Manager session; the chat may need refresh. Continue?"
    }
  ]
}
```

### Required behavior

- `type` must be exactly `csgclaw.action_card`.
- `actions[0].id` must be `rebuild-manager`.
- `actions[0].method` must be `manager-bootstrap-replace`.
- Frontend must render this payload directly as the complete chat content (no prose, no markdown table, no markdown code fence).

### Manager rebuild execution contract

The `rebuild-manager` action must be executed in the browser/web UI by calling:

```bash
POST /api/v1/agents
Content-Type: application/json

{"id":"u-manager","replace":true}
```

Do not call `POST /api/v1/agents/u-manager/recreate` for this flow.

### Security notes

- Never return or log secret values (for example `app_secret`, API keys, tokens).
- If any sensitive value appears in logs, use masked forms such as `present`.

### Related code paths

- Frontend parser/renderer: `web/static/app.js`
- Action-card test coverage: `web/static/app_action_card.test.cjs`
- Feishu setup command output: `runtimes/picoclaw/manager/workspace/skills/feishu/scripts/feishu_setup/csgclaw.py`
