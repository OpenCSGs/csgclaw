# CSGClaw Configuration

English | [中文](config.zh.md)

`csgclaw serve` uses the local config file for server access, bootstrap image selection, sandbox isolation, and optional channels, and it auto-creates missing local state on first run. Agent LLM provider profiles are stored in agent state and managed from the Web UI.

## Server Address

`listen_addr` is the address that the local HTTP server binds to.

`advertise_base_url` is the base URL that CSGClaw gives to manager and worker boxes so they can call back into the local HTTP server. When it is set, CSGClaw uses it as-is after trimming a trailing slash and does not try to infer a host IP. When it is empty, CSGClaw falls back to an inferred local IPv4 address plus the configured listen port.

Use `advertise_base_url` when the automatically inferred address is not reachable from BoxLite boxes, such as when you need a LAN address, a tunnel URL, or a host alias.

`access_token` protects authenticated API routes, including the PicoClaw bot routes. When authentication is enabled, clients must send `Authorization: Bearer <access_token>`.

`no_auth` controls whether CSGClaw skips the bearer-token check. The default is `false`. Set it to `true` only for trusted local or development environments.

`show_upgrade` controls whether the Web UI shows upgrade actions. The default is `true`; set it to `false` only when the deployment cannot self-upgrade, such as managed Kubernetes environments.

String values in `config.toml` can reference environment variables with `${NAME}` or `$NAME`. CSGClaw expands them when loading the config and keeps the placeholder form when it later rewrites the same value. If an environment variable is not set, it expands to an empty string.

```toml
[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"
no_auth = false
show_upgrade = true
```

## Model Provider Examples

### Local CSGHub-lite

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false
show_upgrade = true

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF"]

[bootstrap]
manager_image_override = ""
runtime_kind = "picoclaw_sandbox"

[sandbox]
provider = "boxlite"
```

### Remote LLM API

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false
show_upgrade = true

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://api.openai.com/v1"
api_key = "sk-your-api-key"
models = ["gpt-5.4"]

[bootstrap]
manager_image_override = ""
runtime_kind = "picoclaw_sandbox"

[sandbox]
provider = "boxlite"
```

### Dynamic Codex or Claude Code profiles

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false
show_upgrade = true

[bootstrap]
manager_image_override = ""
runtime_kind = "picoclaw_sandbox"

