# CSGClaw SaaS — Environment Variable Contract

This document is the source of truth for the environment variables that
flow between **csgbot → csgclaw-server → manager-sandbox → worker-sandbox**
in the **CSGHub Sandbox SaaS** layout (server and agents run as Hub
sandboxes; csgclaw is configured with `[sandbox].provider = csghub`).

**Compile-time variants:** this repo supports only:

- **`go build`** — default binary (BoxLite **CLI** backend plus CSGHub
  provider; pick the active backend in `config.toml` / deployment).

There is **no** separate `csghub` build tag. Variables in §2–§6 matter
when the deployment uses the CSGHub sandbox provider and this PVC/API
layout; pure local BoxLite (`provider = boxlite`) does not need
them.

It has two audiences:

- **csgbot** operators, who must populate the server sandbox's env.
- **csgclaw** maintainers, who must keep the CSGHub provider and
  manager / worker env injection code in sync when a new variable is
  added.

## 1. Flow overview

```
┌──────────┐  POST /sandbox      ┌──────────────┐
│ csgbot   │ ─────────────────▶ │ CSGHub API   │
└──────────┘                    └──────┬───────┘
                                       │ create (image = csgclaw-server-sandbox)
                                       ▼
                                ┌──────────────┐
                                │ server pod   │  csgclaw-server-sandbox
                                │  csgclaw     │──────────────────┐
                                └──────┬───────┘                  │
             sandbox.Client.Create(...)│  (env = picoclawBoxEnvVars
                                       │   + CSGHub provider env;  │
                                       │   image = csgclaw-agent- │
                                       │   sandbox)               │
                                       ▼                          │
                                ┌──────────────┐                  │
                                │ manager pod  │  csgclaw-agent-sandbox
                                │ picoclaw     │──────────────┐   │
                                └──────┬───────┘              │   │
             POST /api/bots/:id/worker │                      │   │
                                       ▼                      │   │
                                ┌──────────────┐              │   │
                                │ worker pod   │  csgclaw-agent-sandbox
                                │ picoclaw     │              │   │
                                └──────────────┘              │   │
```

The SaaS uses **two** container images (no runtime role switch):

- **`csgclaw-server-sandbox:<tag>`** — csgclaw Go binaries + supervisor
  + python-sandbox. Runs only in the server pod. csgbot puts this tag
  into its own Deployment (not into any csgclaw env).
- **`csgclaw-agent-sandbox:<tag>`** — picoclaw gateway + supervisor +
  python-sandbox. Runs in manager and worker pods. The csgclaw server
  reads this image from `[bootstrap].manager_image_override` when set,
  otherwise from the built-in default, and propagates it
  into every manager/worker `sandbox.CreateRequest`.

Both images are built in the `sandbox-runtime` repo under
`csgclaw-server/` and `csgclaw-agent/` respectively. No
`CSGCLAW_ROLE`: the image identity carries the role.

Manager vs worker are both the same `csgclaw-agent-sandbox` image
and differ by sandbox name and bot id. The name sent to CSGHub is the
generic sandbox name produced by the agent service (`manager` for the
bootstrap manager, the worker name for workers), optionally prefixed by
`CSGCLAW_NAME` inside the CSGHub provider.

## 2. Server sandbox env (injected by csgbot)

### 2.1 Deployment identity / storage

| Variable | Purpose | Consumer |
|----------|---------|----------|
| `CSGCLAW_NAME` *(optional)* | Prefix applied to CSGHub sandbox names. If set to `tenant-a`, `worker-1` becomes `tenant-a-worker-1`. | `internal/sandbox/csghub/provider.go` |
| `CSGCLAW_PVC_MOUNT_PATH` *(optional)* | Host-side PVC mount root used to turn local mount paths into CSGHub volume subpaths. Default `/opt/csgclaw`. | `internal/sandbox/csghub/provider.go` |
| `CSGCLAW_PVC_SUBPATH_PREFIX` *(optional)* | Extra prefix prepended to computed CSGHub volume subpaths. | `internal/sandbox/csghub/provider.go` |

### 2.2 CSGHub API credentials (required)

