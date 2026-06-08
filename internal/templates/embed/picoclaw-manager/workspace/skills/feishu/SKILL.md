---
name: feishu
description: Configure and troubleshoot CSGClaw Feishu/Lark channel credentials for manager or worker agents. Use when the Manager needs to generate a Feishu bot app creation URL or QR code, collect App ID/App Secret through registration, write and reload channel config through Feishu config API, ensure or recreate agents, or debug Feishu messages not reaching CSGClaw/PicoClaw workers.
---

# Feishu

This skill sets up Feishu/Lark bot app credentials for CSGClaw-managed PicoClaw manager and worker agents.

## Script

Use the bundled script at `/home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py`:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start --bot-id u-dev --role worker --bot-name dev --qr
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

If `start`/`poll` returns a machine-mode `next` command, prefer that absolute command.

## Script roles

- `scripts/feishu_register.py`: User-facing CLI entrypoint. Supports `start`, `poll`, `finalize`, `status`, `recreate-agent`.
- `scripts/feishu_setup/commands.py`: Parses CLI arguments and maps them to handler functions.
- `scripts/feishu_setup/registration.py`: Implements registration flow and device-code polling state transitions.
- `scripts/feishu_setup/csgclaw.py`: Applies config to CSGClaw, triggers reload, and performs participant/agent ensure/recreate actions.
- `scripts/feishu_setup/state.py`: Stores and migrates registration state files.
- `scripts/feishu_setup/config.py`: Defines constants, env-key names, and default path constants.
- `scripts/tests/`: tests and fixtures for script behavior.

The script uses Feishu/Lark's accounts registration flow:

1. `action=init`
2. `action=begin`, with `archetype=PersonalAgent`, `auth_method=client_secret`, and `request_user_info=open_id`
3. return a Feishu/Lark launcher URL, usually under `https://open.feishu.cn/...`; the script appends `from=csgclaw&tp=csgclaw`
4. poll with `action=poll`, `device_code=<...>`, `tp=ob_app`
5. when the user completes app creation, receive `client_id` and `client_secret`
6. map `client_id` -> CSGClaw `app_id`, and `client_secret` -> CSGClaw `app_secret`
7. immediately write the secret to CSGClaw through `PUT /api/v1/channels/feishu/config` without printing it

Do not add or require a public Feishu Open Platform HTTP webhook as the main inbound path. PicoClaw uses Feishu/Lark WebSocket mode for inbound bot messages. CSGClaw's `/api/v1/channels/feishu/participants/{participant}/events` endpoint is an internal SSE bridge for PicoClaw workers, not a Feishu public webhook.

## When to Use

Use this skill when the user asks to:

- create/configure Feishu credentials for the manager agent `u-manager` or a worker agent such as `u-dev`
- generate a Feishu/Lark bot creation URL or QR code
- get Feishu AK/SK, App ID/App Secret, or client_id/client_secret for a CSGClaw-managed agent
- reload CSGClaw channel config after setting Feishu credentials
- recreate a worker or manager after Feishu credentials are configured
- debug why Feishu messages do not reach a CSGClaw/PicoClaw worker

Do not use this skill for generic Feishu webhook integrations or non-CSGClaw Feishu app development.

## Terms

- Target agent ID: usually `u-manager`, `u-dev`, `u-qa`, etc. The helper script still names this legacy argument `--bot-id`.
- Feishu `app_id` / `app_secret`: the Feishu bot application's credentials.
- AK/SK in user wording usually means Feishu `app_id/app_secret` or `client_id/client_secret` returned by the registration flow.
- Manager agent: usually `u-manager`; recreating it can interrupt the current manager skill run.
- Worker agent: any non-manager agent, for example `u-dev`; recreating it is usually safe after config succeeds.

## Prerequisites

1. CSGClaw server is running.
2. Confirm CSGClaw API access is available through environment variables, not command-line token flags:
   - `CSGCLAW_BASE_URL`, default `http://127.0.0.1:18080`
   - `CSGCLAW_ACCESS_TOKEN`, unless server auth is disabled
3. The script is run from the deployed skill directory:
   - inside manager box: typically `~/.picoclaw/workspace/skills/feishu` or your configured skill root
   - host repo path: `internal/templates/embed/picoclaw-manager/workspace/skills/feishu`