[sandbox]
provider = "boxlite"
```

Codex and Claude Code profiles are configured in agent state through the Web UI. CSGClaw starts an embedded CLIProxyAPI on a private localhost port at serve time, so static CLIProxy base URLs are not required.

Workers can also select an explicit runtime kind when they are created. The default runtime kind is `picoclaw_sandbox`. To create a sandboxed OpenClaw worker, use `csgclaw agent create --runtime openclaw_sandbox ...`; to create a Codex worker, use `csgclaw agent create --runtime codex ...`. The API accepts the same values through `runtime_kind` on `POST /api/v1/agents`.

Leave `[bootstrap].manager_image_override` empty to use the built-in default manager image. Set it only when you need to override that default.
The bootstrap manager currently runs on `picoclaw_sandbox`; `openclaw_sandbox` is supported for workers, not as the manager runtime.

Auth is also managed locally:

- Codex auth is imported from `~/.codex/auth.json` when available.
- Claude Code auth is probed from macOS Keychain when available, then falls back to OAuth.
- At server startup, Codex and Claude Code auth are imported or refreshed into the CSGClaw-managed CLIProxy auth directory using CLIProxy-compatible JSON.
- Manual login commands are `csgclaw model auth login codex` and `csgclaw model auth login claude-code`.
- `CSGCLAW_CLIPROXY_AUTH_DIR` overrides the CLIProxy auth directory; the default is `~/.csgclaw/auth`.
- `CSGCLAW_CLIPROXY_AUTO_LOGIN=0` disables automatic import/probing.
- `CSGCLAW_CLIPROXY_NO_BROWSER=1` prints OAuth URLs instead of opening a browser.
- `CSGCLAW_CLIPROXY_DISABLE_KEYCHAIN=1` disables Claude Keychain probing.

When a worker uses the Codex runtime, its local state is stored under `~/.csgclaw/agents/<agent-name>/.codex/`. The workspace lives at `~/.csgclaw/agents/<agent-name>/.codex/workspace`, shell home lives at `~/.csgclaw/agents/<agent-name>/.codex/home`, and Codex-managed files such as `auth.json` are stored directly under `~/.csgclaw/agents/<agent-name>/.codex`. This path is intentionally separate from the sandbox provider home such as `~/.csgclaw/agents/<agent-name>/boxlite`.

For complete Codex worker profiles, CSGClaw writes `~/.csgclaw/agents/<agent-name>/.codex/config.toml` with an OpenAI-compatible proxy provider and always sets `wire_api = "responses"` because current `codex-acp` no longer accepts chat wire API providers. If the upstream provider reports the Responses endpoint as unsupported, CSGClaw keeps the Codex-facing Responses bridge and falls back to upstream chat completions behind that bridge. The raw upstream API key is not written to this file; it is injected into the runtime environment through `env_key = "OPENAI_API_KEY"`.

When a worker uses the Codex runtime, CSGClaw resolves `codex-acp` automatically before startup. You can override that behavior with:

- `CSGCLAW_CODEX_ACP_PATH` to point at a preinstalled `codex-acp` binary
- `CSGCLAW_CODEX_ACP_VERSION` to pin the download version
- `CSGCLAW_CODEX_ACP_BASE_URL` to change the download source

## OpenClaw Runtime

CSGClaw defaults to PicoClaw for the bootstrap manager. To create a sandboxed OpenClaw worker, set the worker runtime explicitly:

```bash
csgclaw agent create --name alice --runtime openclaw_sandbox
```

The recommended image shape is a slim OpenClaw base image with CSGClaw-managed plugins baked under `/home/node/openclaw-plugins` (for example, `csgclaw-extension` and external channel plugins). Runtime state still comes from `~/.csgclaw/agents/<agent>/.openclaw/openclaw.json`; do not mount an empty host directory over `/home/node/openclaw-plugins`, because that hides baked plugins.

## Sandbox Providers

CSGClaw runs Workers through the configured sandbox provider. Supported built-in providers are:

- `boxlite`: runs Workers through the local `boxlite` CLI.
- `docker`: runs Workers through the local Docker CLI.
- `csghub`: runs Workers in the remote CSGHub sandbox. This is currently supported only in [AgenticHub](https://opencsg.com/agentichub).

Official bundles use one of these layouts:

- `csgclaw/bin/csgclaw` plus `csgclaw/bin/boxlite`
- `csgclaw/bin/csgclaw` only

If `[sandbox].provider` is omitted or empty, CSGClaw chooses the default dynamically from the installed bundle:

- bundled `boxlite` present: default to `boxlite`
- bundled `boxlite` absent: default to `docker`

That means a generated config can keep the provider empty to follow the bundle default:

```toml
[sandbox]
provider = ""
```

You can always override the default explicitly:

```toml
[sandbox]
provider = "boxlite"
```

```toml
[sandbox]
provider = "docker"
```

For `provider = "boxlite"`, CSGClaw resolves the bundled sibling `boxlite` binary next to `csgclaw` first, then falls back to `PATH` if that bundle is missing. If neither exists, startup fails with an actionable error instead of silently rewriting the provider.

`debian_registries_override` controls where BoxLite pulls `debian:bookworm-slim` when you need to override the built-in default order. If omitted or empty, CSGClaw uses `harbor.opencsg.com` then `docker.io`. When CSGClaw writes `config.toml`, it keeps this field visible as an empty array so it can be edited in place:

```toml
[sandbox]
provider = "boxlite"
debian_registries_override = []
```

CSGClaw passes an explicit `--home` to the BoxLite CLI for each agent, using the fixed per-agent runtime home `~/.csgclaw/agents/<agent-id>/boxlite`. That explicit home takes precedence over `BOXLITE_HOME` for CSGClaw-managed sandboxes, while `BOXLITE_HOME` still applies when you run `boxlite` manually without `--home`.

The `boxlite` provider does not need a vendored Go SDK at runtime. Current source builds and release packaging use the same BoxLite CLI-backed integration:

- `make build`, `make test`, `make run`, and `make package` all use the standard `boxlite` path.
- `boxlite` remains the built-in BoxLite sandbox provider, alongside other non-BoxLite providers such as `csghub`.

To use Docker as the sandbox provider:

```toml
[sandbox]
provider = "docker"
```

When `provider = "docker"`, CSGClaw runs the local `docker` CLI. By default it resolves `docker` from `PATH`. If you need a specific binary, set `docker_cli_path`:

```toml
[sandbox]
provider = "docker"
docker_cli_path = "/usr/local/bin/docker"
```

Current platform expectations:

- Linux amd64, Linux arm64, and macOS arm64 official bundles include `boxlite`, so an empty provider resolves to `boxlite`.
- macOS amd64 and Windows amd64 official bundles do not include `boxlite`, so an empty provider resolves to `docker`.
- Windows users should have Docker installed and reachable on `PATH`, or set `[sandbox].docker_cli_path` explicitly.

## Hub Configuration

CSGClaw can read agent templates from one or more hub registries. Registry configuration is additive: built-in, local, and remote registries can coexist in the same `config.toml`.

When `[hub]` is omitted, CSGClaw enables three registries by default: `builtin` (read-only), `local` (writable publish target at `~/.csgclaw/hub`), and `official` (official remote at `https://csgclaw.opencsg.com`). If your `config.toml` lists only some registries, missing defaults are merged in automatically so removing `builtin` does not break startup.

