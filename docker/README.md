# CSGClaw base container image

This directory hosts the **base image** used by the CSGHub Sandbox
deployment. The base image intentionally contains only csgclaw-owned
assets:

- picoclaw upstream OS base (Alpine 3.23 + python3.12 + picoclaw
  gateway binary),
- `csgclaw` server (Go, `-tags csghub`),
- `csgclaw-cli`.

It does **not** ship supervisor, does not embed `main.py` /
`skills_sync.py` from sandbox-runtime, and does not install pip
packages. Those runtime-layer concerns live in the `sandbox-runtime`
repo, split into two images by pod role (no `CSGCLAW_ROLE` runtime
switch — the image identity carries the role):

- `csgclaw-server-sandbox:<tag>` — layered **on top of this base image**;
  runs only in the server pod. Supervisor runs `csgclaw serve` +
  `python-sandbox`.
- `csgclaw-agent-sandbox:<tag>` — layered directly on **upstream
  picoclaw** (no csgclaw Go binary); runs in manager and worker pods.
  Supervisor runs `picoclaw gateway` + `python-sandbox` +
  `llm-bridge-probe`.

```
┌──────────────────────────────────────┐   ┌──────────────────────────────────────┐
│ csgclaw-server-sandbox:<tag>         │   │ csgclaw-agent-sandbox:<tag>          │
│ (sandbox-runtime/csgclaw-server/)    │   │ (sandbox-runtime/csgclaw-agent/)     │
│   + supervisor + server.conf         │   │   + supervisor + agent.conf          │
│     (csgclaw-server + python-sandbox)│   │     (probe + picoclaw + python-sb)   │
│   + pip install requirements-minimal │   │   + pip install requirements-minimal │
├──────────────────────────────────────┤   ├──────────────────────────────────────┤
│ csgclaw:<tag>          (this repo)   │   │ picoclaw:<upstream-tag>              │
│   + /usr/local/bin/csgclaw           │   │   + Alpine 3.23 + python3.12         │
│   + /usr/local/bin/csgclaw-cli       │   │   + /usr/local/bin/picoclaw          │
├──────────────────────────────────────┤   └──────────────────────────────────────┘
│ picoclaw:<upstream-tag>              │
└──────────────────────────────────────┘
```

In `docs/saas-env-contract.md`:
- `CSGCLAW_SANDBOX_IMAGE` → must point at a `csgclaw-agent-sandbox`
  tag (used for manager / worker sandboxes);
- the server pod image is picked directly by csgbot in its Deployment
  (typically `csgclaw-server-sandbox:<tag>`), not via any csgclaw env.

## Layout

| File                  | Role                                                |
|-----------------------|-----------------------------------------------------|
| `Dockerfile.unified`  | Multi-stage build producing the `csgclaw` base tag. |

## Build

The build context must be the repo root. `.dockerignore` already
excludes the 229 MB BoxLite CGO archives (unused in the csghub build)
and the local `bin/`, `bin-csghub/` outputs.

```bash
DOCKER_BUILDKIT=1 docker build \
  -f docker/Dockerfile.unified \
  --build-arg PICOCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsg_public/picoclaw:2026041901 \
  -t csgclaw:csghub .
```

Or via the repo Makefile:

```bash
make docker-build          # local build
make docker-publish        # build + push to the configured registry
```

The resulting image is ~70 MB (picoclaw 35 MB + the two Go binaries).

## Why this layering?

Historically this Dockerfile also installed `supervisor`, `py3-pip`,
`fastapi` / `uvicorn`, and copied `third_party/sandbox-runtime/` into
the image. That coupled csgclaw to sandbox-runtime versioning and
forced a full csgclaw rebuild whenever a Python dep moved. Splitting
the runtime layer into the sandbox-runtime repo keeps each repo
responsible for exactly one thing:

- **csgclaw** owns the Go server and the env contract it exposes
  (`CSGCLAW_SANDBOX_IMAGE`, `CSGCLAW_TENANT_ID`, etc).
  `internal/agent/box_csghub.go#agentSandboxEnv` builds the env for
  manager / worker sandboxes. The runtime-role switch
  `CSGCLAW_ROLE` was removed: the role is now the image identity,
  not a runtime variable.
- **sandbox-runtime** owns `main.py`, `skills_sync.py`, and the
  supervisor glue. It ships **two** images (`csgclaw-server-sandbox`
  on top of the csgclaw base, and `csgclaw-agent-sandbox` directly
  on top of picoclaw) instead of one multi-role image.

## Running the base image standalone

Useful for local debugging of the Go server without the python-sandbox
side-car. The default `CMD` is `csgclaw serve`.

```bash
mkdir -p /tmp/csgclaw-smoke/server-state

docker run -d --name csgclaw-server \
  -v /tmp/csgclaw-smoke:/opt/csgclaw \
  -e CSGCLAW_PVC_MOUNT_PATH=/opt/csgclaw \
  -e CSGCLAW_TENANT_ID=smoke \
  -e CSGHUB_API_BASE_URL=https://hub.example.com \
  -e CSGHUB_USER_TOKEN=<hub-token> \
  -e CSGCLAW_SANDBOX_IMAGE=csgclaw-agent-sandbox:<tag> \
  -e CSGCLAW_RESOURCE_ID=<resource-id> \
  -e CSGCLAW_CLUSTER_ID=<cluster-id> \
  -e CSGCLAW_ADVERTISE_BASE_URL=http://<server-host>:18080 \
  -e CSGCLAW_ACCESS_TOKEN=<pick-a-random-token> \
  -e CSGCLAW_LLM_BASE_URL=https://llm.example.com \
  -e CSGCLAW_LLM_API_KEY=<llm-key> \
  -e CSGCLAW_LLM_MODELS=gpt-5.4-medium \
  -p 18080:18080 \
  csgclaw:csghub
```

- Port **18080** — csgclaw server (HTTP API + Web UI).

For the full SaaS smoke test (python-sandbox + supervisor + either
csgclaw-server or picoclaw), pull the relevant sandbox-runtime image
(`csgclaw-server-sandbox:<tag>` or `csgclaw-agent-sandbox:<tag>`)
and follow `sandbox-runtime/csgclaw-server/README.md` /
`sandbox-runtime/csgclaw-agent/README.md`.

## Notes

- `CSGCLAW_ROLE` no longer exists. The server / manager / worker
  distinction is encoded directly in the image identity (see the
  layering diagram above). `internal/agent/box_csghub.go#agentSandboxEnv`
  produces a single env set for both manager and worker sandboxes;
  the two are differentiated only by sandbox name.
- `CSGHUB_API_BASE_URL` + `CSGHUB_USER_TOKEN` must point at a reachable
  Hub instance; the server calls `EnsureBootstrapManager` on start-up
  and aborts if the probe fails.
- There is no `localhost` fallback for `CSGCLAW_ADVERTISE_BASE_URL` in
  the csghub build — set it explicitly or provide `POD_IP` (K8s
  downward API) so manager / worker sandboxes can reach the server.
- Auto-onboard writes `config.toml` to the resolved state dir
  (`CSGCLAW_STATE_DIR` or `$CSGCLAW_PVC_MOUNT_PATH/server-state`).
  Delete that file and restart to force a re-onboard; keep it on the
  PVC to pin the runtime config across container restarts.
