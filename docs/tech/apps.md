# Agent Apps

English | [中文](apps.zh.md)

Configure GitLab, Feishu, and llm-wiki connections once in **Resources > Apps**.
Each resource has a unique name, service address, and protected credentials or credential references.
Multiple instances of one App type can represent different accounts or deployments.

## Add and bind

Create and test a resource, then open an Agent's **Apps** tab and choose **Add from resources**.
Agent bindings store a resource reference and their own enabled/disconnected intent, never copied credentials.
Each binding has an isolated MCP session, private stdio directories, and Agent-scoped tool names.
Updating a resource revokes the previous tools and reconnects its enabled, previously connected bindings.
Explicitly disconnected or disabled bindings remain inactive.
Global disable stops every binding; Agent disable affects only that Agent.
Removing a binding or deleting an Agent keeps the global resource and other Agent bindings.
Deleting a global resource shows the affected Agents and removes all of its bindings.

Feishu resources may reference each bound Agent's Feishu channel.
The global page shows that identity will be checked after binding and allows saving without a misleading global authentication test.
Channel credentials are resolved per Agent at connection time and are never copied into the resource.
Resources using explicit credentials can be tested directly from the global page.
App resources connect to existing HTTP or stdio MCP services; CSGClaw does not deploy them.
Browser OAuth2 remains unsupported.
Existing local installations are converted once to global resources and bindings, preserving binding IDs, connection intent, and tool names.

## Access

App management opens directly like the existing CSGClaw personal-assistant UI, without a service-token login.
GitLab, Feishu, and knowledge-base credentials are still configured separately in each App.
Agent MCP endpoints continue to validate automatically provisioned Agent-scoped tokens.
The App CLI requires no additional login; set `CSGCLAW_BASE_URL` to select the service address.
Existing protected APIs retain their service-token checks; `server.no_auth` is not a Web UI login switch.

## OpenCSG platform authentication

All HTTP Apps, including GitLab, Feishu, and llm-wiki, support a shared OpenCSG platform credential source.
New OpenCSG HTTPS endpoints default to Use current OpenCSG login, reading the latest credential on every request without copying it into the App.
Explicit manual selections and supplied manual Bearer tokens are preserved.
Custom business headers such as `PRIVATE-TOKEN` or `X-API-Key` remain independent of the platform `Authorization` header; using `Authorization` for both is rejected.
External and local URLs and stdio processes do not receive the OpenCSG login automatically.
Credentials are restricted to matching OpenCSG HTTPS environments; they are not sent across production/staging or to external domains.
Manual platform tokens remain supported, with explicit expiry messages and a linkable choice to use the current login.
Logout requires reauthorization; login refreshes opted-in connections while preserving explicit disconnects and disabled Apps.
Connection errors distinguish platform 401/403, missing login, environment mismatch, Feishu token issuance, missing endpoints, and unavailable/sleeping backends, with the HTTP status retained.
## Remote Feishu passthrough services

For HTTP Feishu Apps, select Feishu app credentials, enter the MCP URL, and use the Agent's Feishu channel or supply an App ID/App Secret manually.
The Connector reuses Feishu token issuance with a private per-connection cache and refreshes the tenant token on demand before expiry.
MCP requests receive `lark-access-token` and `X-Lark-Token-Type: tenant_access_token` automatically, while the platform credential uses `Authorization: Bearer ...`.
Users do not need to enter header names or temporary Feishu tokens, and App Secrets are never sent to the passthrough MCP server.
Only an explicit `lark_token_invalid` rejection triggers one refresh and retry; scope failures, network failures, and other business errors are not retried automatically.
A repeated rejection marks the connection as requiring authorization.
Channel credential changes replace the connection and its token cache; disconnect, disable, and removal retain their existing lifecycle behavior.
Local stdio MCP services continue receiving application credentials through environment variables.
User access tokens require user authorization; automatic browser OAuth remains unsupported, while existing UATs can still be configured manually with Bearer/custom-header authentication.

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
csgclaw app list --global
csgclaw app add --global --file resource.json
csgclaw app update --global --id RESOURCE_ID --file changes.json
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

Example `resource.json` for creating a global App:

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
  "connect": false
}
```

For `probe`, omit `name` and `connect`; only `app_id`, `config`, `credentials`, and an optional `installation_id` are accepted.
For `update`, provide the fields to change, such as `{"enabled":false}`; omit `app_id` and `connect`.
Agent add requests use `{"resource_id":"RESOURCE_ID","connect":true}`; Agent updates only accept `enabled`.
An unsuccessful connection keeps the binding with a diagnostic status so it can be repaired.
Inside an Agent runtime, the CLI permits catalog/list/get and supplies an App settings link for the user; connection and credential changes are made in App settings.

## API and runtime behavior

| Endpoint | Purpose |
|---|---|
| `GET /api/v1/apps` | Built-in definitions |
| `GET /api/v1/apps/{app_id}` | Definition and connection defaults |
| `GET/POST /api/v1/app-resources` | List or create global resources |
| `GET/PATCH/DELETE /api/v1/app-resources/{resource_id}` | Manage a resource and inspect affected Agents |
| `POST /api/v1/app-resources:probe` | Test a resource using explicit credentials |
| `GET/POST /api/v1/agents/{agent_id}/apps` | List or add resource bindings |
| `GET/PATCH/DELETE /api/v1/agents/{agent_id}/apps/{installation_id}` | Read, enable/disable, or remove one binding |
| `POST /api/v1/agents/{agent_id}/apps:probe` | Test with the Agent's resolved identity |
| `POST .../{installation_id}/connect` or `/disconnect` | Connect or disconnect one binding |

Codex accesses App tools through the Agent's managed `/api/v1/agents/{agent_id}/mcp` endpoint.
Existing manually configured MCP servers continue to work independently; App-owned rows link to App settings.
Disabling, disconnecting, or removing an App immediately blocks subsequent calls, including names retained by an old session.
Read-only Agents receive and can call only tools annotated as read-only.
Tool search belongs to Codex; requests containing tool definitions or tool history cannot silently fall back to text-only Chat Completions.

Internal packages use root-level `plugin.json` and `apps.json`; the three built-ins do not require `mcp.json`.
Credentials remain in private local state, and each stdio installation receives independent `HOME`, `PLUGIN_DATA`, and `PLUGIN_ROOT` directories.
Plugin markets, arbitrary package imports, Skills/Hooks execution, and browser OAuth2 are outside this version's App flow.

## Installation defaults and settings

New App forms read non-secret connection defaults from the catalog's `config_schema.properties.*.default` values.
GitLab and Feishu prefill the configured staging MCP endpoints and infer the matching OpenCSG login reference.
llm-wiki currently prefills the local test service at `http://127.0.0.1:19093/mcp`; the address can be replaced manually or filled by the knowledge-base picker.
Loopback addresses refer to the machine running CSGClaw.
Credentials are never included in catalog defaults, and existing installations keep their saved settings.
The form separates service connection, MCP service authentication, and Feishu application identity; timeouts and extra headers/environment values remain under advanced settings.
