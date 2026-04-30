# CSGClaw CLI Reference

This document supplements the CLI section in [architecture.md](./architecture.md). The architecture doc describes the role split between `csgclaw` and `csgclaw-cli`, while this document records the currently implemented commands, flags, defaults, and behavior.

## CLI Positioning

`csgclaw` is the full local operator CLI. It manages onboarding, local server lifecycle, agent runtime operations, and the shared collaboration workflows.

`csgclaw-cli` is the lightweight HTTP client intended for bots, agents, and scripts. It exposes only collaboration-oriented workflows and does not manage onboarding, config files, or server lifecycle.

Both CLIs are thin HTTP clients over the local API. They do not talk to BoxLite, stores, or channel SDKs directly.

## Shared Conventions

### Output formats

- `--output table` prints human-readable tables or plain text.
- `--output json` prints structured JSON.
- If `--output` is omitted:
  - terminal output defaults to `table`
  - piped or redirected output defaults to `json`
- Special cases:
  - `csgclaw serve`, `csgclaw stop`, and `csgclaw agent logs` default to `table`
  - `csgclaw-cli --version` defaults to `table`

### Environment variables

Both CLIs support:

- `CSGCLAW_BASE_URL`: default API endpoint
- `CSGCLAW_ACCESS_TOKEN`: default API token

If `--endpoint` or `--token` is passed, the flag overrides the environment variable.

### Channels

Most collaboration commands accept `--channel` with:

- `csgclaw`
- `feishu`

Defaults to `csgclaw` unless stated otherwise.

### Config and local paths

`csgclaw` additionally supports `--config` and reads `~/.csgclaw/config.toml` by default.

Common local defaults:

- config: `~/.csgclaw/config.toml`
- daemon log: `~/.csgclaw/server.log`
- daemon PID: `~/.csgclaw/server.pid`
- agents state: `~/.csgclaw/agents/state.json`
- built-in IM state: `~/.csgclaw/im/state.json`

## `csgclaw`

### Global flags

Usage:

```bash
csgclaw [global-flags] <command> [args]
```

Global flags:

- `--endpoint string`: HTTP server endpoint. Default from `CSGCLAW_BASE_URL`.
- `--token string`: API authentication token. Default from `CSGCLAW_ACCESS_TOKEN`.
- `--output string`: `table` or `json`.
- `--config string`: path to config file.
- `--version`, `-V`: print version and exit.

Top-level commands:

- `onboard`
- `serve`
- `stop`
- `agent`
- `model`
- `user`
- `bot`
- `room`
- `member`
- `message`
- `completion`

### Shell completion

Both CLIs can generate shell completion scripts for `bash`, `zsh`, and `fish`.

Examples:

```bash
csgclaw completion bash
csgclaw completion zsh
csgclaw completion fish
```

### `csgclaw onboard`

Initializes local config and bootstrap state.

Usage:

```bash
csgclaw onboard [flags]
```

Flags:

- `--debian-registries string`: comma-separated OCI registries for `debian:bookworm-slim` pulls. Persisted to config.
- `--log-level string`: log level for onboarding logs. Supported values: `debug`, `info`, `warn`, `error`. Default `info`.

Behavior:

- It writes config, ensures bootstrap IM state, and ensures the bootstrap manager bot.
- It does not prompt for model provider settings. Manager and worker LLM profiles are detected at startup and managed in the Web UI.
- If `--debian-registries` is set, it updates `sandbox.debian_registries` in config.
- If profile auto-detection fails, `serve` still starts and the Web UI asks an admin to complete the Manager profile.

Examples:

```bash
csgclaw onboard
csgclaw onboard --debian-registries "harbor.opencsg.com,docker.io"
```

### `csgclaw serve`

Starts the local HTTP server.

Usage:

```bash
csgclaw serve [-d|--daemon] [flags]
```

Flags:

- `--daemon`, `-d`: run in background.
- `--log-level string`: log level. Supported values: `debug`, `info`, `warn`, `error`. Default `info`.
- `--log string`: daemon log path. Daemon mode only. Default `~/.csgclaw/server.log`.
- `--pid string`: daemon PID path. Daemon mode only. Default `~/.csgclaw/server.pid`.

Behavior:

