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

## Notification bots

Notification bots are channel bots with `type=notification`. They do not create backing worker agents; delivery configuration is stored on `bot.runtime_options` in `bots.json`. Default bot id is `n-{name}` (separate from worker agent ids `u-{name}`); you may set `id` explicitly, but it must not collide with an existing agent or channel bot.

- List: `GET /api/v1/channels/csgclaw/bots` (includes `type=notification` bots; feishu bot list excludes them)
- Create: `POST /api/v1/channels/csgclaw/bots` with `"type":"notification"` and flat `runtime_options` (`delivery_mode`, `webhook_token`, `remote_url`, …)
- Update: `PATCH /api/v1/channels/csgclaw/bots/{id}`
- Delete: `DELETE /api/v1/channels/csgclaw/bots/{id}`
- Push (webhook): `POST /api/v1/channels/csgclaw/bots/{id}/notifications` with `Authorization: Bearer <webhook_token>`

Implementation: `internal/channel/csgclaw/notification_bot/`, `internal/bot/notification.go`.

## csgclaw.notify_card payload

Notification deliveries (GitLab/GitHub webhooks, and so on) to the CSGClaw Web IM use this type: the message **`content` is a single JSON object** produced by `internal/channel/csgclaw/notification_bot`, and the Web UI renders it as a structured card (title, badge, meta rows, optional link, optional collapsible raw JSON).

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

### Field notes

- `type`: must be `csgclaw.notify_card`.
- `schema_version`: currently `1`.
- `provider`: `gitlab`, `github`, or `generic`.
- `event`: normalized name (for example `push`, `merge_request`, `issue`, `pull_request`, `ping`); for `generic` payloads values such as `json`, `text`, or `empty` are used.
- `title` / `subtitle` / `badge` / `summary`: display fields.
- `link`: optional HTTP(S) URL; the UI only allows `http:` and `https:` schemes.
- `meta`: optional list of `{ "label", "value" }` rows.
- `raw`: optional truncated pretty JSON when the webhook shape is unknown.
- Like `action_card`, **`content` must be the raw JSON object only** (no markdown wrapper or code fence).

### Related code paths

- Frontend parser/renderer: `web/app/src/components/business/MessageContent/MessageContent.tsx`, `web/app/src/components/business/MessageContent/structuredMessages.ts`
- Action-card and notifier-card test coverage: `web/app/tests/legacy-contract.test.ts`, `web/app/tests/components/MessageContent/structuredMessages.test.ts`
- Notification card encoding: `internal/channel/csgclaw/notification_bot/notify_card.go`, `internal/channel/csgclaw/notification_bot/notify_webhooks.go`
- Feishu setup command output: `internal/templates/embed/runtimes/picoclaw/manager/workspace/skills/feishu/scripts/feishu_setup/csgclaw.py`
