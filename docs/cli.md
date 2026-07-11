# CSGClaw CLI Reference

This document supplements the CLI section in [architecture.md](./architecture.md). The architecture doc describes the role split between `csgclaw` and `csgclaw-cli`, while this document records the currently implemented commands, flags, defaults, and behavior.

## CLI Positioning

`csgclaw` is the full local operator CLI. It manages local server lifecycle, agent runtime operations, and the shared collaboration workflows.

`csgclaw-cli` is the lightweight HTTP client intended for participants, agents, and scripts. It exposes only collaboration-oriented workflows and does not manage config files or server lifecycle.

Both CLIs are thin HTTP clients over the local API. They do not talk to BoxLite, stores, or channel SDKs directly.

## Shared Conventions

### Output formats

- `--output table` prints human-readable tables or plain text.
- `--output json` prints structured JSON.
- `-o` is a shorthand for `--output`.
- Global flags such as `--output`, `-o`, `--endpoint`, and `--token` must appear **before** the subcommand (for example `csgclaw-cli --output json template list`, not `csgclaw-cli template list --output json`).
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
- root state for agents, participants, model providers, and teams: `~/.csgclaw/state.json`
- task state: `~/.csgclaw/tasks`
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

- `serve`
- `stop`
- `upgrade`
- `agent`
- `model`
- `participant`
- `pt`
- `user`
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

### `csgclaw serve`

Starts the local HTTP server.

Usage:

```bash
csgclaw serve [-d|--daemon] [flags]
```

Flags:

- `--daemon`, `-d`: run in background.
- `--no-browser`: do not open the browser after startup.
- `--no-auth-detect`: disable startup auth/model auto-detection so the Manager Profile setup flow remains incomplete for manual testing.
- `--no-codex-auto-install`: start without automatically installing Codex CLI. Runtime status and manual installation from the Computer page remain available.
- `--log-level string`: log level. Supported values: `debug`, `info`, `warn`, `error`. Default `info`.
- `--log string`: daemon log path. Daemon mode only. Default `~/.csgclaw/server.log`.
- `--pid string`: daemon PID path. Daemon mode only. Default `~/.csgclaw/server.pid`.

Behavior:

- Loads config from `--config` or `~/.csgclaw/config.toml`.
- If local config or bootstrap state is incomplete, it auto-initializes local state before startup.
- Validates effective model configuration before startup.
- For `csghub-lite`, it performs a provider reachability preflight.
- With `--no-auth-detect`, startup skips automatic CLI auth import and Manager Profile provider/model detection unless an existing complete Manager Profile is already saved.
- With `--no-codex-auto-install`, startup skips only the Codex CLI installation attempt; the Computer page can still install or retry it manually.
- In foreground mode it prints the effective config and IM URL.
- In daemon mode it launches the hidden internal `_serve` entrypoint and waits for `/healthz`.

Examples:

```bash
csgclaw serve
csgclaw serve --no-codex-auto-install
csgclaw serve --no-auth-detect --no-browser
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

### `csgclaw upgrade`

Checks the latest release and optionally installs it.

Usage:

```bash
csgclaw upgrade [flags]
```

Flags:

- `--check`: check for updates without downloading or installing.
- `--no-restart`: install the new bundle without restarting the local service.

Behavior:

- `csgclaw upgrade --check` prints the current version, latest version, update availability, and matched asset name.
- `csgclaw upgrade` downloads the matching release archive, validates it, installs the full official bundle, and restarts the daemon if one is running.
- `csgclaw upgrade --no-restart` installs the new bundle but leaves the current daemon process unchanged.
- Automatic install supports only the official bundle layout, where the executable resolves to a dedicated `csgclaw/bin/csgclaw` or `csgclaw/bin/csgclaw.exe` tree. The updater never replaces a shared prefix such as `~/.local` or `/usr/local`.
- On Windows, release assets use `.zip`; other currently supported release assets use `.tar.gz`.
- Automatic restart supports only the default PID path `~/.csgclaw/server.pid`. If the daemon was started with custom PID or startup flags, use `--no-restart` and restart manually.

Common failure cases:

- Source builds or manually copied single binaries can use `--check`, but automatic install is rejected because there is no dedicated bundle root to replace.
- To migrate a binary copied into `~/.local/bin`, rerun `curl -fsSL https://csgclaw.opencsg.com/install.sh | bash`. The installer stores the managed bundle under `~/.local/lib/csgclaw` and replaces only `~/.local/bin/csgclaw` with a symlink; other files in `~/.local/bin` are left untouched.
- If the downloaded archive fails size or SHA256 validation, the CLI aborts before installation and asks you to retry later or report a broken release.
- If the release archive is malformed or missing `bin/csgclaw` / `bin/csgclaw.exe`, the CLI aborts before installation.
- `bin/boxlite` is optional. Bundles without it remain valid and will default to Docker when `[sandbox].provider` is unset.
- If restart cannot use the default PID path, rerun `csgclaw upgrade --no-restart`, then run `csgclaw stop` and `csgclaw serve --daemon` manually.

