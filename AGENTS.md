# CSGCLAW KNOWLEDGE BASE

## Communication

- Reply in the user's language unless asked otherwise.
- Keep responses concise and task-focused.

## Overview

CSGClaw is a Go-based local multi-agent platform. The `csgclaw` CLI bootstraps config, starts the local HTTP server and Web UI, and manages agents, rooms, and users. BoxLite runtime integration is provided through the `boxlite` sandbox path.

## Structure

```text
cmd/csgclaw/            CLI entrypoint
cli/                    command flows and user-facing output
internal/agent/         agent runtime, storage, BoxLite wiring
internal/config/        config defaults, load/save
internal/api/           HTTP handlers and router
internal/im/            IM service and PicoClaw bridge
internal/server/        HTTP server and UI wiring
web/app/                Web UI development source and Vite project
web/static-dist/        generated Web UI assets for Go embed; run make web-build
```

## Commands

See [docs/build.md](docs/build.md) or [docs/build.zh.md](docs/build.zh.md) for Makefile targets, embed dist staging, and optional Docker image builds.

```bash
make                    # default build (no docker images)
make build-all          # full build including embed docker images
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
- Before changing the Vite web app, read and follow `docs/web/development.md`; it owns frontend structure, source layout, components, styling, state/data flow, accessibility, assets, and verification. This includes changes under `web/app/src`, frontend tests, package/config files, CSS/Tailwind/tokens, public asset paths, routes, i18n text, and generated `web/static-dist` handling.
- Do not change BoxLite sandbox integration or packaging paths unless the task is about sandbox/runtime integration.
- When changing config fields or defaults, update loader, saver, onboard flow, tests, and docs together.
- Never hardcode or print real secrets; startup and logs must keep tokens redacted.

## Git And PRs

- Use Conventional Commits for commit messages and PR titles.
- Keep PR bodies compatible with `.github/workflows/pr-message.yml`: each line starts with `- `, no blank lines, no `Co-authored-by:` trailers.
- Keep PR bodies short and focused on summary, validation, impact, related issues, and notes.
- Check PR CI before requesting review or merge.

## Verification

- Use targeted tests first for local changes.
- Run `go test ./...` for shared or cross-package changes.
- Run `make` when touching build, CGO, linker flags, or packaging.
- If you skip verification, say so clearly.

## References

- `README.md`
- `docs/build.md`
- `docs/build.zh.md`
- `docs/web/development.md`
- `Makefile`
- `.github/workflows/release.yml`