- Loads config from `--config` or `~/.csgclaw/config.toml`.
- Validates effective model configuration before startup.
- For `csghub-lite`, it performs a provider reachability preflight.
- In foreground mode it prints the effective config and IM URL.
- In daemon mode it launches the hidden internal `_serve` entrypoint and waits for `/healthz`.

Examples:

```bash
csgclaw serve
csgclaw serve --daemon
csgclaw serve --config /path/to/config.toml
csgclaw --endpoint http://127.0.0.1:18080 serve
```

### `csgclaw stop`

Stops the daemonized local server.

Usage:

```bash
csgclaw stop [flags]
```

Flags:

- `--pid string`: PID file path. Default `~/.csgclaw/server.pid`.

Behavior:

- Sends `SIGTERM` to the PID stored in the PID file.
- If the process is already gone, it removes the stale PID file and reports that state.

### `csgclaw model auth`

Manages local Codex and Claude Code authentication for model providers through the embedded CLIProxyAPI.

Usage:

```bash
csgclaw model auth login <provider> [flags]
```

Providers:

- `codex`
- `claude-code`

Flags:

- `--no-browser`: print the OAuth URL instead of opening a browser.

Behavior:

- `codex` first reuses `~/.codex/auth.json` when available, then starts OAuth if needed.
- `claude-code` first probes macOS Keychain when available, then starts OAuth if needed.
- Auth is stored in the CLIProxy auth directory, defaulting to `~/.cli-proxy-api`.
- Model provider auth is scoped under `csgclaw model auth`, not the server's own API authentication.

Examples:

```bash
csgclaw model auth login codex
csgclaw model auth login claude-code --no-browser
```

### `csgclaw agent`

Manages runtime agents.

Usage:

```bash
csgclaw agent <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `start`
- `stop`
- `delete`
- `logs`

#### `csgclaw agent list`

Usage:

```bash
csgclaw agent list [flags]
```

Flags:

- `--filter string`: filter by agent state after listing.

#### `csgclaw agent create`

Usage:

```bash
csgclaw agent create [flags]
csgclaw agent create [-r|--replace] --id <id> [flags]
```

Flags:

- `--replace`, `-r`: replace an existing agent in place.
- `--force`, `-f`: skip confirmation when replacing an existing agent.
- `--id string`: agent ID.
- `--name string`: agent name.
- `--description string`: agent description.
- `--image string`: agent image.
- `--profile string`: agent LLM profile.

Behavior:

- Without `--replace`, the command creates a new agent.
- With `--replace`, `--id` is required.
- With `--replace`, the CLI sends a single create request with `replace: true` and a `field_mask` containing the flags explicitly passed this time.
- The API/service loads the existing agent, preserves unmasked fields, applies masked fields, then recreates the agent.
- Replacing an agent prompts for confirmation unless `--force` is set.
- `image` follows the same field-mask behavior: omitted keeps the old image, explicit `--image` overrides it.

#### `csgclaw agent delete`

Usage:

```bash
csgclaw agent delete <id>
csgclaw agent delete --all [-f|--force]
```

Flags:

- `--all`, `-a`: delete all agents.
- `--force`, `-f`: skip confirmation when deleting all agents.

Behavior:

- Without `--all`, exactly one agent ID is required.
- With `--all`, no positional ID is allowed.
- Bulk delete prompts for confirmation unless `--force` is set.

#### `csgclaw agent start`

Usage:

```bash
csgclaw agent start <id>
```

#### `csgclaw agent stop`

Usage:

```bash
csgclaw agent stop <id>
```

#### `csgclaw agent logs`

Usage:

```bash
csgclaw agent logs <id> [-f|--follow] [-n lines]
```

Flags:

- `-f`, `--follow`: stream logs continuously.
- `-n int`: number of lines to fetch. Default `20`.

Behavior:

- `-n` must be greater than `0`.
- `--output json` is supported only for non-follow mode.
- `--output json --follow` returns an error.

Examples:

```bash
csgclaw agent list
csgclaw agent list --filter running
csgclaw agent create --name alice --description "frontend worker" --profile openai.gpt-5.4-mini
csgclaw agent create -r --id agent-alice
csgclaw agent create --replace --id agent-alice --name alice-v2 --profile openai.gpt-5.4-mini --force
csgclaw agent start agent-alice
csgclaw agent stop agent-alice
csgclaw agent logs agent-alice -n 50
csgclaw agent logs agent-alice --follow
csgclaw agent delete agent-alice
csgclaw agent delete --all --force
```

### `csgclaw user`

Manages channel users.

Usage:

```bash
csgclaw user <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`

#### `csgclaw user list`

Usage:

```bash
csgclaw user list [flags]
```

Flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

#### `csgclaw user create`

Usage:

```bash
csgclaw user create [flags]
```

Flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--id string`: user ID.
- `--name string`: user name.
- `--handle string`: user handle.
- `--role string`: user role.
- `--avatar string`: avatar initials. Used only for `feishu`.

