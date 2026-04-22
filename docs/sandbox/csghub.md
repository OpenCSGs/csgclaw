# CSGClaw SaaS — Environment Variable Contract

This document is the source of truth for the environment variables that
flow between **csgbot → csgclaw-server → manager-sandbox → worker-sandbox**
in the **CSGHub Sandbox SaaS** layout (server and agents run as Hub
sandboxes; csgclaw is configured with `[sandbox].provider = csghub`).

**Compile-time variants:** this repo supports only:

- **`go build`** — default binary (BoxLite **CLI** backend plus CSGHub
  provider; pick the active backend in `config.toml` / deployment).
- **`go build -tags boxlite_sdk`** — same surface area, but BoxLite is
  linked via the **SDK** (CGO + native library) instead of the CLI.

There is **no** separate `csghub` build tag. Variables in §2–§6 matter
when the deployment uses the CSGHub sandbox provider and this PVC/API
layout; pure local BoxLite (`provider = boxlite-cli` or `boxlite-sdk`)
does not need them.

It has two audiences:

- **csgbot** operators, who must populate the server sandbox's env.
- **csgclaw** maintainers, who must keep the manager / worker env
  injection code in sync when a new variable is added.

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
             sandbox.Client.Create(...)│  (env = hubDownstreamEnv │
                                       │   + picoclawBoxEnvVars;  │
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
  python-sandbox. Runs in manager and worker pods. csgbot puts this
  tag into `CSGCLAW_SANDBOX_IMAGE`; the csgclaw server propagates it
  into every manager/worker `sandbox.CreateRequest`.

Both images are built in the `sandbox-runtime` repo under
`csgclaw-server/` and `csgclaw-agent/` respectively. No
`CSGCLAW_ROLE`: the image identity carries the role.

Manager vs worker are both the same `csgclaw-agent-sandbox` image
and differ only by sandbox name. The Hub name is always
`csgclaw-<tid>-<agent_id>` where `<agent_id>` is the persisted agent id;
for example `csgclaw-<tid>-u-manager` for the bootstrap manager
(`ManagerUserID`) and `csgclaw-<tid>-u-<name>` for a typical worker
created via `CreateWorker`, or any other non-empty agent id string
(convention enforced where the CSGHub provider and agent service agree
on sandbox naming).

## 2. Server sandbox env (injected by csgbot)

### 2.1 Deployment identity (required)

| Variable | Purpose | Consumer |
|----------|---------|----------|
| `CSGCLAW_TENANT_ID` | Tenant id used in sandbox names (`csgclaw-<tid>-<agent>`) and PVC subpaths. | `internal/agent/env_csghub.go` |
| `CSGCLAW_PVC_MOUNT_PATH` | Directory where `<tid>/` subpath of the shared PVC is mounted. Default `/opt/csgclaw`. | `internal/config/dir_csghub.go`, `paths_csghub.go` |
| `CSGCLAW_STATE_DIR` *(optional)* | Overrides the server-state dir. Default: `$CSGCLAW_PVC_MOUNT_PATH/server-state`. | `internal/config/dir_csghub.go` |

### 2.2 CSGHub API credentials (required)

| Variable | Purpose |
|----------|---------|
| `CSGHUB_API_BASE_URL` | Base URL of the CSGHub Sandbox REST API, e.g. `https://hub.example.com`. |
| `CSGHUB_USER_TOKEN` | Bearer token used for all sandbox lifecycle calls. **Never forwarded to workers with create-scope.** |
| `CSGHUB_AIGATEWAY_URL` *(optional)* | Overrides the AI-gateway endpoint if it differs from `CSGHUB_API_BASE_URL`. Forwarded to workers. |
| `CSGHUB_USER_NAME` *(optional)* | Forwarded to workers so skills-sync can attribute cached artefacts. |

### 2.3 Sandbox template parameters (image required, others optional)

Used to build every `sandbox.CreateRequest` the server issues for a
manager or worker sandbox.

Current code-level contract (`internal/agent/env_csghub.go#loadSandboxParams`):

- Required: `CSGCLAW_SANDBOX_IMAGE`
- Optional (parsed when set): `CSGCLAW_RESOURCE_ID`, `CSGCLAW_CLUSTER_ID`,
  `CSGCLAW_SANDBOX_PORT`, `CSGCLAW_SANDBOX_TIMEOUT`

| Variable | Purpose |
|----------|---------|
| `CSGCLAW_SANDBOX_IMAGE` | Fully-qualified image reference for manager / worker sandboxes. Must point at a **`csgclaw-agent-sandbox`** tag built in `sandbox-runtime/csgclaw-agent/`. The server pod uses a different image (`csgclaw-server-sandbox`) picked directly by csgbot in its own Deployment — it is **not** set via this variable. |
| `CSGCLAW_RESOURCE_ID` *(int, optional)* | CSGHub resource spec id (CPU/RAM class). |
| `CSGCLAW_CLUSTER_ID` *(optional)* | Cluster identifier in CSGHub. |
| `CSGCLAW_SANDBOX_PORT` *(int, optional)* | Port exposed by picoclaw gateway; default set by Hub. |
| `CSGCLAW_SANDBOX_TIMEOUT` *(int seconds, optional)* | Sandbox idle timeout. |
| `CSGCLAW_SANDBOX_READY_TIMEOUT` *(duration or int seconds, optional)* | Max wall time `reconcileSandbox` waits for the Hub-reported state to converge on `Running` before failing. Default `5m`; clamped at a `5s` minimum. Accepts both Go duration (`90s`, `2m30s`) and bare integer seconds. |
| `CSGCLAW_SANDBOX_POLL_INTERVAL` *(duration or int seconds, optional)* | Cadence of the readiness poll. Default `3s`; clamped to `[500ms, 30s]`. |

### 2.4 `[server]` tuple (listen / advertise / access token)

The three env vars below map 1:1 onto the `[server]` block in
`config.toml`. Auto-onboard writes them on first boot; every
subsequent startup reads the env and overrides the in-memory config
(file on disk is preserved). CLI `--endpoint` still wins over the env.

| Env                           | `config.toml` field            | Purpose |
|-------------------------------|--------------------------------|---------|
| `CSGCLAW_LISTEN`              | `server.listen_addr`           | Bind address inside the pod, default `0.0.0.0:18080`. |
| `CSGCLAW_ADVERTISE_BASE_URL`  | `server.advertise_base_url`    | URL manager/worker sandboxes use to reach the server (becomes `CSGCLAW_BASE_URL` in their env). |
| `CSGCLAW_ACCESS_TOKEN`        | `server.access_token`          | Bearer required by `/api/*` incl. the LLM bridge. |

`server.advertise_base_url` has a precedence chain implemented in
`resolveManagerBaseURL` (`internal/agent/manager_url_csghub.go`):

1. explicit value from config/env/CLI, else
2. `POD_IP` (K8s Downward API) + listen port, else
3. first non-loopback IPv4 of the pod (last-resort, usually the
   docker bridge when running locally — not reachable from a
   remote CSGHub cluster).

There is **no `localhost` fallback** in this SaaS layout. If none
of these yields a URL the server fails fast at bot provisioning.

### 2.5 Auto-onboard (replaces the pre-run `csgclaw onboard` step)

The server container no longer requires a pre-baked `config.toml`; on
first start `csgclaw serve` reads the vars below and writes config +
IM state itself (`cli/serve/bootstrap.go#ensureAutoOnboard`). Once
the file exists on the PVC subsequent restarts skip this path (but
the `[server]` env overrides in §2.4 still apply).

| Variable | Purpose | Required |
|----------|---------|----------|
| `CSGCLAW_LLM_BASE_URL` | Upstream LLM provider base URL, e.g. `https://api.openai.com/v1`. | yes |
| `CSGCLAW_LLM_API_KEY` | Upstream LLM API key. | yes |
| `CSGCLAW_LLM_MODELS` | CSV of model ids; the first becomes the default profile (`provider/model`). | yes |
| `CSGCLAW_LLM_PROVIDER` | Provider key (default `default`). | no |
| `CSGCLAW_LLM_REASONING_EFFORT` | Optional upstream `reasoning_effort` default. | no |
| `CSGCLAW_DISABLE_AUTO_ONBOARD` | Set non-empty to fall back to the legacy "run `csgclaw onboard` first" flow. | no |

On first boot, `ensureAutoOnboard` copies `CSGCLAW_SANDBOX_IMAGE` (§2.3)
into `[bootstrap] manager_image` in the generated `config.toml` when the
variable is non-empty; otherwise the onboard default image applies. Use
only `CSGCLAW_SANDBOX_IMAGE` for the agent sandbox image — there is no
separate bootstrap-only override env.

If `config.toml` is missing **and** any of the three required vars is
empty, the server aborts with a clear error before the listener is
opened.

## 3. Manager sandbox env (injected by the server)

The server composes the manager env via `agentSandboxEnv`
(`internal/agent/box_csghub.go`). Same helper produces the worker
env — both pods use the identical `csgclaw-agent-sandbox` image and
identical env. Only the sandbox name differs.

| Group | Variable | Source |
|-------|----------|--------|
| Constant | `HOME=/home/picoclaw` | constant |
| Picoclaw ↔ server | `CSGCLAW_BASE_URL` | `resolveManagerBaseURL(server)` |
| Picoclaw ↔ server | `CSGCLAW_ACCESS_TOKEN` | `server.AccessToken` |
| Picoclaw ↔ server | `PICOCLAW_CHANNELS_CSGCLAW_BOT_ID` | per-agent |
| Picoclaw ↔ server | `CSGCLAW_LLM_BASE_URL` | `llmBridgeBaseURL(...)` |
| Picoclaw ↔ server | `CSGCLAW_LLM_MODEL_ID` | per-agent |
| Hub read-only | `CSGHUB_API_BASE_URL` | from server env |
| Hub read-only | `CSGHUB_USER_TOKEN` | from server env |
| Hub read-only | `CSGHUB_AIGATEWAY_URL` | from server env |
| Hub read-only | `CSGHUB_USER_NAME` *(if set)* | from server env |
| Feishu (optional) | `PICOCLAW_CHANNELS_FEISHU_APP_ID` / `PICOCLAW_CHANNELS_FEISHU_APP_SECRET` | `channels` config |

The manager does **not** receive `CSGHUB_*` with sandbox-create scope;
creating worker sandboxes always goes through the csgclaw server HTTP
API, gated by `CSGCLAW_ACCESS_TOKEN`.

## 4. Worker sandbox env

Identical to the manager (§3). Same helper (`agentSandboxEnv`,
`box_csghub.go`). The worker is distinguished from the manager only
by its sandbox name — the container image, env, and supervisor
program set are exactly the same.

## 5. Shared volumes

Every manager/worker sandbox mounts the following subpaths of the
tenant PVC (see `gatewayVolumeSpecs`):

| Sandbox path | PVC subpath (claim-relative) | Owner |
|--------------|------------------------------|-------|
| `/home/picoclaw/.picoclaw` | `<tid>/agents/<name>/.picoclaw` | server writes `config.json`, `.security.yml`; read by picoclaw |
| `/home/picoclaw/.picoclaw/workspace` | `<tid>/agents/<name>/workspace` | picoclaw read/write |
| `/home/picoclaw/.picoclaw/workspace/projects` | `<tid>/projects` | shared across all agents |

The server sandbox mounts `<tid>/` at `CSGCLAW_PVC_MOUNT_PATH`,
so the same filesystem looks like `<pvc_mount>/{agents,projects,server-state}/…`
from inside the server.

## 6. Networking contract

- Every sandbox (server + manager + worker) must share an overlay
  reachable by pod-IP or Hub service DNS; the server's advertised URL
  must resolve from inside manager/worker pods.
- Manager/worker pods must reach the server on
  `CSGCLAW_BASE_URL` (LLM bridge `/api/bots/<id>/llm`, worker spawn
  `/api/bots/<id>/workers`, health `/healthz`).
- Server pod must reach the CSGHub Sandbox API on
  `CSGHUB_API_BASE_URL` (TLS + bearer).
- Workers must reach the AI gateway on `CSGHUB_AIGATEWAY_URL` if skills
  require it.

## 7. csgbot checklist

Before invoking `POST /sandbox` for a csgclaw server, csgbot must
populate, at minimum:

- `CSGCLAW_TENANT_ID`, `CSGCLAW_PVC_MOUNT_PATH`
- `CSGHUB_API_BASE_URL`, `CSGHUB_USER_TOKEN`
- `CSGCLAW_SANDBOX_IMAGE` **→ must be a `csgclaw-agent-sandbox` tag**
- `CSGCLAW_RESOURCE_ID`, `CSGCLAW_CLUSTER_ID` *(optional but recommended)*
- `CSGCLAW_ADVERTISE_BASE_URL` **or** the K8s Downward API that populates `POD_IP`
- `CSGCLAW_ACCESS_TOKEN`
- `CSGCLAW_LLM_BASE_URL`, `CSGCLAW_LLM_API_KEY`, `CSGCLAW_LLM_MODELS`
  *(required unless the PVC already holds a `config.toml`; see 2.5)*

The server pod's own container image (`csgclaw-server-sandbox:<tag>`)
is picked by csgbot directly in the Deployment / CreateRequest spec —
not via any csgclaw env variable.

Missing required variables trigger a fast-fail at server startup; see
`internal/agent/env_csghub.go#loadSandboxParams`,
`internal/config/dir_csghub.go#DefaultDir`, and
`cli/serve/bootstrap.go#ensureAutoOnboard`.
