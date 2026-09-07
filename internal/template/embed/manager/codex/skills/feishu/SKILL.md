---
name: feishu
description: Configure and troubleshoot CSGClaw Feishu/Lark channel credentials for manager or worker agents. Use when the Manager needs to generate a Feishu bot app creation URL or QR code, collect App ID/App Secret through registration, bind Feishu participants through `csgclaw-cli participant bind`, recreate workers, or debug Feishu messages not reaching CSGClaw workers.
---

# Feishu

This skill sets up Feishu/Lark bot app credentials for CSGClaw-managed manager and worker agents.

## Script

Use the bundled script from the Codex skill root:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" start --agent <worker-agent-id-or-display-name> --role worker --bot-name dev --qr
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" finalize --registration-id <id>
```

If `start`/`poll` returns a machine-mode `next` command, prefer that absolute command.

## Script roles

- `scripts/feishu_register.py`: User-facing CLI entrypoint. Supports `start`, `poll`, `finalize`, `status`, `recreate-agent`, `bind-manager`.
- `scripts/feishu_setup/commands.py`: Parses CLI arguments and maps them to handler functions.
- `scripts/feishu_setup/registration.py`: Implements registration flow and device-code polling state transitions.
- `scripts/feishu_setup/csgclaw.py`: Applies config to CSGClaw through `participant bind` and activates manager Feishu bindings without rebuilding the manager runtime.
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

Do not add or require a public Feishu Open Platform HTTP webhook as the main inbound path. CSGClaw uses Feishu/Lark WebSocket mode for inbound bot messages. CSGClaw's `/api/v1/channels/feishu/participants/{participant}/events` endpoint is an internal runtime bridge for workers, not a Feishu public webhook.

## When to Use

Use this skill when the user asks to:

- create/configure Feishu credentials for the manager agent (`manager`, `u-manager`, or `agent-manager`) or an existing worker Agent
- generate a Feishu/Lark bot creation URL or QR code
- get Feishu AK/SK, App ID/App Secret, or client_id/client_secret for a CSGClaw-managed agent
- bind Feishu participant config after setting Feishu credentials
- recreate a worker after Feishu credentials are configured, or activate a manager binding without recreating the manager
- debug why Feishu messages do not reach a CSGClaw worker

Do not use this skill for generic Feishu webhook integrations or non-CSGClaw Feishu app development.

## Terms

- Target agent reference: use `manager`, `u-manager`, or `agent-manager` for the manager; for a worker, `start --agent` reads the Agent registry once and matches either its exact runtime Agent ID or exact display name.
- Feishu `app_id` / `app_secret`: the Feishu bot application's credentials.
- AK/SK in user wording usually means Feishu `app_id/app_secret` or `client_id/client_secret` returned by the registration flow.
- Manager agent: `manager`, `u-manager`, and `agent-manager` are equivalent references; recreating it can interrupt the current manager skill run.
- Worker agent: any non-manager Agent; recreating it is usually safe after config succeeds.

## Prerequisites

1. CSGClaw server is running.
2. Confirm CSGClaw API access is available through environment variables, not command-line token flags:
   - `CSGCLAW_BASE_URL`, default `http://127.0.0.1:18080`
   - `CSGCLAW_ACCESS_TOKEN`, unless server auth is disabled
3. The script is run from the deployed skill directory:
   - inside Codex Manager: `$CODEX_HOME/skills/feishu`
   - host repo path: `internal/template/embed/manager/codex/workspace/skills/feishu`
4. Server build supports:
   - `csgclaw-cli participant bind`
   - `POST /api/v1/channels/feishu/participants`
   - `POST /api/v1/agents/{id}/recreate`

## Manager Group Permissions

CSGClaw cannot silently grant Feishu/Lark app scopes from inside the Manager runtime. Feishu group operations use the manager agent's Feishu bot app credentials, so the tenant admin must approve the required scopes in Feishu/Lark Open Platform.

