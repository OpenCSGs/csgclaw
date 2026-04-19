# CSGClaw Configuration

English | [中文](config.zh.md)

`csgclaw onboard` writes the local config file used by `csgclaw serve`. The config covers server access, model providers, bootstrap image selection, sandbox isolation, and optional channels.

## Bootstrap Source

By default, CSGClaw starts manager and worker boxes from `manager_image`, which is a normal OCI image reference.

You can also point bootstrap to a preloaded local rootfs bundle by setting `manager_rootfs_path` instead. This is useful for mirrored or air-gapped deployments where PicoClaw is prepared locally ahead of time.

`manager_image` and `manager_rootfs_path` are mutually exclusive.

## Server Address

`listen_addr` is the address that the local HTTP server binds to.

`advertise_base_url` is the base URL that CSGClaw gives to manager and worker boxes so they can call back into the local HTTP server. When it is set, CSGClaw uses it as-is after trimming a trailing slash and does not try to infer a host IP. When it is empty, CSGClaw falls back to an inferred local IPv4 address plus the configured listen port.

Use `advertise_base_url` when the automatically inferred address is not reachable from BoxLite boxes, such as when you need a LAN address, a tunnel URL, or a host alias.

## Model Provider Examples

### Local CSGHub-lite

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF"]

[bootstrap]
manager_image = "ghcr.io/russellluo/picoclaw:2026.4.18"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

### Remote LLM API

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://api.openai.com/v1"
api_key = "sk-your-api-key"
models = ["gpt-5.4"]

[bootstrap]
manager_image = "ghcr.io/russellluo/picoclaw:2026.4.18"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

### Local Codex via CLIProxyAPI

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"

[models]
default = "codex.gpt-5.4"

[models.providers.codex]
base_url = "http://127.0.0.1:8317/v1"
api_key = "local"
models = ["gpt-5.4"]

[bootstrap]
manager_image = "ghcr.io/russellluo/picoclaw:2026.4.18"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

## Sandbox Providers

CSGClaw runs Workers through the configured sandbox provider. The default is `boxlite`, which uses the vendored BoxLite Go SDK.

You can opt in to the CLI-backed provider when you already have the `boxlite` binary installed:

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

`boxlite_cli_path` is the executable path used only by `provider = "boxlite-cli"`. The default value, `boxlite`, resolves from `PATH`; set an absolute path if the binary is installed elsewhere.

CSGClaw passes an explicit `--home` to the BoxLite CLI for each agent, using the agent directory plus `home_dir_name` such as `~/.csgclaw/agents/<agent-id>/boxlite`. That explicit home takes precedence over `BOXLITE_HOME` for CSGClaw-managed sandboxes, while `BOXLITE_HOME` still applies when you run `boxlite` manually without `--home`.

The `boxlite-cli` provider does not need the vendored Go SDK at runtime. Source builds and the current default `boxlite` provider still use the SDK path, so commands such as `make test` may fetch or link the vendored BoxLite native library.

`manager_rootfs_path` currently works with the default `boxlite` provider. The `boxlite-cli` provider still expects an image reference.

## Worker Override Example

```json
{
  "id": "u-reviewer",
  "name": "reviewer",
  "description": "code review worker",
  "profile": "codex.gpt-5.4",
  "role": "worker"
}
```