4. Server build supports:
   - Feishu config API (`PUT`/`GET`/`POST /api/v1/channels/feishu/config`)
   - `POST /api/v1/channels/feishu/participants`
   - `POST /api/v1/agents/{id}/recreate`

## Safe Credential Rules

1. Never print `app_secret`, `client_secret`, access tokens, verification tokens, encryption keys, or connection strings.
2. If a secret must be represented in examples or summaries, write `[REDACTED]`.
3. The script must print only `app_secret: present` after finalize.
4. Do not store returned `client_secret` in skill state files. `finalize` pipes it directly to `PUT /api/v1/channels/feishu/config`.
5. Verify with `GET /api/v1/channels/feishu/config?bot_id=<id>`, not by printing the secret.

## Choose Target Agent

Ask for the target when it is not explicit.

If the user does not specify an agent in the request, ask: "请明确要对接飞书的目标 Agent 名字（如 `manager`/`u-manager` 或 `dev`/`u-dev`）".
Resolve target:
1. If input is `manager` or `u-manager`, treat as manager flow.
2. Otherwise, treat input as worker flow, set the target agent ID to the input if it already starts with `u-`, otherwise prefix `u-`.
3. If only role was inferred as manager, stop using recreate path and force action-card flow.

Example normalization:
- `dev` -> worker agent `u-dev`, participant `dev`
- `u-dev` -> worker agent `u-dev`, participant `dev`
- `manager` -> manager
- `u-manager` -> manager

For worker flow, check whether the Feishu participant already exists before deciding recreate:
```bash
./csgclaw-cli --output json participant list --channel feishu --type agent
```
Treat a row whose `agent_id` equals the target agent ID as an existing Feishu participant (needs recreate after ensure), and no matching row as missing (skip recreate, let participant ensure create it).

## Primary QR/Launcher Flow

### 1. Start registration and show URL/QR

Run from this skill directory:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start \
  --bot-id <target_agent_id> \
  --role worker \
  --bot-name <worker_name> \
  --description "dev worker agent" \
  --qr
```

Expected output includes:

- `Registration ID: <id>`
- an `https://open.feishu.cn/...` or Lark launcher URL with `from=csgclaw&tp=csgclaw`
- an ASCII QR code if Python package `qrcode` is installed
- the exact finalize command

Send the URL or QR to the user and ask them to open it in Feishu/Lark and confirm app creation.

If `--qr` cannot render a QR code because `qrcode` is not installed, send the printed URL. Do not block setup only because QR rendering is unavailable.

### 2. Poll/finalize after user confirms

After the user clicks the link and completes creation:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

When running `finalize` through the manager's exec tool, always set the tool timeout to at least 600 seconds. Worker setup can create or pull a BoxLite image on first use, and the default tool timeout can interrupt the create flow before CSGClaw persists the worker agent.

By default, `finalize` will:

1. poll Feishu/Lark until credentials are available or timeout
2. receive `client_id/client_secret`
3. write `app_id/app_secret` to CSGClaw through `Feishu config API`
   - for `u-manager`, overwrite global `admin_open_id` only with the registration `open_id`
   - for worker agents, ignore registration `open_id` and do not read, preserve, write, or report `admin_open_id`
4. auto-reload channel config
5. ensure the CSGClaw Feishu participant through `POST /api/v1/channels/feishu/participants`
6. for worker targets, check whether the Feishu participant already existed before ensure using `./csgclaw-cli --output json participant list --channel feishu --type agent`:
   - existing participant: recreate its worker so the new Feishu env takes effect
   - missing participant: let participant ensure create it with the already-reloaded config, then skip redundant recreate
   - if BoxLite reports `box with name '<name>' already exists` while CSGClaw reports `agent "<id>" not found`, stop and tell the user the host has a stale partial worker box; do not keep trying random API paths or host-only commands from inside manager
7. for manager targets, print a `csgclaw.action_card` JSON payload with a whitelisted `rebuild-manager` action; the CSGClaw Web chat message should render the button to complete the window-triggered manager bootstrap replace flow.
8. print JSON with `app_secret: present`, never the real secret

For a worker, default finalize is usually enough:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

Use an exec/tool timeout of at least 600 seconds for this command. Before deciding recreate, use `./csgclaw-cli --output json participant list --channel feishu --type agent`:
 - matching `id`: recreate existing worker
 - no matching `id`: skip recreate, because participant ensure has already created it
