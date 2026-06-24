---
name: feishu
description: Configure and troubleshoot CSGClaw Feishu/Lark channel credentials for manager or worker agents. Use when the Manager needs to generate a Feishu bot app creation URL or QR code, collect App ID/App Secret through registration, bind Feishu participants through `csgclaw-cli participant bind`, recreate workers, or debug Feishu messages not reaching CSGClaw/PicoClaw workers.
---

# Feishu

This skill sets up Feishu/Lark bot app credentials for CSGClaw-managed PicoClaw manager and worker agents.

## Script

Use the bundled script at `/home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py`:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start --agent u-dev --role worker --bot-name dev --qr
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

If `start`/`poll` returns a machine-mode `next` command, prefer that absolute command.

## Script roles

- `scripts/feishu_register.py`: User-facing CLI entrypoint. Supports `start`, `poll`, `finalize`, `status`, `recreate-agent`, `bind-manager`.
- `scripts/feishu_setup/commands.py`: Parses CLI arguments and maps them to handler functions.
- `scripts/feishu_setup/registration.py`: Implements registration flow and device-code polling state transitions.
- `scripts/feishu_setup/csgclaw.py`: Applies config to CSGClaw through `participant bind` and returns the manager action card when needed.
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
7. immediately pipe the secret to `csgclaw-cli participant bind --feishu-kind bot --app-secret-stdin` without printing it

Do not add or require a public Feishu Open Platform HTTP webhook as the main inbound path. PicoClaw uses Feishu/Lark WebSocket mode for inbound bot messages. CSGClaw's `/api/v1/channels/feishu/participants/{participant}/events` endpoint is an internal SSE bridge for PicoClaw workers, not a Feishu public webhook.

## When to Use

Use this skill when the user asks to:

- create/configure Feishu credentials for the manager agent `u-manager` or a worker agent such as `u-dev`
- generate a Feishu/Lark bot creation URL or QR code
- get Feishu AK/SK, App ID/App Secret, or client_id/client_secret for a CSGClaw-managed agent
- bind Feishu participant config after setting Feishu credentials
- recreate a worker or manager after Feishu credentials are configured
- debug why Feishu messages do not reach a CSGClaw/PicoClaw worker

Do not use this skill for generic Feishu webhook integrations or non-CSGClaw Feishu app development.

## Terms

- Target agent ID: usually `u-manager`, `u-dev`, `u-qa`, etc. Pass it to the helper script with `--agent`.
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
   - host repo path: `internal/hub/templates/manager/picoclaw/workspace/skills/feishu`
4. Server build supports:
   - `csgclaw-cli participant bind`
   - `POST /api/v1/channels/feishu/participants`
   - `POST /api/v1/agents/{id}/recreate`

## Manager Group Permissions

CSGClaw cannot silently grant Feishu/Lark app scopes from inside the PicoClaw runtime. Feishu group operations use the manager agent's Feishu bot app credentials, so the tenant admin must approve the required scopes in Feishu/Lark Open Platform.

For new Feishu groups, after the manager and worker Feishu configs exist, prefer creating the group with all participant IDs already included:

```bash
csgclaw-cli room create --title dev-ui-group --creator-id manager --member-ids manager,dev --channel feishu
```

CSGClaw creates the Feishu chat first, then resolves those participant IDs to configured Feishu app credentials and invites the worker bot apps. This keeps the created `chat_id` visible if the invite fails, but it still requires manager app group scopes for chat creation and member invites.

For Feishu group operations, `room create --member-ids`, `csgclaw-cli member list`, and `member create` require manager app scopes such as:

- `im:chat:read`
- `im:chat.members:read`
- `im:chat.members:write_only`
- or the broader `im:chat`

`finalize` prints `manager_group_scopes` and `manager_group_permission_url`. Send that URL to the user/admin when Feishu returns `Access denied` for group member inspection or adding a worker agent's Feishu bot app to an existing group.

## Safe Credential Rules