For new Feishu groups, after the manager and worker Feishu configs exist, prefer creating the group with all participant IDs already included:

```bash
csgclaw-cli room create --title worker-group --creator-id admin --member-ids pt-manager,<worker-participant-id> --channel feishu
```

CSGClaw records the Manager participant in the room, but does not invite its Feishu app again: the Manager app bot is automatically added when it creates the chat. It resolves and invites only the worker bot apps. This keeps the created `chat_id` visible if a worker invite fails, but it still requires manager app group scopes for chat creation and member invites.
When creating the group from a direct/private request, keep the human requester as `--creator-id` (default `admin`) so the requester is recorded in the CSGClaw room members. Include the actual Manager participant ID (normally `pt-manager`) plus the requested worker participant IDs in `--member-ids`; obtain them from `participant list`.

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

## User-Facing Completion Reply

Treat the script's JSON as tool output, not as the chat reply. After a successful `finalize` or `bind-manager`, reply in the user's language with one concise confirmation, for example: `已完成 manager 的飞书对接，飞书桥接已生效。`

Include the target Agent name and, when useful, one safe next step such as “请在飞书中发送一条消息测试”。 Never return or quote the raw JSON object, its fields, or any secret-related status in the chat reply. If the operation fails or is partial, summarize the failure and the next action in plain language instead.

## Choose Target Agent

Ask for the target when it is not explicit.

If the user asks to **create/provision/add a new worker and connect it to Feishu** in one request, do this as a two-phase workflow:

1. Use `agent-creator` first to create the worker. That skill must run `template list`, `template get`, then `csgclaw-cli participant create --type agent --bind create --from-template ...`.
2. Only after the worker agent exists, return to this Feishu skill and run the QR/manual credential flow for that existing agent.

Do not run Feishu `start`, `finalize`, or `participant bind --feishu-kind bot` for a worker that does not exist yet. `participant bind` only attaches Feishu credentials to an existing agent; it does not create the worker.

If the user does not specify an agent in the request, ask: "请明确要对接飞书的目标 Agent 名字（如 `manager`/`u-manager`/`agent-manager`、worker 显示名称，或 `agent-...` ID）".
Resolve target:
1. If input is `manager`, `u-manager`, or `agent-manager`, treat as manager flow.
2. Otherwise, treat input as a worker **Agent reference**. Pass it unchanged to `start --agent`; it calls `GET /api/v1/agents` once, then matches an exact runtime Agent ID or display name in that registry.
3. Use the canonical `agent_id` returned by `start` for the rest of the flow. Never manufacture an ID by adding `u-` to a name.
4. `participant list --channel csgclaw` and `participant list --channel feishu` only list channel participants. Their absence does **not** prove that a worker Agent is missing, so never invoke `agent-creator` solely because those lists do not contain the worker.
5. Invoke `agent-creator` only when `start` reports that the Agent was not found. If the input matches multiple Agents (including an ID/name collision), ask for the runtime Agent ID instead.
6. If only role was inferred as manager, stop using recreate path and force the manager binding activation flow.

Examples:
- `dev` -> resolve the existing Agent whose display name is `dev`, then use its returned runtime Agent ID
- `agent-dev` -> use that existing runtime Agent ID directly
- `manager` -> manager
- `u-manager` -> manager
- `agent-manager` -> manager

For worker flow, `finalize` calls `csgclaw-cli participant bind --feishu-kind bot` with the resolved runtime Agent ID. The bind command saves the Feishu participant config and recreates the worker unless the skill helper was run with `finalize --recreate none` or `finalize --recreate manager`.
If the target worker is missing, `start` fails before creating a Feishu app and points back to `agent-creator`.

## Primary QR/Launcher Flow

### 1. Start registration and show URL/QR

