# CSGClaw Go Runtime Notes

This repository includes the Go implementation for:

- `csgclaw onboard`
- `csgclaw serve`
- agent, bot, IM, channel, and API services

Runtime notes:

- Agent execution uses the `internal/sandbox` abstraction.
- The default sandbox provider is `boxlite-sdk`, backed by the vendored BoxLite Go SDK.
- `boxlite-cli` is also available for environments that provide a preinstalled `boxlite` binary.
- The generated config includes:

```toml
[sandbox]
provider = "boxlite-sdk"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

To opt in to the CLI provider:

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

- `boxlite_cli_path` is used only by `provider = "boxlite-cli"`. The default value resolves `boxlite` from `PATH`; use an absolute path when needed.
- CSGClaw passes `--home` to the BoxLite CLI for each agent using the agent directory plus `home_dir_name`, for example `~/.csgclaw/agents/<agent-id>/boxlite`.
- That explicit sandbox home is independent of `BOXLITE_HOME` for CSGClaw-managed boxes. `BOXLITE_HOME` still applies to manual `boxlite` invocations that omit `--home`.
- The `boxlite-cli` provider does not require the vendored Go SDK at runtime.
- The BoxLite Go SDK requires Go 1.24+ with CGO enabled.
- The vendored SDK tracks `github.com/RussellLuo/boxlite/sdks/go v0.7.6`.
- Source builds and the current default `boxlite-sdk` provider fetch `libboxlite.a` on demand via `make boxlite-setup` and the standard `make build` / `make test` targets.
- If you prefer to run the setup manually, use `(cd third_party/boxlite-go && BOXLITE_SDK_VERSION=v0.7.6 go run ./cmd/setup)`.
- The prebuilt BoxLite native library currently supports macOS arm64 and Linux amd64.
