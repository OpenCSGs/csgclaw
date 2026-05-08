# CSGClaw Configuration

English | [中文](config.zh.md)

`csgclaw serve` uses the local config file for server access, bootstrap image selection, sandbox isolation, and optional channels, and it auto-creates missing local state on first run. Agent LLM provider profiles are stored in agent state and managed from the Web UI.

## Server Address

`listen_addr` is the address that the local HTTP server binds to.

`advertise_base_url` is the base URL that CSGClaw gives to manager and worker boxes so they can call back into the local HTTP server. When it is set, CSGClaw uses it as-is after trimming a trailing slash and does not try to infer a host IP. When it is empty, CSGClaw falls back to an inferred local IPv4 address plus the configured listen port.

Use `advertise_base_url` when the automatically inferred address is not reachable from BoxLite boxes, such as when you need a LAN address, a tunnel URL, or a host alias.

`access_token` protects authenticated API routes, including the PicoClaw bot routes. When authentication is enabled, clients must send `Authorization: Bearer <access_token>`.

`no_auth` controls whether CSGClaw skips the bearer-token check. The default is `false`. Set it to `true` only for trusted local or development environments.

String values in `config.toml` can reference environment variables with `${NAME}` or `$NAME`. CSGClaw expands them when loading the config and keeps the placeholder form when it later rewrites the same value. If an environment variable is not set, it expands to an empty string.

```toml
[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"
no_auth = false
```

## Model Provider Examples

### Local CSGHub-lite

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF"]

[bootstrap]
manager_image_override = ""

[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
```

### Remote LLM API

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://api.openai.com/v1"
api_key = "sk-your-api-key"
models = ["gpt-5.4"]

[bootstrap]
manager_image_override = ""

[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
```

### Dynamic Codex or Claude Code profiles

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[bootstrap]
manager_image_override = ""

[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
```

Codex and Claude Code profiles are configured in agent state through the Web UI. CSGClaw starts an embedded CLIProxyAPI on a private localhost port at serve time, so static CLIProxy base URLs are not required.

Workers can also select an explicit runtime kind when they are created. The default runtime kind is `picoclaw-sandbox`. To create a Codex worker, use `csgclaw agent create --runtime codex ...` or send `runtime_kind: "codex"` to `POST /api/v1/agents`.

Leave `[bootstrap].manager_image_override` empty to use the built-in default manager image. Set it only when you need to override that default.

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

When a worker uses the Codex runtime, CSGClaw resolves `codex-acp` automatically before startup. You can override that behavior with:

- `CSGCLAW_CODEX_ACP_PATH` to point at a preinstalled `codex-acp` binary
- `CSGCLAW_CODEX_ACP_VERSION` to pin the download version
- `CSGCLAW_CODEX_ACP_BASE_URL` to change the download source

## Sandbox Providers

CSGClaw runs Workers through the configured sandbox provider. BoxLite integration is provided through `boxlite-cli`, which runs BoxLite through the external CLI process.

The default source build and official release bundles already align with the CLI-backed provider:

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
```

For `provider = "boxlite-cli"`, CSGClaw resolves the bundled sibling `boxlite` binary next to `csgclaw` first, then falls back to `PATH` if that bundle is missing.

`debian_registries_override` controls where BoxLite pulls `debian:bookworm-slim` when you need to override the built-in default order. If omitted or empty, CSGClaw uses `harbor.opencsg.com` then `docker.io`. When CSGClaw writes `config.toml`, it keeps this field visible as an empty array so it can be edited in place:

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
debian_registries_override = []
```

CSGClaw passes an explicit `--home` to the BoxLite CLI for each agent, using the agent directory plus `home_dir_name` such as `~/.csgclaw/agents/<agent-id>/boxlite`. That explicit home takes precedence over `BOXLITE_HOME` for CSGClaw-managed sandboxes, while `BOXLITE_HOME` still applies when you run `boxlite` manually without `--home`.

The `boxlite-cli` provider does not need a vendored Go SDK at runtime. Current source builds and release packaging use the same BoxLite CLI-backed integration:

- `make build`, `make test`, `make run`, and `make package` all use the standard `boxlite-cli` path.
- `boxlite-cli` remains the built-in BoxLite sandbox provider, alongside other non-BoxLite providers such as `csghub`.

## Channel Configuration

Channel integration is optional. CSGClaw works with the built-in Web UI by default, and you only need channel config when you want to connect external messaging platforms such as Feishu.

Channel-specific settings live under top-level config sections such as `channels.feishu`. Keep the main config focused on shared server, model, bootstrap, and sandbox settings, then add only the channel blocks you actually use.

For detailed field definitions and examples, see [Feishu Channel Configuration](channel/feishu.md).
