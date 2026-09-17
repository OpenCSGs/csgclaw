# Agent Apps

English | [中文](apps.zh.md)

Each Agent manages its own GitLab, Feishu, and llm-wiki Apps.
You can add the same App more than once, with a separate name, service address, and credentials for each installation.

## Add and connect

Open an Agent's **Apps** tab, choose **Add app**, configure the service, test the connection, and finish adding it.
An App connects to an existing HTTP or stdio MCP service; CSGClaw does not deploy that service.
For HTTP, supply its MCP URL and the authentication mode required by that service.
For stdio, supply its command, arguments, working directory, and required environment variables.

GitLab accepts the service's Token/PAT or custom authentication header.
Feishu can reference the current Agent's channel App ID and App Secret or use separately supplied credentials.
Channel credentials are resolved when connecting and are refreshed after changes, without copying the channel secret into the installation.
llm-wiki accepts a knowledge-base MCP address and its Token; the knowledge-base picker can fill the address.
Browser OAuth2 authorization is not supported in this version.

**Disable** preserves settings and credentials while blocking calls.
**Disconnect** clears the installation's managed credentials and keeps its name and ordinary settings.
A manual disconnect remains in effect after channel updates and server restarts until you explicitly connect again.
**Remove** deletes the installation and its private data; it does not delete a referenced Feishu channel.

## Access

App management opens directly like the existing CSGClaw personal-assistant UI, without a service-token login.
GitLab, Feishu, and knowledge-base credentials are still configured separately in each App.
Agent MCP endpoints continue to validate automatically provisioned Agent-scoped tokens.
The App CLI requires no additional login; set `CSGCLAW_BASE_URL` to select the service address.
Existing protected APIs retain their service-token checks; `server.no_auth` is not a Web UI login switch.

## Remote Feishu passthrough services

For MCP services using OpenCSG ingress authentication and forwarded Feishu credentials, select HTTP with Bearer Token authentication.
Set the Token field to the OpenCSG access token, then add `lark-access-token` (the raw Feishu token without a Bearer prefix) and `X-Lark-Token-Type` (`user_access_token` or `tenant_access_token`) in the advanced headers.
Headers are sent with upstream MCP requests and stored as per-instance credentials without echoing their values.
Do not use direct App ID/App Secret header mapping for a passthrough service that does not accept those credentials.
Existing Feishu tokens can be supplied manually; token issuance, automatic refresh, and user OAuth are not implemented for this mode.
App credentials normally obtain a tenant access token; a user access token additionally requires user authorization.

## Selecting an App in conversation

Users can ask for work such as “list GitLab issues” without naming an App installation.
The Agent uses the only matching connected App directly.
With multiple matches, it uses an explicit account, resource URL, project identifier, or established choice in the current task when that uniquely identifies an installation; otherwise it asks the user to choose from the matching App names.
When no matching App is usable, it guides the user to add or reconnect one.
Tool descriptions include the owning App name and service type, and renaming an App refreshes its tool catalog.

## CLI

Both `csgclaw` and `csgclaw-cli` provide the following commands:

```sh
csgclaw app catalog
csgclaw app list --agent agent-dev
csgclaw app get --agent agent-dev --id INSTALLATION_ID
csgclaw app probe --agent agent-dev --file probe.json
csgclaw app add --agent agent-dev --file request.json
csgclaw app update --agent agent-dev --id INSTALLATION_ID --file changes.json
csgclaw app connect --agent agent-dev --id INSTALLATION_ID
csgclaw app disconnect --agent agent-dev --id INSTALLATION_ID
csgclaw app remove --agent agent-dev --id INSTALLATION_ID
```

`add`, `update`, and `probe` read a JSON request from `--file`; use `--file -` for standard input.
There are no command-line flags for App credentials.
Keep credential files private, for example with permission `0600`, and exclude them from source control.
The CLI prints the API's redacted result and does not echo the request file or its credentials.
Use the global `--output json` flag before `app` for complete response fields.

Example `request.json` for adding an App:

```json
{
  "app_id": "gitlab",
  "name": "Work GitLab",
  "config": {
    "transport": "http",
    "url": "https://mcp.example.com/mcp",
    "auth_mode": "bearer"
  },
  "credentials": {"token": "<upstream-service-token>"},
  "connect": true
}
```

For `probe`, omit `name` and `connect`; only `app_id`, `config`, `credentials`, and an optional `installation_id` are accepted.
For `update`, provide the fields to change, such as `{"enabled":false}`; omit `app_id` and `connect`.
An add request whose connection fails still returns the newly created installation with an error status so it can be repaired.
Inside an Agent runtime, the CLI permits catalog/list/get and supplies an App settings link for the user; connection and credential changes are made in App settings.

## API and runtime behavior

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/apps` | Built-in catalog |
| `GET /api/v1/apps/{app_id}` | App definition and configuration fields |
| `GET/POST /api/v1/agents/{agent_id}/apps` | List or add installations |
| `GET/PATCH/DELETE /api/v1/agents/{agent_id}/apps/{installation_id}` | Read, change, or remove an installation |
| `POST /api/v1/agents/{agent_id}/apps:probe` | Test unsaved connection settings |
| `POST .../{installation_id}/connect` or `/disconnect` | Connect or explicitly disconnect |

Codex accesses App tools through the Agent's managed `/api/v1/agents/{agent_id}/mcp` endpoint.
Existing manually configured MCP servers continue to work independently; App-owned rows link to App settings.
Disabling, disconnecting, or removing an App immediately blocks subsequent calls, including names retained by an old session.
Read-only Agents receive and can call only tools annotated as read-only.
Tool search belongs to Codex; requests containing tool definitions or tool history cannot silently fall back to text-only Chat Completions.

Internal packages use root-level `plugin.json` and `apps.json`; the three built-ins do not require `mcp.json`.
Credentials remain in private local state, and each stdio installation receives independent `HOME`, `PLUGIN_DATA`, and `PLUGIN_ROOT` directories.
Plugin markets, arbitrary package imports, Skills/Hooks execution, and browser OAuth2 are outside this version's App flow.