1. Never print `app_secret`, `client_secret`, access tokens, verification tokens, encryption keys, or connection strings.
2. If a secret must be represented in examples or summaries, write `[REDACTED]`.
3. The script must print only `app_secret: present` after finalize.
4. Do not store returned `client_secret` in skill state files. `finalize` pipes it directly to `csgclaw-cli participant bind --app-secret-stdin`.
5. Verify with `csgclaw-cli participant list --channel feishu` and check the `channel_app_config.app_id` you configured; keep `app_secret` masked.

## Choose Target Agent

Ask for the target when it is not explicit.

If the user asks to **create/provision/add a new worker and connect it to Feishu** in one request, do this as a two-phase workflow:

1. Use `agent-creator` first to create the worker. That skill must run `hub list`, `hub get`, then `csgclaw-cli participant create --type agent --bind create --from-template ...`.
2. Only after the worker agent exists, return to this Feishu skill and run the QR/manual credential flow for that existing agent.

Do not run Feishu `start`, `finalize`, or `participant bind --feishu-kind bot` for a worker that does not exist yet. `participant bind` only attaches Feishu credentials to an existing agent; it does not create the worker.

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

For worker flow, `finalize` calls `csgclaw-cli participant bind --feishu-kind bot`. The bind command saves the Feishu participant config and recreates the worker unless the skill helper was run with `finalize --recreate none` or `finalize --recreate manager`.
If the target worker is missing, `start` fails before creating a Feishu app and points back to `agent-creator`.

## Primary QR/Launcher Flow

### 1. Start registration and show URL/QR

Run from this skill directory:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start \
  --agent <target_agent_id> \
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
3. for manager targets only, bind `feishu:admin` human to the registration `open_id` when Feishu returns one
4. bind the Feishu bot participant through `csgclaw-cli participant bind --feishu-kind bot`
5. for worker targets, recreate the worker from the bind command so the new Feishu env/files take effect
   - if BoxLite reports `box with name '<name>' already exists` while CSGClaw reports `agent "<id>" not found`, stop and tell the user the host has a stale partial worker box; do not keep trying random API paths or host-only commands from inside manager
6. for manager targets, print a `csgclaw.action_card` JSON payload with a whitelisted `rebuild-manager` action; the CSGClaw Web chat message should render the button to complete the window-triggered manager bootstrap replace flow.
7. print JSON with `app_secret: present`, never the real secret

For a worker, default finalize is usually enough:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

Use an exec/tool timeout of at least 600 seconds for this command. The bind command should report `restart_status`; do not create a second worker or change the target agent ID.
Worker finalize must not bind or overwrite `feishu:admin`, even when Feishu returns a registration `open_id`; `feishu:admin` belongs to the manager Feishu app scope.

For manager, default finalize binds `feishu:admin` when Feishu returns `open_id`, binds `feishu:manager`, then prints a structured action card. Return the JSON object exactly as the chat message content: no leading sentence, no Markdown table, no bullet list, no ```json fence, and no explanatory wrapper. The CSGClaw Web frontend will render a "重建 Manager" button.
The click is handled by the browser and calls the manager bootstrap replace surface (`POST /api/v1/agents` with `{"id":"u-manager","replace":true}`), not the hazardous generic recreate route.

Do not run `python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py recreate-agent --agent u-manager` as a terminal self-recreate step. The manager-rebuild action must be completed by clicking the rendered Web window button, which calls `POST /api/v1/agents` with `{"id":"u-manager","replace":true}`.

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

Use `participant bind` to set manually:

```bash
printf '%s' '[REDACTED]' | csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind bot \
  --agent u-dev \
  --app-id cli_xxx \
  --app-secret-stdin \
  --restart
```

For manager setup, use the wrapper so the final chat response is a browser action card:

```bash
printf '%s' '[REDACTED]' | python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py bind-manager \
  --open-id ou_xxx \
  --app-id cli_xxx \
  --app-secret-stdin
