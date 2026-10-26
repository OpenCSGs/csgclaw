# CSGHub Sandbox Provider

The CSGHub provider connects CSGClaw to remote sandbox instances through the CSGHub API.
Provider implementation lives in `internal/sandbox/csghub/` and `internal/sandbox/csghub/csghubsdk/`.
It supports instance creation, inspection, commands, lifecycle operations and image listing through the shared sandbox interfaces.

Configure the provider through `[sandbox]` and `[sandbox.csghub]` as documented in [configuration](../config.md).
Provider credentials and runtime credentials must remain redacted in logs and API responses.

Codex and DeepSeek Harness run as local processes in the CSGClaw server environment.
Their creation does not provision a separate CSGHub sandbox.
For container deployment, see the [Codex server plan](csghub-local-codex-cluster-plan.zh.md) and [supervised upgrade plan](csghub-supervised-upgrade-plan.zh.md).

Verify provider changes with `go test ./internal/sandbox/csghub/...` and the affected API/lifecycle checks.