| Variable | Purpose |
|----------|---------|
| `CSGHUB_API_BASE_URL` | Base URL of the CSGHub Sandbox REST API, e.g. `https://hub.example.com`. |
| `CSGHUB_USER_TOKEN` | Bearer token used for sandbox lifecycle and gateway calls. The CSGHub provider also injects this value into manager / worker sandbox environments as `CSGHUB_USER_TOKEN`. |
| `CSGHUB_AIGATEWAY_URL` *(optional)* | Overrides the AI-gateway endpoint if it differs from `CSGHUB_API_BASE_URL`. Used by the server-side CSGHub client; not injected into manager / worker env by current code. |

### 2.3 Sandbox request parameters

Used to build every `sandbox.CreateRequest` the server issues for a
manager or worker sandbox.

Current code-level contract (`internal/sandbox/csghub/provider.go#loadRuntimeConfigFromEnv`):

- Required by the CSGHub provider: `CSGHUB_API_BASE_URL`,
  `CSGHUB_USER_TOKEN`
- Optional sandbox request parameters: `CSGCLAW_RESOURCE_ID`,
  `CSGCLAW_CLUSTER_ID`, `CSGCLAW_SANDBOX_PORT`,
  `CSGCLAW_SANDBOX_TIMEOUT`

| Variable | Purpose |
|----------|---------|
| `CSGCLAW_RESOURCE_ID` *(int, optional)* | CSGHub resource spec id (CPU/RAM class). |
| `CSGCLAW_CLUSTER_ID` *(optional)* | Cluster identifier in CSGHub. |
| `CSGCLAW_SANDBOX_PORT` *(int, optional)* | Port exposed by picoclaw gateway; default set by Hub. |
| `CSGCLAW_SANDBOX_TIMEOUT` *(int seconds, optional)* | Sandbox idle timeout. |
| `CSGCLAW_SANDBOX_READY_TIMEOUT` *(duration or int seconds, optional)* | Max wall time `reconcileSandbox` waits for the Hub-reported state to converge on `Running` before failing. Default `5m`; clamped at a `5s` minimum. Accepts both Go duration (`90s`, `2m30s`) and bare integer seconds. |
| `CSGCLAW_SANDBOX_POLL_INTERVAL` *(duration or int seconds, optional)* | Cadence of the readiness poll. Default `3s`; clamped to `[500ms, 30s]`. |

### 2.4 Server config and agent image

Current code reads server listen / advertise / access token, model
providers, and bootstrap image from `config.toml` (or CLI flags where
supported). There is no current direct env override for
`CSGCLAW_LISTEN`, `CSGCLAW_ADVERTISE_BASE_URL`, `CSGCLAW_LLM_BASE_URL`,
or `CSGCLAW_SANDBOX_IMAGE` in this repository. If a deployment wants to
drive those values from env, put env placeholders such as
`${CSGCLAW_ACCESS_TOKEN}` in `config.toml`; the config loader expands
them when loading.

The manager / worker image is the service's `managerImage`, normally
from the built-in default unless `[bootstrap].manager_image_override`
is set.

## 3. Manager sandbox env (injected by the server)

For manager and workers created through `createGatewayBox`, the agent
service first composes Picoclaw / LLM / channel env in
`internal/agent/box.go#gatewayCreateSpec`. When `[sandbox].provider =
csghub`, `internal/sandbox/csghub/provider.go#createRequest` then adds
the CSGHub API env before sending the CSGHub `CreateRequest`.