Examples:

```bash
csgclaw upgrade --check
csgclaw upgrade
csgclaw upgrade --no-restart
```

### `csgclaw model auth`

Manages local Codex and Claude Code authentication for model providers through the embedded CLIProxyAPI.

Usage:

```bash
csgclaw model auth login <provider> [flags]
csgclaw model auth logout <provider>
```

Providers:

- `codex`
- `claude-code`

Flags:

- `--no-browser`: print the OAuth URL instead of opening a browser.

Behavior:

- `codex` first reuses `~/.codex/auth.json` when available, then starts OAuth if needed.
- `claude-code` first probes macOS Keychain when available, then starts OAuth if needed.
- Auth is stored in the CSGClaw-managed CLIProxy auth directory, defaulting to `~/.csgclaw/auth`.
- `logout` disables the local CLIProxy auth record and blocks immediate re-import from the same Codex home auth or Claude Keychain entry.
- Model provider auth is scoped under `csgclaw model auth`, not the server's own API authentication.

Examples:

```bash
csgclaw model auth login codex
csgclaw model auth login claude-code --no-browser
csgclaw model auth logout codex
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
- `--runtime string`: agent runtime kind, for example `picoclaw_sandbox` or `codex`.

Behavior:

- Without `--replace`, the command creates a new agent.
- With `--replace`, `--id` is required.
- With `--replace`, the CLI sends a single create request with `replace: true` and a `field_mask` containing the flags explicitly passed this time.
- The API/service loads the existing agent, preserves unmasked fields, applies masked fields, then recreates the agent.
- Replacing an agent prompts for confirmation unless `--force` is set.
- `image` follows the same field-mask behavior: omitted keeps the old image, explicit `--image` overrides it.
- `runtime` follows the same field-mask behavior during replace: omitted keeps the old runtime kind, explicit `--runtime` overrides it.

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
csgclaw agent create --name alice --runtime codex --profile codex.gpt-5.4
csgclaw agent create -r --id agent-alice
csgclaw agent create --replace --id agent-alice --runtime codex --force
csgclaw agent create --replace --id agent-alice --name alice-v2 --profile openai.gpt-5.4-mini --force
csgclaw agent start agent-alice
csgclaw agent stop agent-alice
csgclaw agent logs agent-alice -n 50
csgclaw agent logs agent-alice --follow
csgclaw agent delete agent-alice
csgclaw agent delete --all --force
```

Notes:

- `--runtime codex` requires a local `codex` CLI that supports `app-server --listen stdio://`.
- Binary lookup uses `PATH` by default and can be overridden with `CSGCLAW_CODEX_PATH`.
  On Windows, use a native `codex.exe` rather than the PowerShell `codex.ps1` shim.
  Legacy `codex.cmd` and `codex.bat` values fall back to a sibling or CSGClaw-managed native executable.

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
csgclaw user create --name Alice --role worker
csgclaw user create --channel feishu --name Alice --role manager --avatar AL
csgclaw user delete alice
```

### Shared collaboration groups in `csgclaw`

The following command groups are shared with `csgclaw-cli` and use the same flags and semantics.

#### `participant`

Usage:

```bash
csgclaw participant <subcommand> [flags]
csgclaw pt <subcommand> [flags]
```

Subcommands:

- `list`
- `create`
- `bind`
- `delete`

`participant list` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--type string`: filter by `human`, `agent`, or `notification`.
- `--agent-id string`: filter by bound agent ID.