If `worker_existed_before_ensure` is `true`, the script recreates the existing worker after config reload; do not create a second worker or change the target agent ID.

For manager, default finalize configures credentials and ensures the participant, then prints a structured action card. Return the JSON object exactly as the chat message content: no leading sentence, no Markdown table, no bullet list, no ```json fence, and no explanatory wrapper. The CSGClaw Web frontend will render a "重建 Manager" button.
The click is handled by the browser and calls the manager bootstrap replace surface (`POST /api/v1/agents` with `{"id":"u-manager","replace":true}`), not the hazardous generic recreate route.

Do not run `python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py recreate-agent --bot-id u-manager` as a terminal self-recreate step anymore. The manager-rebuild action must be completed by clicking the rendered Web window button, which calls `POST /api/v1/agents` with `{"id":"u-manager","replace":true}`.

For manager only, BoxLite status is not a valid post-recreate success check in this skill. The manager gateway starts with `picoclaw gateway -d`, so the launch command can return while the daemonized gateway continues separately; BoxLite may report `stopped` and CSGClaw may show `AVAILABLE=false`. Do not treat that as a reason to recreate manager again from the same manager-hosted run.

### 3. Optional status/poll commands

Check saved state without exposing device_code or secret:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py status --registration-id <id>
```

Check whether user has confirmed yet:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py poll --registration-id <id>
```

`poll` never prints credentials. If credentials are available, use `finalize` to write them immediately to CSGClaw.

## Manual Fallback

If Feishu/Lark registration endpoint fails, expires, or tenant policy blocks scan-to-create, ask the user to create/select an internal bot app manually:

1. Open Feishu/Lark Open Platform.
2. Create or select a self-built/internal app.
3. Enable Bot capability.
4. Publish or enable the app in the tenant as required.
5. Obtain:
   - App ID, usually `cli_...`
   - App Secret, provided only through a secure path.

Use the Feishu config API to set manually:

```bash
curl -sS -X PUT "$CSGCLAW_BASE_URL/api/v1/channels/feishu/config" \
  -H "Authorization: Bearer [REDACTED]" \
  -H "Content-Type: application/json" \
  -d '{"bot_id":"u-dev","app_id":"cli_xxx","app_secret":"[REDACTED]"}'
```

For manager setup, include `admin_open_id`:

```bash
curl -sS -X PUT "$CSGCLAW_BASE_URL/api/v1/channels/feishu/config" \
  -H "Authorization: Bearer [REDACTED]" \
  -H "Content-Type: application/json" \
  -d '{"bot_id":"u-manager","app_id":"cli_xxx","app_secret":"[REDACTED]","admin_open_id":"ou_xxx"}'
```

## CLI Workflow Used by Script

The script writes and reloads Feishu config through `Feishu config API` because sandboxed skills should not edit host files directly or hand-roll config API calls.

For `u-manager`, the script passes the registration `open_id` as the global `admin_open_id` while setting config and auto-reloading:

```bash
curl -sS -X PUT "$CSGCLAW_BASE_URL/api/v1/channels/feishu/config" \
  -H "Authorization: Bearer [REDACTED]" \
  -H "Content-Type: application/json" \
  -d '{"bot_id":"u-manager","app_id":"cli_xxx","app_secret":"[REDACTED]","admin_open_id":"ou_xxx"}'
```

Expected response shape:

```json
{
  "bot_id": "u-manager",
  "configured": true,
  "app_id": "cli_xxx",
  "app_secret": "present",
  "admin_open_id": "ou_xxx",
  "reloaded": true
}
```

Ensure participant:

```bash
csgclaw-cli participant create --type agent --bind create --id dev --agent-id u-dev --name dev --description "dev worker agent" --role worker --channel feishu --channel-user-ref ou_xxx --channel-user-kind open_id --channel-app-ref cli_xxx
```

Recreate existing worker only if `./csgclaw-cli --output json participant list --channel feishu --type agent` already showed a participant bound to `u-dev` before ensure; if the participant was missing, the participant ensure step creates it with the already-reloaded config and this recreate call is skipped:

```bash
curl -sS -X POST "$CSGCLAW_BASE_URL/api/v1/agents/u-dev/recreate" \
  -H "Authorization: Bearer [REDACTED]"