Run from this skill directory:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" start \
  --agent <target_agent_id_or_exact_display_name> \
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
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" finalize --registration-id <id>
```

When running `finalize` through the manager's exec tool, always set the tool timeout to at least 600 seconds. Worker setup can take time on first use, and the default tool timeout can interrupt the create flow before CSGClaw persists the worker agent.

By default, `finalize` will:

1. poll Feishu/Lark until credentials are available or timeout
2. receive `client_id/client_secret`
3. for manager targets only, bind `feishu:admin` human to the registration `open_id` when Feishu returns one
4. bind the Feishu bot participant through `csgclaw-cli participant bind --feishu-kind bot`
5. for worker targets, recreate the worker from the bind command so the new Feishu config takes effect
   - if the runtime reports a name conflict while CSGClaw reports `agent "<id>" not found`, stop and tell the user the host has a stale partial worker runtime; do not keep trying random API paths or host-only commands from inside manager
6. for manager targets, call the Agent binding activation API so the existing Codex runtime refreshes its Feishu bridge without rebuilding the manager
7. print JSON with `app_secret: present`, never the real secret

For a worker, default finalize is usually enough:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" finalize --registration-id <id>
```

Use an exec/tool timeout of at least 600 seconds for this command. The bind command should report `restart_status`; do not create a second worker or change the target agent ID.
Worker finalize must not bind or overwrite `feishu:admin`, even when Feishu returns a registration `open_id`; `feishu:admin` belongs to the manager Feishu app scope.