Behavior:

- `--name` is required.
- `csgclaw` and `feishu` use different backend routes and payload shapes.

#### `csgclaw user delete`

Usage:

```bash
csgclaw user delete <id> [flags]
```

Flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

Examples:

```bash
csgclaw user list
csgclaw user list --channel feishu
csgclaw user create --name Alice --handle alice --role worker
csgclaw user create --channel feishu --name Alice --handle alice --role manager --avatar AL
csgclaw user delete u-alice
```

### Shared collaboration groups in `csgclaw`

The following command groups are shared with `csgclaw-cli` and use the same flags and semantics.

#### `bot`

Usage:

```bash
csgclaw bot <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`

`bot list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--role string`: filter by `manager` or `worker`.

`bot create` flags:

- `--id string`: bot ID.
- `--name string`: required.
- `--description string`: bot description.
- `--role string`: required. `manager` or `worker`.
- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--model-id string`: agent model ID.

`bot delete` usage and flags:

```bash
csgclaw bot delete <id> [flags]
```

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

#### `room`

Usage:

```bash
csgclaw room <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `delete`

`room list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

`room create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--title string`: room title.
- `--description string`: room description.
- `--creator-id string`: creator user ID.
- `--member-ids string`: comma-separated member IDs.
- `--locale string`: room locale.

`room delete` usage and flags:

```bash
csgclaw room delete <id> [flags]
```

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.

#### `member`

Usage:

```bash
csgclaw member <subcommand> [flags]
```

Subcommands:

- `list`
- `create`

`member list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: target room ID.

`member create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: target room ID.
- `--user-id string`: required. User to add.
- `--inviter-id string`: inviter user ID.
- `--locale string`: room locale.

`member create` behavior:

- `--user-id` is required.

#### `message`

Usage:

```bash
csgclaw message <subcommand> [flags]
```

Subcommands:

- `list`
- `create`

`message list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: required.

`message create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--room-id string`: required.
- `--sender-id string`: required.
- `--content string`: required.
- `--mention-id string`: optional mentioned user ID.

`message list` behavior:

- `--room-id` is required.

Examples:

```bash
csgclaw bot list
csgclaw bot create --name alice --role worker --model-id gpt-5.4-mini
csgclaw room create --title "release-room" --creator-id admin --member-ids admin,manager
csgclaw member create --room-id room-1 --user-id u-alice --inviter-id admin
csgclaw message list --room-id room-1
csgclaw message create --channel csgclaw --room-id room-1 --sender-id admin --content hello
```

## `csgclaw-cli`

### Global flags

Usage:

```bash
csgclaw-cli [global-flags] <command> [args]
```

Global flags:

- `--endpoint string`: HTTP server endpoint. Default from `CSGCLAW_BASE_URL`.
- `--token string`: API authentication token. Default from `CSGCLAW_ACCESS_TOKEN`.
- `--output string`: `table` or `json`.
- `--version`, `-V`: print version and exit.

Top-level commands:

- `bot`
- `room`
- `member`
- `message`
- `completion`

### Shell completion

Examples:

```bash
csgclaw-cli completion bash
csgclaw-cli completion zsh
csgclaw-cli completion fish
```

### Command groups

`csgclaw-cli` reuses the same implementations as `csgclaw` for:

- `bot list`
- `bot create`
- `bot delete`
- `room list`
- `room create`
- `room delete`
- `member list`
- `member create`
- `message list`
- `message create`

That means flags, defaults, validations, and JSON shapes are identical between the two CLIs for those groups.

Examples:

```bash
csgclaw-cli bot list --channel feishu
csgclaw-cli bot create --name manager --role manager --channel feishu
csgclaw-cli room create --channel feishu --title "ops-room"
csgclaw-cli member list --channel feishu --room-id oc_x
csgclaw-cli message create --channel feishu --room-id oc_x --sender-id u-manager --content hello
```
