# CSGCLAW KNOWLEDGE BASE

## Reply Format (Required)

- Every first reply must begin with: "I have followed the instructions in AGENTS.md."
- Immediately follow with a concise, natural-English refinement of the user's query to aid English learning.

## Overview

CSGClaw is a Go-based local multi-agent platform. The `csgclaw` CLI bootstraps config, starts the local HTTP server and Web UI, and manages agents, rooms, and users. BoxLite runtime integration lives in `third_party/boxlite-go`.

## Structure

```text
cmd/csgclaw/            CLI entrypoint
cli/                    command flows and user-facing output
internal/agent/         agent runtime, storage, BoxLite wiring
internal/config/        config defaults, load/save
internal/api/           HTTP handlers and router
internal/im/            IM service and PicoClaw bridge
internal/server/        HTTP server and UI wiring
web/static/             shipped frontend assets
third_party/boxlite-go/ vendored BoxLite SDK
```

## Commands

```bash
make                    # default build
make fmt
make test
make run
make onboard
go test ./...
go test ./cli ./internal/config
make package
make release
```

## Rules

- Keep `cmd/` thin; put command behavior in `cli/` and domain logic in `internal/`.
- Prefer existing patterns and the standard library before adding dependencies.
- Format with `make fmt`.
- Add or update tests when changing CLI, config, API, or runtime behavior.
- Do not edit `third_party/boxlite-go` unless the task is about SDK, CGO, or linker/runtime integration.
- When changing config fields or defaults, update loader, saver, onboard flow, tests, and docs together.
- Never hardcode or print real secrets; startup and logs must keep tokens redacted.

## Verification

- Use targeted tests first for local changes.
- Run `go test ./...` for shared or cross-package changes.
- Run `make` when touching build, CGO, linker flags, or packaging.
- If you skip verification, say so clearly.

## References

- `README.md`
- `docs/README.go.md`
- `Makefile`
- `.github/workflows/release.yml`