```

## CLI Workflow for Manual Control

Use `Feishu config API` for channel config. Use the helper script or the backend recreate API for agent recreate, because lite `csgclaw-cli` does not expose agent commands and manager boxes usually do not have full `csgclaw`.

```bash
curl -sS "$CSGCLAW_BASE_URL/api/v1/channels/feishu/config?bot_id=u-dev" \
  -H "Authorization: Bearer [REDACTED]"
curl -sS -X POST "$CSGCLAW_BASE_URL/api/v1/channels/feishu/config" \
  -H "Authorization: Bearer [REDACTED]"
csgclaw-cli participant create --type agent --bind create --id dev --agent-id u-dev --name dev --description "dev worker agent" --role worker --channel feishu --channel-user-ref ou_xxx --channel-user-kind open_id --channel-app-ref cli_xxx
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py recreate-agent --bot-id u-dev
```

## Worker One-Shot Recipe

1. Start registration:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start --bot-id <worker_id> --role worker --bot-name <worker_name> --description "<worker_desc>" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

Run the command with exec `timeout` at least `600`.

4. Confirm existing Feishu participant before taking recreate path:

```bash
./csgclaw-cli --output json participant list --channel feishu --type agent
```

If the list contains `<worker_id>`, the manager can trigger recreate flow for this worker after reload.
If the list does not contain `<worker_id>`, skip recreate and let participant ensure creation stand.

5. Tell the user to test from Feishu by messaging or @mentioning the Feishu bot app.

## Manager One-Shot Recipe

Run this recipe from the normal flow and render the manager rebuild action card in the web window.

1. Start registration:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start --bot-id u-manager --role manager --bot-name manager --description "manager agent" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize without recreate:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

4. Return the `finalize` JSON object exactly as the chat response. Do not summarize it, translate it, add a Markdown table, or wrap it in a code fence. The object contains `type: csgclaw.action_card` and action metadata so the Web frontend can render the button.

5. Do not call a manager recreate API or host command from this skill. The Manager rebuild must be completed by the rendered Web action-card button.

Do not use the generic manager recreate endpoint or any terminal/host-side manager rebuild fallback. The Web action card uses `POST /api/v1/agents` with `{"id":"u-manager","replace":true}` from the browser after the user clicks the window button.

## Common Pitfalls

1. Using `csgclaw-cli agent ...`: lite CLI does not have agent commands. Use full `csgclaw` or API.
2. Running host-only `csgclaw` or `boxlite` commands from inside manager: manager usually only has `csgclaw-cli`; use this script/API from manager, and ask the host operator to clean stale BoxLite boxes if needed.
3. Looking for removed `csgclaw-cli bot config ...` commands: use `csgclaw-cli participant config --channel feishu ...`, backed by `/api/v1/channels/feishu/config`.
4. Creating the CSGClaw participant before writing/reloading Feishu config: this can create local placeholder identity.
5. Expecting reload to update an already-running PicoClaw box: recreate is still required.
6. Calling manager recreate from inside this manager-hosted skill: return the action card so the current window renders the rebuild button.
7. Checking `agent list` or `participant list` after manager recreate and treating `stopped` as failure: manager gateway runs in daemon mode, so BoxLite status is not a reliable success signal for this skill.
8. Printing secrets in summaries or logs: always mask as `[REDACTED]` or `present`.
9. Calling CSGClaw SSE endpoint a Feishu webhook: it is an internal CSGClaw-to-PicoClaw bridge.
10. If Feishu changes the accounts registration endpoint or tenant policy blocks PersonalAgent creation, fall back to manual App ID/App Secret setup.

## Verification Checklist

- [ ] `start` printed a launcher URL or QR code for the user.
- [ ] `finalize` output shows `app_secret` only as `present`.
- [ ] `finalize` configured the target agent ID (`bot_id` field) and `app_id` in CSGClaw.
- [ ] CSGClaw channel config was reloaded.
- [ ] CSGClaw participant exists with `channel=feishu`.
- [ ] Existing worker agents are recreated after config reload.
- [ ] New worker finalize was run with a tool timeout of at least 600 seconds.
- [ ] Manager finalize returned a raw `csgclaw.action_card` JSON object with `rebuild-manager` action metadata for the web button.
- [ ] No manager-hosted command called the generic manager recreate endpoint or any host-side manager rebuild command.
- [ ] No public Feishu webhook endpoint was added or required.