`participant create` flags:

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--id string`: participant ID.
- `--name string`: required participant display name.
- `--description string`: participant metadata description and agent description for `--bind create`.
- `--type string`: `human`, `agent`, or `notification`. Default `agent`.
- `--channel-user-ref string`: channel user identity, such as a local user ID or Feishu open_id.
- `--channel-user-kind string`: channel user identity kind, such as `local_user_id` or `open_id`.
- `--channel-app-ref string`: channel app/config reference, such as a Feishu app_id.
- `--bind string`: agent binding mode: `create`, `reuse`, or `none`. Default `none`.
- `--agent-id string`: agent ID for `--bind reuse`, or optional agent ID for `--bind create`.
- `--role string`: agent role for `--bind create`.
- `--runtime string`: agent runtime kind for `--bind create`.
- `--image string`: agent image for `--bind create`.
- `--from-template string`: hub template for `--bind create`.
- `--model-id string`: agent model ID for `--bind create`.
- `--env KEY=VALUE`: agent image environment variable for `--bind create`; repeatable.

`participant delete` usage and flags:

```bash
csgclaw participant delete <id> [flags]
```

- `--channel string`: `csgclaw` or `feishu`. Default `csgclaw`.
- `--delete-agent string`: agent cleanup mode. Supported value: `if_unreferenced`.

`participant bind` applies Feishu participant credentials.
Currently, only Feishu is supported.

`participant bind` flags:

- `--channel string`: `feishu` only. Default `feishu`.
- `--feishu-kind string`: `human` or `bot`.
- `--admin`: bind the Feishu admin human participant.
- `--open-id string`: required when `--feishu-kind human` and `--admin` is set.
- `--name string`: optional name for Feishu admin human participant.
- `--agent string`: worker/manager agent for Feishu bot binding.
- `--app-id string`: Feishu app id for bot binding.
- `--app-secret-file string`: read Feishu app secret from file.
- `--app-secret-env string`: read Feishu app secret from env var.
- `--app-secret-stdin`: read Feishu app secret from stdin.
- `--restart`: recreate the target agent after saving config. Manager targets return `restart_status=manager_recreated`
  when the recreate succeeds.

`participant bind` behavior:

- Exactly one `--feishu-kind` value is required.
- `--feishu-kind human` requires `--admin` and `--open-id`.
- `--feishu-kind bot` requires `--agent` and `--app-id`.
- For bot binding, exactly one of `--app-secret-file`, `--app-secret-env`, or `--app-secret-stdin` is required.
- `--restart` defaults to `false`; include it only when you want the target agent to be recreated.
- `pt bind` is equivalent to `participant bind`.

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
- `--creator-id string`: creator participant ID, such as `manager`.
- `--member-ids string`: comma-separated participant IDs, such as `manager,dev`.
- `--locale string`: room locale.

Design note for `csgclaw-cli`: room creation should expose CSGClaw participant IDs, not channel user IDs, agent IDs, Feishu open IDs, Feishu app IDs, or app credentials. In the Feishu channel, the channel adapter resolves participant IDs to the configured Feishu app credentials and channel identifiers internally. When Feishu group creation needs a real human owner ID, CSGClaw continues to use the configured `admin_open_id` internally; callers should still pass participant IDs at the CLI boundary.

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
- `--user-id string`: required. Participant ID to add, such as `dev`.
- `--inviter-id string`: inviter participant ID, such as `manager`.
- `--locale string`: room locale.

`member create` behavior:

- `--user-id` is required.
- `csgclaw-cli` room membership commands should use participant IDs consistently across channels. Feishu open IDs and app IDs are channel implementation details.

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
- `--sender-id string`: required sender participant ID.
- `--content string`: required.
- `--mention-id string`: optional mentioned participant ID.

`message list` behavior:

- `--room-id` is required.

Examples:

```bash
csgclaw participant list
csgclaw participant create --name alice --bind create --role worker --model-id gpt-5.4-mini
csgclaw participant bind --channel feishu --feishu-kind human --admin --open-id ou_xxx
csgclaw pt bind --channel feishu --feishu-kind bot --agent u-manager --app-id cli_xxx --app-secret-env FEISHU_APP_SECRET
csgclaw room create --title "release-room" --creator-id manager --member-ids manager,alice
csgclaw member create --room-id room-1 --user-id alice --inviter-id manager
csgclaw message list --room-id room-1
csgclaw message create --channel csgclaw --room-id room-1 --sender-id manager --content hello
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

- `participant`
- `pt`
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

- `participant list`
- `participant create`
- `participant delete`
- `pt list`
- `pt create`
- `pt delete`
- `participant bind`
- `pt bind`
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
csgclaw-cli participant list --channel feishu --type agent
csgclaw-cli pt create --name manager --channel feishu --type agent --bind create --role manager
csgclaw-cli participant bind --channel feishu --feishu-kind human --admin --open-id ou_xxx
csgclaw-cli pt bind --channel feishu --feishu-kind bot --agent u-manager --app-id cli_xxx --app-secret-stdin
csgclaw-cli room create --channel feishu --title "ops-room" --creator-id manager --member-ids manager,dev
csgclaw-cli member list --channel feishu --room-id oc_x
csgclaw-cli member create --channel feishu --room-id oc_x --user-id dev --inviter-id manager
csgclaw-cli message create --channel feishu --room-id oc_x --sender-id manager --mention-id dev --content hello
```

`csgclaw-cli` is the participant-facing CLI. Room, member, and message commands should not require callers to know or pass agent IDs, Feishu open IDs, Feishu app IDs, App ID/App Secret, or other channel credentials. Channel-specific adapters are responsible for exchanging participant IDs for the identifiers required by the target channel.
