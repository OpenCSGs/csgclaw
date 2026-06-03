---
name: feishu
description: Configure and troubleshoot CSGClaw Feishu/Lark channel credentials for manager or worker bots. Use when the Manager needs to generate a bot creation URL or QR code, collect App ID/App Secret through registration, write and reload channel config through csgclaw-cli bot config, ensure or recreate agents, or debug Feishu messages not reaching CSGClaw/OpenClaw bots.
---

# Feishu

This skill sets up Feishu/Lark bot credentials for CSGClaw-managed OpenClaw manager and worker bots.

## Script

Use the bundled script at `/home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py`:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py start --bot-id u-dev --role worker --bot-name dev --qr
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

If `start`/`poll` returns a machine-mode `next` command, prefer that absolute command.

## Script roles

- `scripts/feishu_register.py`: User-facing CLI entrypoint. Supports `start`, `poll`, `finalize`, `status`, `recreate-agent`.
- `scripts/feishu_setup/commands.py`: Parses CLI arguments and maps them to handler functions.
- `scripts/feishu_setup/registration.py`: Implements registration flow and device-code polling state transitions.
- `scripts/feishu_setup/csgclaw.py`: Applies config to CSGClaw, triggers reload, and performs bot/agent ensure/recreate actions.
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
7. immediately write the secret to CSGClaw through `csgclaw-cli bot config --channel feishu --set` without printing it

Do not add or require a public Feishu Open Platform HTTP webhook as the main inbound path. OpenClaw uses Feishu/Lark WebSocket mode for real inbound bot messages. CSGClaw's `/api/v1/channels/feishu/bots/{bot}/events` endpoint is an internal SSE bridge for CSGClaw manager-to-worker dispatch, not a Feishu public webhook.

## When to Use

Use this skill when the user asks to:

- create/configure Feishu for `u-manager` or a worker such as `u-dev`
- generate a Feishu/Lark bot creation URL or QR code
- get Feishu AK/SK, App ID/App Secret, or client_id/client_secret for a CSGClaw bot
- reload CSGClaw channel config after setting Feishu credentials
- recreate a worker or manager after Feishu credentials are configured
- debug why Feishu messages do not reach a CSGClaw/OpenClaw bot

Do not use this skill for generic Feishu webhook integrations or non-CSGClaw Feishu app development.

## Terms

- CSGClaw bot ID: usually `u-manager`, `u-dev`, `u-qa`, etc.
- Feishu `app_id` / `app_secret`: the Feishu bot application's credentials.
- AK/SK in user wording usually means Feishu `app_id/app_secret` or `client_id/client_secret` returned by the registration flow.
- Manager agent: usually `u-manager`; recreating it can interrupt the current manager skill run.
- Worker agent: any non-manager bot, for example `u-dev`; recreating it is usually safe after config succeeds.

## Prerequisites

1. CSGClaw server is running.
2. Confirm CSGClaw API access is available through environment variables, not command-line token flags:
   - `CSGCLAW_BASE_URL`, default `http://127.0.0.1:18080`
   - `CSGCLAW_ACCESS_TOKEN`, unless server auth is disabled
3. The script is run from the deployed skill directory:
   - inside manager box: typically `~/.openclaw/workspace/skills/feishu` or your configured skill root
   - host repo path: `internal/templates/embed/openclaw-manager/workspace/skills/feishu`
4. Server build supports:
   - `csgclaw-cli bot config --channel feishu --set/--get/--reload`
   - `POST /api/v1/channels/feishu/bots`
   - `POST /api/v1/agents/{id}/recreate`

## Manager Group Permissions

CSGClaw cannot silently grant Feishu/Lark app scopes from inside the OpenClaw runtime. Feishu group operations use the manager bot's Feishu app credentials, so the tenant admin must approve the required scopes in Feishu/Lark Open Platform.

For new Feishu groups, after the manager and worker Feishu configs exist, prefer creating the group with all bot IDs already included:

```bash
csgclaw-cli room create --title dev-ui-group --creator-id u-manager --member-ids u-manager,u-dev --channel feishu
```

This uses Feishu `bot_id_list` during chat creation and avoids the separate `member create` path for new groups.

For existing Feishu groups, `csgclaw-cli member list` and `member create` require manager app scopes such as:

- `im:chat:read`
- `im:chat.members:read`
- `im:chat.members:write_only`
- or the broader `im:chat`

`finalize` prints `manager_group_scopes` and `manager_group_permission_url`. Send that URL to the user/admin when Feishu returns `Access denied` for group member inspection or adding a worker bot to an existing group.

## Safe Credential Rules

