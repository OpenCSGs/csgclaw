# Remove `boxlite-sdk` Provider

## Goal

Remove support for the `boxlite-sdk` sandbox provider while keeping the sandbox abstraction, the `boxlite-cli` provider, and other non-SDK providers intact.

This is not a sandbox-removal project. The `internal/sandbox` interfaces and the agent lifecycle built on top of them should remain unchanged.

## Scope Boundary

In scope:

- Remove the SDK-only provider registration and adapter code
- Remove SDK-specific build tags, build targets, setup flow, and tests
- Remove SDK-specific dependencies and vendored SDK wiring
- Update docs and config guidance so `boxlite-cli` becomes the only BoxLite provider

Out of scope:

- Removing the sandbox concept
- Refactoring `internal/agent` away from `sandbox.Provider`, `sandbox.Runtime`, or `sandbox.Instance`
- Changing the behavior of `boxlite-cli` or `csghub`

## Current Design Summary

The codebase already isolates `boxlite-sdk` behind a build tag and a dedicated adapter/provider path:

- `internal/config/sandbox_provider_boxlite_sdk.go` switches the default provider to `boxlite-sdk` when the `boxlite_sdk` build tag is enabled
- `internal/sandboxproviders/boxlite_sdk_provider.go` registers the SDK-backed provider
- `internal/sandbox/boxlitesdk/` contains the SDK adapter from BoxLite SDK types to generic sandbox interfaces
- `Makefile` contains SDK-only setup, build, test, and run targets
- `go.mod` and `third_party/boxlite-go` keep the vendored SDK dependency alive

This separation means the removal can stay local to provider selection, provider registration, build plumbing, dependencies, tests, and docs.

## Implementation Steps

### 1. Freeze the target behavior

Define the post-change behavior before editing code:

- `boxlite-cli` remains the default BoxLite sandbox provider in all builds
- `csghub` remains available
- `boxlite-sdk` is no longer a valid configured provider
- there is no longer a `boxlite_sdk` build shape

Expected config behavior after removal:

```toml
[sandbox]
provider = "boxlite-cli"
debian_registries_override = []
```

### 2. Remove config-level SDK selection

Update the config layer so there is no code path that can select `boxlite-sdk`.

Files to change:

- `internal/config/config.go`
- `internal/config/sandbox_provider_default.go`
- `internal/config/sandbox_provider_boxlite_sdk.go`

Concrete changes:

- Remove `BoxLiteSDKProvider` from `internal/config/config.go`
- Keep `BoxLiteCLIProvider` and existing sandbox config fields
- Delete `internal/config/sandbox_provider_boxlite_sdk.go`
- Keep `internal/config/sandbox_provider_default.go` as the single source of truth for `DefaultSandboxProvider = BoxLiteCLIProvider`

Why:

- `SandboxConfig.Resolved()` already uses `DefaultSandboxProvider`
- once the SDK-specific default file is removed, config resolution becomes uniform across all builds

### 3. Remove SDK provider registration

Delete the SDK provider registration entry point.

Files to change:

- `internal/sandboxproviders/boxlite_sdk_provider.go`

Concrete changes:

- Delete the file entirely

Why:

- provider registration is init-driven
- if this file remains, the provider can still be compiled in and advertised by `SupportedProviders()`

What should remain:

- `internal/sandboxproviders/boxlite_cli_provider.go`
- `internal/sandboxproviders/csghub_provider.go`
- `internal/sandboxproviders/registry.go`

No change is needed to the generic registry design.

### 4. Remove the SDK adapter package

Delete the BoxLite SDK adapter implementation.

Files to change:

- `internal/sandbox/boxlitesdk/adapter.go`
- `internal/sandbox/boxlitesdk/options.go`
- `internal/sandbox/boxlitesdk/adapter_test.go`

Concrete changes:

- Delete the entire `internal/sandbox/boxlitesdk/` package if nothing else depends on it

Why:

- after provider registration is removed, this package becomes dead code
- keeping it around encourages accidental reintroduction of SDK support

Important:

- do not change `internal/sandbox/sandbox.go`
- do not change `internal/agent/service.go`, `internal/agent/runtime.go`, or `internal/agent/box.go` just because the SDK provider is going away

Those packages depend on the sandbox abstraction, not on the SDK implementation.

### 5. Remove SDK build plumbing from `Makefile`

Delete the build paths that exist only for the SDK provider.

File to change:

- `Makefile`

Concrete changes:

- Remove `BOXLITE_SDK_TAG ?= boxlite_sdk`
- Remove `boxlite-setup`
- Remove `test-with-boxlite-sdk`
- Remove `build-with-boxlite-sdk`
- Remove `run-with-boxlite-sdk`
- Remove help text that references SDK-specific setup or build shapes
- Remove any `boxlite-setup` prerequisite links

Post-change command model:

- `make test` is the only Go test entry point documented for normal development
- `make build` is the only csgclaw build path
- `make run` is the only local run path

Why:

- two build shapes only exist because SDK support currently exists
- once the provider is removed, the extra shape is noise and future maintenance cost

### 6. Remove the vendored SDK dependency

Delete the direct SDK dependency and its vendored replacement.

Files to change:

- `go.mod`
- `go.sum`
- `third_party/boxlite-go/`

Concrete changes:

- Remove `require github.com/RussellLuo/boxlite/sdks/go v0.7.6`
- Remove `replace github.com/RussellLuo/boxlite/sdks/go => ./third_party/boxlite-go`
- Run `go mod tidy`
- Delete `third_party/boxlite-go/` if nothing else in the repo imports it

Why:

- after adapter removal there should be no code importing the SDK
- leaving the vendored dependency behind makes the repo look like SDK support still exists

Verification checkpoint:

- `rg -n "github.com/RussellLuo/boxlite/sdks/go|third_party/boxlite-go|boxlite_sdk" .`
  should show no live code references after cleanup, aside from historical docs if intentionally kept

### 7. Update tests

Remove tests that only exist for SDK behavior and adjust tests that mention supported providers.

Files to change:

- `internal/sandboxproviders/registry_boxlite_sdk_test.go`
- `internal/sandbox/boxlitesdk/adapter_test.go`
- any config or serve tests that still mention `boxlite-sdk`

Concrete changes:

- Delete SDK-only tests
- Keep tests that verify:
  - `boxlite-cli` is compiled in
  - `csghub` is compiled in
  - unsupported providers are rejected

Recommended test expectations after removal:

- `SupportedProviders()` should not contain `boxlite-sdk`
- configuring `provider = "boxlite-sdk"` should fail with an unsupported provider error

Suggested targeted test commands:

```bash
go test ./internal/sandboxproviders ./internal/config ./cli/serve
go test ./...
```

### 8. Update documentation

Remove documentation that implies SDK support still exists.

Files to change:

- `docs/tech/config.zh.md`
- `docs/tech/config.md` if it mentions SDK build variants
- `README.zh.md`
- `README.md` if it mentions vendored SDK or SDK build modes
- any archived or upgrade note that is still treated as active guidance

Concrete doc changes:

- replace “default build uses `boxlite-cli`, SDK build uses `boxlite-sdk`” with “BoxLite integration is provided through `boxlite-cli`”
- remove references to `make build-with-boxlite-sdk`, `make test-with-boxlite-sdk`, and `make run-with-boxlite-sdk`
- remove references to vendored Go SDK support when describing current architecture

Documentation nuance:

- archived design notes can be left alone if they are clearly historical
- active docs should not present SDK support as current behavior

### 9. Validate user-visible behavior

After code and docs are updated, validate the externally visible behavior.

Checkpoints:

- default config output still renders `[sandbox] provider = "boxlite-cli"`
- startup errors for unsupported providers are still clear
- `boxlite-cli` path resolution behavior is unchanged
- agent sandbox lifecycle still works through the generic interfaces

Suggested verification:

```bash
make fmt
go test ./internal/sandboxproviders ./internal/config ./cli/serve
go test ./...
```

If a local runtime smoke test is desired:

```bash
make build
./bin/csgclaw serve
```

## File-Level Checklist

Delete:

- `internal/config/sandbox_provider_boxlite_sdk.go`
- `internal/sandboxproviders/boxlite_sdk_provider.go`
- `internal/sandboxproviders/registry_boxlite_sdk_test.go`
- `internal/sandbox/boxlitesdk/adapter.go`
- `internal/sandbox/boxlitesdk/options.go`
- `internal/sandbox/boxlitesdk/adapter_test.go`
- `third_party/boxlite-go/` if fully unused

Edit:

- `internal/config/config.go`
- `internal/config/sandbox_provider_default.go`
- `internal/sandboxproviders/registry.go`
- `Makefile`
- `go.mod`
- `go.sum`
- `docs/tech/config.zh.md`
- `docs/tech/config.md` if needed
- `README.zh.md`
- `README.md` if needed

Keep unchanged unless another requirement appears:

- `internal/sandbox/sandbox.go`
- `internal/agent/service.go`
- `internal/agent/runtime.go`
- `internal/agent/box.go`
- `internal/sandbox/boxlitecli/*`
- `internal/sandbox/csghub/*`

## Risks

### Hidden references in tests and docs

The most likely cleanup misses are:

- tests built only under `boxlite_sdk`
- help text or release scripts still mentioning SDK-only commands
- docs that describe two build shapes after the code only supports one

### Dependency residue

If `third_party/boxlite-go` remains in the repo after imports are removed, contributors may assume SDK support still exists.

### Config compatibility

Existing user configs may still contain:

```toml
[sandbox]
provider = "boxlite-sdk"
```

Post-removal, startup should fail clearly with an unsupported provider error. If smoother migration is desired, add a migration note in release docs or a dedicated compatibility error message.

## Recommended Rollout Order

1. Remove provider selection and registration
2. Remove SDK adapter code
3. Remove build targets and dependency wiring
4. Update tests
5. Update docs
6. Run formatting and tests

This order reduces churn because the compile graph becomes simpler early, and dead code becomes obvious.

## Non-Goals Reminder

Do not:

- rewrite the sandbox abstraction
- rename generic sandbox concepts just because one provider is being removed
- mix this cleanup with broader agent runtime changes

The clean implementation is to remove one provider implementation and its build plumbing, not to redesign the execution model.
