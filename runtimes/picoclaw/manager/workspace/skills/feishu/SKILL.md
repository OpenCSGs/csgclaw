---
name: feishu
description: Configure and troubleshoot CSGClaw Feishu/Lark channel credentials for manager or worker bots. Use when the Manager needs to generate a bot creation URL or QR code, collect App ID/App Secret through registration, write and reload channel config through csgclaw-cli bot config, ensure or recreate agents, or debug Feishu messages not reaching CSGClaw/PicoClaw bots.
---

# Feishu

This skill sets up Feishu/Lark bot credentials for CSGClaw-managed PicoClaw manager and worker bots.

The primary automation is the local script:

```bash
cd ~/.picoclaw/workspace/skills/feishu
python scripts/feishu_register.py start --bot-id u-dev --role worker --bot-name dev --qr
python scripts/feishu_register.py finalize --registration-id <id>
```

The script uses Feishu/Lark's accounts registration flow:

1. `action=init`
2. `action=begin`, with `archetype=PersonalAgent`, `auth_method=client_secret`, and `request_user_info=open_id`
3. return a Feishu/Lark launcher URL, usually under `https://open.feishu.cn/...`; the script appends `from=csgclaw&tp=csgclaw`
4. poll with `action=poll`, `device_code=<...>`, `tp=ob_app`
5. when the user completes app creation, receive `client_id` and `client_secret`
6. map `client_id` -> CSGClaw `app_id`, and `client_secret` -> CSGClaw `app_secret`
7. immediately write the secret to CSGClaw through `csgclaw-cli bot config --channel feishu --set` without printing it

Do not add or require a public Feishu Open Platform HTTP webhook as the main inbound path. PicoClaw uses Feishu/Lark WebSocket mode for inbound bot messages. CSGClaw's `/api/v1/channels/feishu/bots/{bot}/events` endpoint is an internal SSE bridge for PicoClaw workers, not a Feishu public webhook.

## When to Use

Use this skill when the user asks to:

- create/configure Feishu for `u-manager` or a worker such as `u-dev`
- generate a Feishu/Lark bot creation URL or QR code
- get Feishu AK/SK, App ID/App Secret, or client_id/client_secret for a CSGClaw bot
- reload CSGClaw channel config after setting Feishu credentials
- recreate a worker or manager after Feishu credentials are configured
- debug why Feishu messages do not reach a CSGClaw/PicoClaw bot

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
3. The script is run from this skill directory:
   - inside manager box: `~/.picoclaw/workspace/skills/feishu`
   - host repo path: `runtimes/picoclaw/manager/workspace/skills/feishu`
4. Server build supports:
   - `csgclaw-cli bot config --channel feishu --set/--get/--reload`
   - `POST /api/v1/bots`
   - `POST /api/v1/agents/{id}/recreate`

## Safe Credential Rules

1. Never print `app_secret`, `client_secret`, access tokens, verification tokens, encryption keys, or connection strings.
2. If a secret must be represented in examples or summaries, write `[REDACTED]`.
3. The script must print only `app_secret: present` after finalize.
4. Do not store returned `client_secret` in skill state files. `finalize` pipes it directly to `csgclaw-cli bot config --channel feishu --set --app-secret-stdin`.
5. Verify with `csgclaw-cli bot config --channel feishu --get`, not by printing the secret.

## Choose Target Bot

Ask or infer the target:

- Manager setup:
  - `bot_id = u-manager`
  - `role = manager`
  - manager recreate is risky and must be the final step after explicit confirmation.
  - after the confirmed recreate request returns, stop immediately; do not check manager runtime status or retry based on `stopped`.
- Worker setup:
  - `bot_id = u-{name}`, for example `u-dev`
  - `role = worker`
  - after credentials are configured and reloaded, an existing worker is recreated; a missing worker is created by `POST /api/v1/bots` and is not redundantly recreated.

If the user says "dev 飞书机器人", use `u-dev` unless they specify another ID.

## Primary QR/Launcher Flow

### 1. Start registration and show URL/QR

Run from this skill directory:

```bash
python scripts/feishu_register.py start \
  --bot-id u-dev \
  --role worker \
  --bot-name dev \
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
python scripts/feishu_register.py finalize --registration-id <id>
```

When running `finalize` through the manager's exec tool, always set the tool timeout to at least 600 seconds. Worker setup can create or pull a BoxLite image on first use, and the default tool timeout can interrupt the create flow before CSGClaw persists the worker agent.

By default, `finalize` will:

1. poll Feishu/Lark until credentials are available or timeout
2. receive `client_id/client_secret`
3. write `app_id/app_secret` to CSGClaw through `csgclaw-cli bot config`
   - if global `admin_open_id` already exists, preserve it
   - if global `admin_open_id` is missing, fill it from explicit `--admin-open-id` or the registration `open_id`
4. auto-reload channel config
5. ensure the CSGClaw bot through `POST /api/v1/bots`
6. for worker targets, check whether the worker agent already existed before ensure:
   - existing worker: recreate it so the new Feishu env takes effect
   - missing worker: let `POST /api/v1/bots` create it with the already-reloaded config, then skip redundant recreate
   - if BoxLite reports `box with name '<name>' already exists` while CSGClaw reports `agent "<id>" not found`, stop and tell the user the host has a stale partial worker box; do not keep trying random API paths or host-only commands from inside manager
7. skip manager recreate unless explicitly requested with final confirmation
8. print JSON with `app_secret: present`, never the real secret

For a worker, default finalize is usually enough:

```bash
python scripts/feishu_register.py finalize --registration-id <id>
```

Use an exec/tool timeout of at least 600 seconds for this command. If `worker_existed_before_ensure` is `true`, the script recreates the existing worker after config reload; do not create a second worker or change the bot id.

For manager, default finalize configures and ensures the bot, but skips manager recreate. Warn the user first. If the user explicitly confirms that interrupting the current manager is acceptable, run the standalone recreate command as the final action:

```bash
python scripts/feishu_register.py recreate-agent --bot-id u-manager --confirm-manager
```

Treat `status: recreate_requested` or an HTTP success response from this command as terminal success for this skill. Do not expect to continue the current manager-hosted skill after manager recreate. Do not run `csgclaw agent list`, `csgclaw bot list`, `csgclaw-cli bot config --channel feishu --get`, `/api/v1/agents/u-manager/start`, `/api/v1/agents/u-manager/recreate`, or any manager status polling after this command.

For manager only, BoxLite status is not a valid post-recreate success check in this skill. The manager gateway starts with `picoclaw gateway -d`, so the launch command can return while the daemonized gateway continues separately; BoxLite may report `stopped` and CSGClaw may show `AVAILABLE=false`. Do not treat that as a reason to recreate manager again from the same manager-hosted run.

### 3. Optional status/poll commands

Check saved state without exposing device_code or secret:

```bash
python scripts/feishu_register.py status --registration-id <id>
```

Check whether user has confirmed yet:

```bash
python scripts/feishu_register.py poll --registration-id <id>
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

Set config and auto-reload:

```bash
printf '%s' '[REDACTED]' | csgclaw-cli --output json bot config --channel feishu --set \
  --bot-id u-dev \
  --app-id cli_xxx \
  --admin-open-id ou_xxx \
  --app-secret-stdin
```

Expected response shape:

```json
{
  "bot_id": "u-dev",
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

Recreate existing worker only if `GET /api/v1/agents/u-dev` returned an existing worker before ensure; if the worker was missing, the bot ensure step creates it with the already-reloaded config and this recreate call is skipped:

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
python scripts/feishu_register.py recreate-agent --bot-id u-dev
```

## Worker One-Shot Recipe

1. Start registration:

```bash
python scripts/feishu_register.py start --bot-id u-dev --role worker --bot-name dev --description "dev worker agent" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize:

```bash
python scripts/feishu_register.py finalize --registration-id <id>
```

Run the command with exec `timeout` at least `600`.

4. Tell the user to test from Feishu by messaging or @mentioning the bot.

## Manager One-Shot Recipe

1. Start registration:

```bash
python scripts/feishu_register.py start --bot-id u-manager --role manager --bot-name manager --description "manager agent" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize without recreate:

```bash
python scripts/feishu_register.py finalize --registration-id <id>
```

4. Warn the user:

"Manager config is set and reloaded. Recreating `u-manager` is required for the PicoClaw runtime env to refresh, but it may interrupt this current task. Confirm before running the recreate step."

5. If confirmed, run as the final action:

```bash
python scripts/feishu_register.py recreate-agent --bot-id u-manager --confirm-manager
```

Stop after this command succeeds. Do not inspect manager agent or bot status afterward; the caller is the manager being recreated, and `stopped` can be a BoxLite daemon-mode artifact rather than a Feishu setup failure.

Alternative backend API recreate:

```bash
curl -sS -X POST "$CSGCLAW_BASE_URL/api/v1/agents/u-manager/recreate" \
  -H "Authorization: Bearer [REDACTED]"
```

## Common Pitfalls

1. Using `csgclaw-cli agent ...`: lite CLI does not have agent commands. Use full `csgclaw` or API.
2. Running host-only `csgclaw` or `boxlite` commands from inside manager: manager usually only has `csgclaw-cli`; use this script/API from manager, and ask the host operator to clean stale BoxLite boxes if needed.
3. Looking for removed `csgclaw channel ...` commands: Feishu config belongs to `csgclaw-cli bot config --channel feishu`.
4. Creating the CSGClaw bot before writing/reloading Feishu config: this can create local placeholder identity.
5. Expecting reload to update an already-running PicoClaw box: recreate is still required.
6. Recreating manager in the middle of a skill: this can terminate the current task.
7. Checking `agent list` or `bot list` after manager recreate and treating `stopped` as failure: manager gateway runs in daemon mode, so BoxLite status is not a reliable success signal for this skill.
8. Printing secrets in summaries or logs: always mask as `[REDACTED]` or `present`.
9. Calling CSGClaw SSE endpoint a Feishu webhook: it is an internal CSGClaw-to-PicoClaw bridge.
10. If Feishu changes the accounts registration endpoint or tenant policy blocks PersonalAgent creation, fall back to manual App ID/App Secret setup.

## Verification Checklist

- [ ] `start` printed a launcher URL or QR code for the user.
- [ ] `finalize` output shows `app_secret` only as `present`.
- [ ] `finalize` configured `bot_id` and `app_id` in CSGClaw.
- [ ] CSGClaw channel config was reloaded.
- [ ] CSGClaw bot exists with `channel=feishu`.
- [ ] Existing worker agents are recreated after config reload.
- [ ] New worker finalize was run with a tool timeout of at least 600 seconds.
- [ ] Manager recreate is deferred until explicit final confirmation.
- [ ] Confirmed manager recreate is treated as terminal; no immediate manager status check is run.
- [ ] No public Feishu webhook endpoint was added or required.
