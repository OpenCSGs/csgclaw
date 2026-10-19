# Runtime Evidence and Source Map

These are starting points, not proof of the affected installation's layout.
Confirm paths, routes, and flags against the downloaded revision and the running process.

## Version and endpoint

Use the endpoint supplied by the runtime or the actual application's configuration.
Do not assume a default port or use sandbox `localhost` as the host address.
The read-only `/healthz` and `/api/v1/version` routes establish reachability and the server's advertised version.
The version response currently contains `version`, not a guaranteed build commit.
For a locally accessible executable, `go version -m <executable>` may expose VCS build metadata when the Go toolchain is available; record a dirty-build marker if present.

## Evidence by installation

| Surface | Starting point | Interpretation |
| --- | --- | --- |
| CLI daemon | `~/.csgclaw/server.log` by default; `serve --log` can override it | Match the PID and daemon arguments to the affected instance. |
| CLI foreground server | The launching terminal or service manager's stdout/stderr | Foreground mode need not write `server.log`. |
| Desktop on macOS | `~/Library/Logs/CSGClaw/main.log`, `main.previous.log`, and `backend.log` | Correlate Electron events by `runId`; `backend.log` contains sidecar output. |
| Desktop on Windows | Electron user data, usually under `%APPDATA%\CSGClaw` | Locate actual log files rather than assuming an exact subdirectory. |
| Desktop on Linux | The actual Electron user-data/log directory | Resolve XDG configuration and packaging rather than copying macOS paths. |
| Agent runtime | Authorized `csgclaw-cli agent logs <agent-id> -n 200`, where supported | Use the real agent ID and current CLI help; keep credentials in the existing environment. |
| Turn diagnostics | User-supplied Desktop diagnostics export or permitted UI observation | Correlate source, room, agent, turn, and runtime request IDs. |
| Configuration | Normally `~/.csgclaw/config.toml`, plus the process's `--config` override | Inspect only relevant fields and redact before output. |

`~/.csgclaw` belongs to the current OS user.
A sandbox's copy does not represent the host's state.
Codex and DSH runtime layouts include `stderr.log`; discover the affected runtime's actual layout instead of guessing a path.

Room diagnostic APIs under `/api/v1/rooms/{id}/diagnostics` are a Desktop surface and reject agent callers.
Do not try to bypass that restriction through local diagnostic files, a different token, or modified headers.
When host evidence is unavailable, explain the limit and ask the user for the relevant redacted excerpt or export.

For Desktop startup, crash, or installer failures, read `docs/tech/desktop/electron-desktop-logs.zh.md` in the matching source.
Windows builds may include `scripts/collect-desktop-diagnostics.cmd` and its PowerShell companion.
Inspect their behavior before suggesting execution, and keep generated archives local unless the user requests sharing.

## Locate the owning implementation

| Symptom | Source entry points |
| --- | --- |
| CLI startup, bootstrap, service lifecycle | `cmd/csgclaw/`, `cli/serve/`, `internal/config/`, `internal/server/` |
| HTTP failure, permissions, routing | `internal/api/router.go`, the matching handler, `internal/auth/` |
| Agent offline, launch failure, stuck execution | `internal/agentengine/agents/`, `internal/runtime/codex/`, `internal/runtime/dsh/`, `internal/sandbox/` |
| Missing replies, streaming, cancellation, room dispatch | `internal/channel/csgclaw/`, `internal/im/`, `internal/agentengine/` |
| Slow replies and turn timelines | `internal/diagnostics/`, `internal/api/diagnostics.go`, runtime diagnostics tests |
| Provider protocol, model selection, upstream errors | `internal/modelprovider/`, `internal/llm/`, `internal/modelpreset/` |
| Feishu or another channel | `internal/channel/`, relevant `docs/tech/channel/` documentation |
| Skills, apps, MCP discovery or invocation | `internal/skill/`, `internal/apps/`, `internal/agentengine/agents/`, runtime MCP code |
| Web UI, preview, rendering | `web/app/`, `docs/tech/web/development.md` |
| Desktop window, sidecar, GPU or installer | `desktop/`, Desktop log documentation and packaging scripts |

Start with `rg -n -F '<exact-error-or-route>' <relevant-directories>`.
Use `git -C <source> rev-parse HEAD` and line-numbered excerpts for citations.
Link findings to `https://github.com/OpenCSGs/csgclaw/blob/<commit>/<path>#L<line>` when the inspected source is from the official repository.
For a fork, use that repository's verified URL.
Inspect a newer revision separately when checking whether an existing fix is available; a newer fix does not by itself establish the current incident's cause.