```toml
[hub]
default_registry = "builtin"
default_publish_registry = "local"
default_manager_template = "builtin.picoclaw-manager"
default_worker_template = "builtin.picoclaw-worker"

[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

[[hub.registries]]
name = "local"
kind = "local"
path = "~/.csgclaw/hub"
enabled = true

[[hub.registries]]
name = "official"
kind = "remote"
url = "https://csgclaw.opencsg.com"
enabled = true

[[hub.registries]]
name = "team"
kind = "remote"
url = "https://hub.example.com"
token = "${CSGCLAW_HUB_TOKEN}"
enabled = true
```

Field behavior:

- `default_registry` selects the default source registry when a command needs one registry context.
- `default_publish_registry` selects the default publish target when a command does not pass a registry explicitly.
- `default_manager_template` selects the default manager template when a flow needs a manager template implicitly.
- `default_worker_template` selects the default worker template when a flow needs a worker template implicitly.
- `name` is the registry identifier used by CLI and API flows.
- `kind` is `builtin`, `local`, or `remote`.
- `path` is used by `local` registries.
- `url` and `token` are used by `remote` registries.
- `enabled` controls whether the registry participates in hub operations. If omitted, it defaults to `true`.

The built-in registry is read-only. Use a writable `local` or `remote` registry as the publish target.

## Skill registry configuration

`csgclaw skill` uses two skill registries by default:

- **opencsg** (primary): `https://claw.opencsg.com`
- **clawhub** (official): `https://clawhub.ai`

`skill search` queries opencsg first and returns immediately when there are hits; it only queries clawhub.ai when opencsg returns no results.

Use `--registry opencsg` or `--registry clawhub` on `get` / `install` to target one registry. Omit it to try opencsg first, then clawhub.

Supported registry APIs:

- `GET /api/v1/search` — `csgclaw skill search`
- `GET /api/v1/skills/:slug` — `csgclaw skill get` (includes `versions[]` on OpenCSG)
- `GET /api/v1/skills/:slug/versions` — `csgclaw skill versions` (paginated on clawhub.ai; OpenCSG falls back to `versions[]` on get)
- `GET /api/v1/skills/:slug/versions/:version` — `csgclaw skill get --version`
- `GET /api/v1/download/:slug` or `GET /api/v1/download?slug=` — install download

Browse skills with `search`; there is no catalog list endpoint on the current registry.

```toml
[skill]
base_url = "https://claw.opencsg.com"
official_base_url = "https://clawhub.ai"
token = "${SKILL_TOKEN}"
non_suspicious_only = true
```

- `base_url` is the primary (opencsg) registry. You can also set `SKILL_BASE_URL` (legacy: `CLAWHUB_BASE_URL`).
- `official_base_url` is the secondary (clawhub.ai) registry. Defaults to `https://clawhub.ai`. Set to `""` to disable dual-registry search. Override with `SKILL_OFFICIAL_BASE_URL` (legacy: `CLAWHUB_OFFICIAL_BASE_URL`).
- `token` is optional for read-only commands and required for future publish flows. You can also set `SKILL_TOKEN` (legacy: `CLAWHUB_TOKEN`).
- The legacy `[clawhub]` section is still read for backward compatibility.
- `non_suspicious_only` defaults to `true` when omitted.

## Channel Configuration

Channel integration is optional. CSGClaw works with the built-in Web UI by default, and you only need channel config when you want to connect external messaging platforms such as Feishu.

Keep `config.toml` focused on shared server, model, bootstrap, and sandbox settings. Feishu credentials live in a standalone `channels/feishu.toml` file next to the selected `config.toml`; legacy `[channels.feishu]` blocks in `config.toml` are not read.

For detailed field definitions and examples, see [Feishu Channel Configuration](channel/feishu.md).