1. Never print `app_secret`, `client_secret`, access tokens, verification tokens, encryption keys, or connection strings.
2. If a secret must be represented in examples or summaries, write `[REDACTED]`.
3. The script must print only `app_secret: present` after finalize.
4. Do not store returned `client_secret` in skill state files. `finalize` pipes it directly to `csgclaw-cli bot config --channel feishu --set --app-secret-stdin`.
5. Verify with `csgclaw-cli bot config --channel feishu --get`, not by printing the secret.

## Choose Target Bot

Ask for the target when it is not explicit.

If the user does not specify an agent in the request, ask: "请明确要对接飞书的目标 Agent 名字（如 `manager`/`u-manager` 或 `dev`/`u-dev`）".
Resolve target:
1. If input is `manager` or `u-manager`, treat as manager flow.
2. Otherwise, treat input as worker flow, set `bot_id` to the input if it already starts with `u-`, otherwise prefix `u-`.
3. If only role was inferred as manager, stop using recreate path and force action-card flow.

Example normalization:
- `dev` -> worker `u-dev`
- `u-dev` -> worker `u-dev`
- `manager` -> manager
- `u-manager` -> manager

For worker flow, finalize writes config, reloads Feishu channel config, ensures the CSGClaw bot, then recreates the worker so runtime env/files are materialized from the updated config.

## Primary QR/Launcher Flow

### 1. Start registration and show URL/QR

Run from this skill directory:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py start \
  --bot-id <worker_id> \
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
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

When running `finalize` through the manager's exec tool, always set the tool timeout to at least 600 seconds. Worker setup can create or pull a BoxLite image on first use, and the default tool timeout can interrupt the create flow before CSGClaw persists the worker agent.

By default, `finalize` will:

1. poll Feishu/Lark until credentials are available or timeout
2. receive `client_id/client_secret`
3. write `app_id/app_secret` to CSGClaw through `csgclaw-cli bot config`
   - for `u-manager`, overwrite global `admin_open_id` only with the registration `open_id`
   - for worker bots, ignore registration `open_id` and do not read, preserve, write, or report `admin_open_id`
4. auto-reload channel config
5. ensure the CSGClaw bot through `POST /api/v1/channels/feishu/bots`
6. for worker targets, recreate the worker after bot ensure so the new Feishu env/files take effect
   - if BoxLite reports `box with name '<name>' already exists` while CSGClaw reports `agent "<id>" not found`, stop and tell the user the host has a stale partial worker box; do not keep trying random API paths or host-only commands from inside manager
7. for manager targets, print a `csgclaw.action_card` JSON payload with a whitelisted `rebuild-manager` action; the CSGClaw Web chat message should render the button to complete the window-triggered manager bootstrap replace flow.
8. print JSON with `app_secret: present`, never the real secret

For a worker, default finalize is usually enough:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

Use an exec/tool timeout of at least 600 seconds for this command. For workers, finalize recreates the target worker after config reload and bot ensure; do not create a second worker or change the bot id.

For manager, default finalize configures and ensures the bot, then prints a structured action card. Return the JSON object exactly as the chat message content: no leading sentence, no Markdown table, no bullet list, no ```json fence, and no explanatory wrapper. The CSGClaw Web frontend will render a "重建 Manager" button.
The click is handled by the browser and calls the manager bootstrap replace surface (`POST /api/v1/agents` with `{"id":"u-manager","replace":true}`), not the hazardous generic recreate route.

Do not run `python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py recreate-agent --bot-id u-manager` as a terminal self-recreate step anymore. The manager-rebuild action must be completed by clicking the rendered Web window button, which calls `POST /api/v1/agents` with `{"id":"u-manager","replace":true}`.

For manager only, BoxLite status is not a valid post-recreate success check in this skill. The OpenClaw gateway is managed by the runtime wrapper, so BoxLite may report a transient lifecycle state while CSGClaw is replacing the manager. Do not treat that as a reason to recreate manager again from the same manager-hosted run.

### 3. Optional status/poll commands

Check saved state without exposing device_code or secret:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py status --registration-id <id>
```

Check whether user has confirmed yet:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py poll --registration-id <id>
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

Use `csgclaw-cli bot config` to set manually:

```bash
printf '%s' '[REDACTED]' | csgclaw-cli bot config --channel feishu --set \
  --bot-id u-dev \
  --app-id cli_xxx \
  --app-secret-stdin
```

or:

```bash
csgclaw-cli bot config --channel feishu --set \
  --bot-id u-dev \
  --app-id cli_xxx \
  --app-secret-file /secure/path/feishu_app_secret
```

## CLI Workflow Used by Script