| Group | Variable | Source |
|-------|----------|--------|
| Constant | `HOME=/home/picoclaw` | constant |
| Picoclaw ↔ server | `CSGCLAW_BASE_URL` | `resolveManagerBaseURL(server)` |
| Picoclaw ↔ server | `CSGCLAW_ACCESS_TOKEN` | `server.AccessToken` |
| Picoclaw ↔ server | `PICOCLAW_CHANNELS_CSGCLAW_BASE_URL` | `resolveManagerBaseURL(server)` |
| Picoclaw ↔ server | `PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN` | `server.AccessToken` |
| Picoclaw ↔ server | `PICOCLAW_CHANNELS_CSGCLAW_BOT_ID` | per-agent |
| Picoclaw ↔ server | `CSGCLAW_LLM_BASE_URL` | `llmBridgeBaseURL(...)` |
| Picoclaw ↔ server | `CSGCLAW_LLM_API_KEY` | `server.AccessToken` |
| Picoclaw ↔ server | `CSGCLAW_LLM_MODEL_ID` | per-agent |
| OpenAI-compatible bridge | `OPENAI_BASE_URL` / `OPENAI_API_KEY` / `OPENAI_MODEL` | LLM bridge values |
| Picoclaw model | `PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME` / `PICOCLAW_CUSTOM_MODEL_NAME` / `PICOCLAW_CUSTOM_MODEL_ID` / `PICOCLAW_CUSTOM_MODEL_API_KEY` / `PICOCLAW_CUSTOM_MODEL_BASE_URL` | per-agent model and bridge values |
| Hub API | `CSGHUB_API_BASE_URL` | injected by CSGHub provider from `CSGHUB_API_BASE_URL` |
| Hub API | `CSGHUB_USER_TOKEN` | injected by CSGHub provider from `CSGHUB_USER_TOKEN` |
| Feishu (optional) | `PICOCLAW_CHANNELS_FEISHU_APP_ID` / `PICOCLAW_CHANNELS_FEISHU_APP_SECRET` | `channels` config |

`CSGHUB_AIGATEWAY_URL` and `CSGHUB_USER_NAME` are not injected into
manager / worker environments by current code.

## 4. Worker sandbox env

Workers created through `CreateWorker` use the same `createGatewayBox`
path as the manager, so their env is the same shape as §3. Values such
as bot id, sandbox name, model selection, and LLM bridge URL are
per-worker.

## 5. Shared volumes

Every manager/worker sandbox created through `createGatewayBox` mounts
the following paths. The CSGHub provider converts host paths under
`CSGCLAW_PVC_MOUNT_PATH` into claim-relative `sandbox_mount_subpath`
values and optionally prefixes them with `CSGCLAW_PVC_SUBPATH_PREFIX`.

| Sandbox path | PVC subpath (claim-relative) | Owner |
|--------------|------------------------------|-------|
| `/home/picoclaw/.picoclaw/workspace` | path relative to `CSGCLAW_PVC_MOUNT_PATH`, typically `.csgclaw/agents/<name>/workspace` | server writes workspace template; picoclaw read/write |
| `/home/picoclaw/.picoclaw/workspace/projects` | path relative to `CSGCLAW_PVC_MOUNT_PATH`, typically `.csgclaw/projects` | shared across all agents |

## 6. Networking contract

- Every sandbox (server + manager + worker) must share an overlay
  reachable by pod-IP or Hub service DNS; the server's advertised URL
  must resolve from inside manager/worker pods.
- Manager/worker pods must reach the server on
  `CSGCLAW_BASE_URL` (LLM bridge `/api/bots/<id>/llm`, worker spawn
  `/api/bots/<id>/workers`, health `/healthz`).
- Server pod must reach the CSGHub Sandbox API on
  `CSGHUB_API_BASE_URL` (TLS + bearer).
- The server-side CSGHub client uses `CSGHUB_AIGATEWAY_URL` when set;
  otherwise it derives gateway URLs from `CSGHUB_API_BASE_URL`.

## 7. csgbot checklist

Before invoking `POST /sandbox` for a csgclaw server, csgbot must
populate, at minimum:

- `CSGCLAW_NAME` *(optional but useful for name isolation)*,
  `CSGCLAW_PVC_MOUNT_PATH`
- `CSGHUB_API_BASE_URL`, `CSGHUB_USER_TOKEN`
- `CSGCLAW_RESOURCE_ID`, `CSGCLAW_CLUSTER_ID` *(optional but recommended)*
- a `config.toml` whose `[sandbox].provider` is `csghub`, whose
  `[server].show_upgrade` is `false` for managed deployments that cannot
  self-upgrade, whose
  `[bootstrap].manager_image_override` points at the
  `csgclaw-agent-sandbox` image when you need to override the built-in
  default, and whose `[server]` / `[models]` sections are valid for the
  deployment

The server pod's own container image (`csgclaw-server-sandbox:<tag>`)
is picked by csgbot directly in the Deployment / CreateRequest spec —
not via any csgclaw env variable.

Missing required CSGHub provider variables trigger a fast-fail when the
server opens the CSGHub sandbox runtime; see
`internal/sandbox/csghub/provider.go#loadRuntimeConfigFromEnv`.