For manager, default finalize binds `feishu:admin` when Feishu returns `open_id`, binds `feishu:manager`, calls the binding activation API, then prints a structured JSON object for the tool. Follow [User-Facing Completion Reply](#user-facing-completion-reply) instead of returning that object. A successful manager finalize includes `config.binding_activation` / `activation` and no `rebuild-manager` action.

Do not run `python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" recreate-agent` for the Manager as a terminal self-recreate step. A normal manager finalize should not produce or require a manager rebuild action.

For manager only, do not use host runtime status as a post-recreate success check in this skill. The manager path is complete when the binding activation result succeeds.

### 3. Optional status/poll commands

Check saved state without exposing device_code or secret:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" status --registration-id <id>
```

Check whether user has confirmed yet:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" poll --registration-id <id>
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
  --agent <worker-agent-id> \
  --app-id cli_xxx \
  --app-secret-stdin \
  --restart
```

For manager setup, use the wrapper so the binding and Feishu bridge activation complete automatically:

```bash
printf '%s' '[REDACTED]' | python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" bind-manager \
  --open-id ou_xxx \
  --app-id cli_xxx \
  --app-secret-stdin
```

After a successful manual binding, follow [User-Facing Completion Reply](#user-facing-completion-reply). Do not echo the printed JSON.

## CLI Workflow Used by Script

The script writes Feishu config through `csgclaw-cli participant bind` because skills should not edit host files directly.

For any Manager reference (`manager`, `u-manager`, or `agent-manager`), `bind-manager` binds `feishu:admin` when `--open-id` is provided, binds `feishu:manager` without restarting the Codex runtime, then calls the Agent binding activation API to refresh the Feishu bridge against the existing Codex session:

```bash
printf '%s' '[REDACTED]' | python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" bind-manager --open-id ou_xxx --app-id cli_xxx --app-secret-stdin
```

Expected wrapper response shape:

```json
{
  "status": "configured",
  "agent_id": "u-manager",
  "bot_id": "u-manager",
  "binding_activated": true,
  "config": {
    "bot_bind": {
      "participant_id": "manager",
      "restart_status": "restart_skipped"
    },
    "binding_activation": {
      "id": "u-manager",
      "status": "running"
    }
  }
}
```

For workers, the bind command recreates the worker by default so the runtime picks up the updated Feishu credentials:

```bash
printf '%s' '[REDACTED]' | csgclaw-cli participant bind --channel feishu --feishu-kind bot --agent <worker-agent-id> --app-id cli_xxx --app-secret-stdin --restart
```

## CLI Workflow for Manual Control

Use `participant bind` for channel config. The manager wrapper automatically activates its Feishu bridge without recreating the manager runtime.

```bash
printf '%s' '[REDACTED]' | csgclaw-cli participant bind --channel feishu --feishu-kind bot --agent <worker-agent-id> --app-id cli_xxx --app-secret-stdin --restart
```

## Worker One-Shot Recipe

1. Start registration:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" start --agent <worker_id> --role worker --bot-name <worker_name> --description "<worker_desc>" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" finalize --registration-id <id>
```

Run the command with exec `timeout` at least `600`.

4. Confirm finalize returned `config.bot_bind.restart_status` for the worker.
5. Tell the user to test from Feishu by messaging or @mentioning the Feishu bot app.

## Manager One-Shot Recipe

Run this recipe from the normal flow and use the returned binding activation result as the success signal.

1. Start registration:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" start --agent agent-manager --role manager --bot-name manager --description "manager agent" --qr
```

2. Send the printed URL/QR to the user.
3. After user confirms creation, finalize without recreate:

```bash
python "$CODEX_HOME/skills/feishu/scripts/feishu_register.py" finalize --registration-id <id>
```

4. Follow [User-Facing Completion Reply](#user-facing-completion-reply). Do not echo the `finalize` JSON object.

5. Do not call a manager recreate API or host command from this skill. The script activates the updated Feishu binding through CSGClaw without rebuilding the Manager.

Do not use the generic manager recreate endpoint or any terminal/host-side manager rebuild fallback.

## Common Pitfalls

### Creating a Feishu worker room

When the user asks for a group containing workers, keep the human requester as `--creator-id`.
Include `manager` plus the requested worker participant IDs in `--member-ids`.
Use the actual requester instead of `admin` when different, and replace `<worker-participant-id>` with IDs from `participant list`.

```bash
csgclaw-cli room create --title worker-group --creator-id admin --member-ids manager,<worker-participant-id> --channel feishu
```

1. Using `csgclaw-cli agent ...`: lite CLI does not have agent commands. Use full `csgclaw` or API.
2. Running host-only commands from inside manager: manager usually only has `csgclaw-cli`; use this script/API from manager, and ask the host operator to clean stale runtime state if needed.
3. If you see older workflow docs mentioning alternate Feishu config commands, ignore them and use `csgclaw-cli participant bind ...` to write config.
4. Binding the wrong target: for a worker pass the resolved runtime Agent ID (or a unique display name); never derive an ID by prepending `u-`. The bind command writes the canonical Feishu participant ID.
5. Expecting bind alone to update an already-running worker: worker recreate is still required; the manager wrapper activates its binding automatically.
6. Calling manager recreate from inside this manager-hosted skill: use the binding activation result instead; it preserves the current Codex session.
7. Treating a binding activation API failure as configured: the participant may be saved, but the activation must succeed before the Skill reports completion.
8. Printing secrets in summaries or logs: always mask as `[REDACTED]` or `present`.
9. Calling CSGClaw SSE endpoint a Feishu webhook: it is an internal CSGClaw-to-runtime bridge.
10. If Feishu changes the accounts registration endpoint or tenant policy blocks PersonalAgent creation, fall back to manual App ID/App Secret setup.

## Verification Checklist

- [ ] `start` printed a launcher URL or QR code for the user.
- [ ] `finalize` output shows `app_secret` only as `present`.
- [ ] `finalize` configured the target agent ID (`agent_id` field) and `app_id` in CSGClaw.
- [ ] `config.bot_bind.participant_id` is the canonical Feishu participant ID, such as `pt-dev` or `pt-manager`.
- [ ] CSGClaw participant exists with `channel=feishu`.
- [ ] Worker bind reported `restart_status` such as `worker_recreated` or `restart_skipped`.
- [ ] New worker finalize was run with a tool timeout of at least 600 seconds.
- [ ] Manager finalize reports a successful `config.binding_activation` result and no `rebuild-manager` action.
- [ ] No manager-hosted command called the generic manager recreate endpoint or any host-side manager rebuild command.
- [ ] No public Feishu webhook endpoint was added or required.