The script writes and reloads Feishu config through `csgclaw-cli bot config` because sandboxed skills should not edit host files directly or hand-roll config API calls.

For `u-manager`, the script passes the registration `open_id` as the global `admin_open_id` while setting config and auto-reloading:

```bash
printf '%s' '[REDACTED]' | csgclaw-cli --output json bot config --channel feishu --set \
  --bot-id u-manager \
  --app-id cli_xxx \
  --admin-open-id ou_xxx \
  --app-secret-stdin
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

Ensure bot:

```bash
csgclaw-cli bot create --id u-dev --name dev --description "dev worker agent" --role worker --channel feishu
```

Recreate the worker after config reload and bot ensure so the runtime picks up the updated Feishu credentials:

```bash
curl -sS -X POST "$CSGCLAW_BASE_URL/api/v1/agents/u-dev/recreate" \
  -H "Authorization: Bearer [REDACTED]"
```

## CLI Workflow for Manual Control

Use `csgclaw-cli bot config` for channel config. Use the helper script or the backend recreate API for agent recreate, because lite `csgclaw-cli` does not expose agent commands and manager boxes usually do not have full `csgclaw`.

```bash
csgclaw-cli bot config --channel feishu --get --bot-id u-dev
csgclaw-cli bot config --channel feishu --reload
csgclaw-cli bot create --id u-dev --name dev --description "dev worker agent" --role worker --channel feishu
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py recreate-agent --bot-id u-dev
```

## Worker One-Shot Recipe

1. Start registration:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py start --bot-id <worker_id> --role worker --bot-name <worker_name> --description "<worker_desc>" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

Run the command with exec `timeout` at least `600`.

4. Confirm finalize recreated the worker after reload and bot ensure.
5. Tell the user to test from Feishu by messaging or @mentioning the bot.

## Manager One-Shot Recipe

Run this recipe from the normal flow and render the manager rebuild action card in the web window.

1. Start registration:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py start --bot-id u-manager --role manager --bot-name manager --description "manager agent" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize without recreate:

```bash
python /home/node/.openclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

4. Return the `finalize` JSON object exactly as the chat response. Do not summarize it, translate it, add a Markdown table, or wrap it in a code fence. The object contains `type: csgclaw.action_card` and action metadata so the Web frontend can render the button.

5. Do not call a manager recreate API or host command from this skill. The Manager rebuild must be completed by the rendered Web action-card button.

Do not use the generic manager recreate endpoint or any terminal/host-side manager rebuild fallback. The Web action card uses `POST /api/v1/agents` with `{"id":"u-manager","replace":true}` from the browser after the user clicks the window button.

## Common Pitfalls

1. Using `csgclaw-cli agent ...`: lite CLI does not have agent commands. Use full `csgclaw` or API.
2. Running host-only `csgclaw` or `boxlite` commands from inside manager: manager usually only has `csgclaw-cli`; use this script/API from manager, and ask the host operator to clean stale BoxLite boxes if needed.
3. Looking for removed `csgclaw channel ...` commands: Feishu config belongs to `csgclaw-cli bot config --channel feishu`.
4. Creating the CSGClaw bot before writing/reloading Feishu config: this can create local placeholder identity.
5. Expecting reload to update an already-running OpenClaw box: recreate is still required.
6. Calling manager recreate from inside this manager-hosted skill: return the action card so the current window renders the rebuild button.
7. Checking `agent list` or `bot list` after manager recreate and treating `stopped` as failure: manager gateway runs in daemon mode, so BoxLite status is not a reliable success signal for this skill.
8. Printing secrets in summaries or logs: always mask as `[REDACTED]` or `present`.
9. Calling CSGClaw SSE endpoint a Feishu webhook: it is an internal CSGClaw-to-runtime bridge.
10. If Feishu changes the accounts registration endpoint or tenant policy blocks PersonalAgent creation, fall back to manual App ID/App Secret setup.

## Verification Checklist

- [ ] `start` printed a launcher URL or QR code for the user.
- [ ] `finalize` output shows `app_secret` only as `present`.
- [ ] `finalize` configured `bot_id` and `app_id` in CSGClaw.
- [ ] CSGClaw channel config was reloaded.
- [ ] CSGClaw bot exists with `channel=feishu`.
- [ ] Worker agents are recreated after config reload and bot ensure.
- [ ] New worker finalize was run with a tool timeout of at least 600 seconds.
- [ ] Manager finalize returned a raw `csgclaw.action_card` JSON object with `rebuild-manager` action metadata for the web button.
- [ ] No manager-hosted command called the generic manager recreate endpoint or any host-side manager rebuild command.
- [ ] No public Feishu webhook endpoint was added or required.