```

Return the printed JSON object exactly as the chat response. Do not summarize it, translate it, add a Markdown table, or wrap it in a code fence.

## CLI Workflow Used by Script

The script writes Feishu config through `csgclaw-cli participant bind` because sandboxed skills should not edit host files directly.

For `u-manager`, `bind-manager` binds `feishu:admin` when `--open-id` is provided, binds `feishu:manager` without direct restart from inside the manager runtime, then prints a top-level action card:

```bash
printf '%s' '[REDACTED]' | python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py bind-manager --open-id ou_xxx --app-id cli_xxx --app-secret-stdin
```

Expected wrapper response shape:

```json
{
  "type": "csgclaw.action_card",
  "status": "manager_recreate_pending",
  "agent_id": "u-manager",
  "bot_id": "u-manager",
  "setup_status": "configured",
  "config": {
    "bot_bind": {
      "participant_id": "manager",
      "restart_status": "restart_skipped"
    }
  },
  "actions": [
    {
      "id": "rebuild-manager",
      "method": "manager-bootstrap-replace"
    }
  ]
}
```

For workers, the bind command recreates the worker by default so the runtime picks up the updated Feishu credentials:

```bash
printf '%s' '[REDACTED]' | csgclaw-cli participant bind --channel feishu --feishu-kind bot --agent u-dev --app-id cli_xxx --app-secret-stdin --restart
```

## CLI Workflow for Manual Control

Use `participant bind` for channel config. Use the helper script for manager rebuild because the manager must not recreate itself from the same manager-hosted run.

```bash
printf '%s' '[REDACTED]' | csgclaw-cli participant bind --channel feishu --feishu-kind bot --agent u-dev --app-id cli_xxx --app-secret-stdin --restart
```

## Worker One-Shot Recipe

1. Start registration:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start --agent <worker_id> --role worker --bot-name <worker_name> --description "<worker_desc>" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id <id>
```

Run the command with exec `timeout` at least `600`.

4. Confirm finalize returned `config.bot_bind.restart_status` for the worker.
5. Tell the user to test from Feishu by messaging or @mentioning the Feishu bot app.

## Manager One-Shot Recipe

Run this recipe from the normal flow and render the manager rebuild action card in the web window.

1. Start registration:

```bash
python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py start --agent u-manager --role manager --bot-name manager --description "manager agent" --qr
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
3. If you see older workflow docs mentioning alternate Feishu config commands, ignore them and use `csgclaw-cli participant bind ...` to write config.
4. Binding the wrong target: pass the CSGClaw agent ID such as `u-dev` or `u-manager`; the bind command writes the canonical Feishu participant ID.
5. Expecting bind alone to update an already-running PicoClaw box: worker recreate or manager rebuild is still required.
6. Calling manager recreate from inside this manager-hosted skill: return the action card so the current window renders the rebuild button.
7. Checking `agent list` or `participant list` after manager recreate and treating `stopped` as failure: manager gateway runs in daemon mode, so BoxLite status is not a reliable success signal for this skill.
8. Printing secrets in summaries or logs: always mask as `[REDACTED]` or `present`.
9. Calling CSGClaw SSE endpoint a Feishu webhook: it is an internal CSGClaw-to-PicoClaw bridge.
10. If Feishu changes the accounts registration endpoint or tenant policy blocks PersonalAgent creation, fall back to manual App ID/App Secret setup.

## Verification Checklist

- [ ] `start` printed a launcher URL or QR code for the user.
- [ ] `finalize` output shows `app_secret` only as `present`.
- [ ] `finalize` configured the target agent ID (`agent_id` field) and `app_id` in CSGClaw.
- [ ] `config.bot_bind.participant_id` is the canonical Feishu participant ID, such as `dev` or `manager`.
- [ ] CSGClaw participant exists with `channel=feishu`.
- [ ] Worker bind reported `restart_status` such as `worker_recreated` or `restart_skipped`.
- [ ] New worker finalize was run with a tool timeout of at least 600 seconds.
- [ ] Manager finalize returned a raw `csgclaw.action_card` JSON object with `rebuild-manager` action metadata for the web button.
- [ ] No manager-hosted command called the generic manager recreate endpoint or any host-side manager rebuild command.
- [ ] No public Feishu webhook endpoint was added or required.
