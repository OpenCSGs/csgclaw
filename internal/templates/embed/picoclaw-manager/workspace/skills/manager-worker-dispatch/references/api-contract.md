# CSGClaw API Contract

This skill follows the actual routes described in `docs/api.md`.

The skill and CLI use `room` as the user-facing term. Where the underlying HTTP API still uses a legacy field name, the contract notes the mapping explicitly.

## Environment Variables

- `CSGCLAW_BASE_URL`: Preferred when the script runs inside a CSGClaw box.
- `CSGCLAW_ACCESS_TOKEN`: Preferred bearer token when the script runs inside a CSGClaw box.
- `MANAGER_API_BASE_URL`: Optional. Default: `http://127.0.0.1:18080`
- `MANAGER_API_TOKEN`: Optional bearer token. Required for `/api/bots/*` when the server enables auth.
- `MANAGER_API_TIMEOUT`: Optional request timeout in seconds. Default: `30`

## Local Config

When available, load the CSGClaw API settings from `~/.picoclaw/config.json`:

- `channels.csgclaw.base_url`
- `channels.csgclaw.access_token`

## Expected Endpoints

### Dispatch task by bot message

- Method: `POST`
- Path: `/api/bots/{bot_id}/messages/send`
- Request body:

```json
{
  "room_id": "room-123",
  "text": "<at user_id=\"u-bob\">bob</at> 你来写前端代码，实现设置页 UI"
}
```

### Read IM bootstrap

- Method: `GET`
- Path: `/api/v1/bootstrap`
- Purpose: resolve room members and assignee handles for tracker sequencing

### Read room message history

- Method: `GET`
- Path: `/api/v1/messages?room_id={room_id}`
- Purpose: inspect prior tracker dispatches and assignee room replies

## Notes

- There is no dedicated task-assignment API.
- Dispatch still means sending a normal bot message in the target room and mentioning the worker.
- Each task in `todo.json` should carry an `id` task number, increasing in dispatch order such as `1`, `2`, `3`.
- `start-tracking` watches `todo.json`, room history, and IM bootstrap data.
- The first task dispatches immediately. Later tasks dispatch only after the previous task both:
  updates `passes` to `true`, and
  receives a normal in-room reply from that assignee after the tracker dispatch message.
- Tool trace messages that start with `🔧` do not count as completion replies.
- The tracker resolves each gated assignee against real room member handles; unresolved assignees are treated as tracker errors, not silent skips.
- While tracking is active, the tracker is the only sequencer. Manager/worker prose should not manually assign the next worker.
- Worker provisioning and room membership remain explicit steps through `list-workers`, `create-worker`, and `join-worker`.
