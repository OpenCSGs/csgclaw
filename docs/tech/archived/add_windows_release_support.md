# Add Windows Release Support

## Goal

Add official release support for Windows and other non-BoxLite bundle targets without breaking the existing bundled-BoxLite experience on current supported platforms.

The target release behavior is:

- Platforms where the release bundle includes `bin/boxlite` default to `provider = "boxlite"`.
- Platforms where the release bundle does not include `bin/boxlite` default to `provider = "docker"`.
- Users may always override the provider in `config.toml`.
- Existing bundled-BoxLite platforms continue to work without requiring users to install `boxlite` separately.
- New platforms, especially Windows, can install and run `csgclaw` without a bundled `boxlite` binary.

This is a release and packaging project first. It does not require changing the BoxLite runtime contract itself.

## Scope Boundary

In scope:

- Release bundle layout changes
- Packaging script changes
- GitHub release workflow changes
- Dynamic sandbox provider defaulting based on bundle contents
- Upgrade/install compatibility changes for bundles without `bin/boxlite`
- Documentation and tests for the new bundle shapes

Out of scope:

- Adding Windows support to the BoxLite binary itself
- Changing Docker runtime semantics
- Refactoring the generic sandbox abstraction
- Changing agent runtime behavior unrelated to bundle detection or provider defaulting

## Desired Behavior

### Bundle behavior

There should be one official `csgclaw` bundle layout across platforms:

```text
csgclaw/
  bin/
    csgclaw[.exe]
    boxlite        # optional, only present on bundled-BoxLite targets
```

This keeps install, upgrade, and runtime resolution centered on one bundle convention.

For `csgclaw-cli`, the current separate archive behavior can remain unchanged unless Windows support later requires bundle semantics there too.

### Sandbox provider behavior

The provider resolution policy should be:

1. If `[sandbox].provider` is explicitly set in `config.toml`, use it as-is.
2. If `[sandbox].provider` is missing or empty, resolve the default dynamically:
   - if bundled `boxlite` exists, default to `boxlite`
   - otherwise default to `docker`
3. Do not overwrite an explicit user choice during normal startup.

This means a bundle with `bin/boxlite` defaults to BoxLite, but users can still switch to Docker manually.

### Upgrade behavior

The official upgrade flow should accept both official bundle shapes:

- bundle with `bin/csgclaw` and `bin/boxlite`
- bundle with `bin/csgclaw` only

If the current config still points to `boxlite` but the upgraded bundle no longer includes bundled `boxlite`, startup should fail with a clear error unless `boxlite` can be found via `PATH` or the user switches to `docker`.

## Current Design Constraints

The current implementation has several assumptions that prevent this behavior today.

### 1. Packaging assumes bundled BoxLite for non-Windows `csgclaw`

The current packaging script only builds an official bundle directory when:

- `APP = csgclaw`
- `GOOS != windows`
- `PACKAGE_MODE = bundled-boxlite-cli`

In that case it always fetches and stages `boxlite`.

Relevant file:

- [scripts/package-release.sh](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/scripts/package-release.sh:19)

### 2. Windows packaging is not an official bundle layout

The current Windows branch creates a zip containing only the binary, not a `csgclaw/bin/...` bundle tree.

Relevant file:

- [scripts/package-release.sh](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/scripts/package-release.sh:45)

### 3. Release workflow only covers the old BoxLite-capable matrix

The current workflow publishes only:

- `linux/amd64`
- `linux/arm64`
- `darwin/arm64`

and passes one global package mode into every matrix entry.

Relevant file:

- [.github/workflows/release.yml](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/.github/workflows/release.yml:1)

### 4. Upgrade validation requires `bin/boxlite`

Upgrade currently rejects any official bundle that does not contain `bin/boxlite`.

Relevant file:

- [internal/upgrade/download.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/upgrade/download.go:203)

### 5. Upgrade asset selection only supports `.tar.gz`

Upgrade currently matches only:

```text
csgclaw_<version>_<goos>_<goarch>.tar.gz
```

This excludes Windows zip assets.

Relevant file:

- [internal/upgrade/upgrade.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/upgrade/upgrade.go:130)

### 6. Official install root detection assumes `csgclaw`, not `csgclaw.exe`

Bundle root detection is currently Unix-only in practice because it looks only for `bin/csgclaw`.

Relevant file:

- [internal/upgrade/install.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/upgrade/install.go:83)

### 7. Default sandbox provider is a compile-time constant

The current default provider is always `boxlite`, regardless of whether the bundle includes `boxlite`.

Relevant file:

- [internal/config/sandbox_provider_default.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/config/sandbox_provider_default.go:1)

This is the key reason why packaging changes alone are not enough.

## Proposed Design

## 1. Keep one official bundle layout, make `boxlite` optional

For `csgclaw`, every official release artifact should unpack into:

```text
csgclaw/
  bin/
    csgclaw[.exe]
    boxlite        # optional
```

This should be true for:

- Linux
- macOS
- Windows

The only difference across targets is whether `bin/boxlite` is present.

Recommended target groups:

- bundled-BoxLite targets:
  - `linux/amd64`
  - `linux/arm64`
  - `darwin/arm64`
- Docker-default targets:
  - `darwin/amd64`
  - `windows/amd64`
  - `windows/arm64`

If additional BoxLite binaries become available later, a target can move from Docker-default to bundled-BoxLite without redesigning the release flow.

## 2. Change provider defaulting from compile-time to runtime/bundle-aware

The default provider should no longer be a single unconditional constant in practice.

Instead, config initialization or config resolution should behave like this:

- explicit `provider` wins
- missing/empty `provider` triggers dynamic default resolution
- dynamic default resolution checks whether bundled `boxlite` exists next to the installed `csgclaw`

Recommended semantics:

- bundled `boxlite` present: default `boxlite`
- bundled `boxlite` absent: default `docker`

Important:

- this logic should only apply when the provider is unset
- startup must not silently rewrite an explicit user choice
- migration logic for legacy aliases such as `boxlite-cli` should remain intact

## 3. Keep BoxLite path resolution as bundle-first with `PATH` fallback

The current BoxLite path resolution already supports the desired runtime model:

- explicit configured path wins
- otherwise bundled sibling `boxlite` is preferred
- otherwise `PATH` fallback is used

Relevant file:

- [internal/sandbox/boxlitecli/resolve.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/sandbox/boxlitecli/resolve.go:13)

This means no design change is required there. The missing piece is provider defaulting, not BoxLite path resolution.

## 4. Make release workflow matrix-driven by target capabilities

The release workflow should move from one global package mode to per-target capability flags.

Recommended matrix fields:

- `goos`
- `goarch`
- `runner`
- `archive_format`
- `include_boxlite`

Example conceptual matrix:

```yaml
include:
  - runner: ubuntu-latest
    goos: linux
    goarch: amd64
    include_boxlite: true
  - runner: ubuntu-24.04-arm
    goos: linux
    goarch: arm64
    include_boxlite: true
  - runner: macos-14
    goos: darwin
    goarch: arm64
    include_boxlite: true
  - runner: macos-13
    goos: darwin
    goarch: amd64
    include_boxlite: false
  - runner: windows-latest
    goos: windows
    goarch: amd64
    include_boxlite: false
```

Whether `windows/arm64` is built directly or deferred depends on GitHub Actions runner availability and cross-compilation constraints.

## 5. Make upgrade/install accept official bundles without `bin/boxlite`

Upgrade should continue to enforce the official bundle shape, but `bin/boxlite` should become optional.

Validation should distinguish:

- required:
  - `bin/csgclaw` or `bin/csgclaw.exe` depending on platform
- optional:
  - `bin/boxlite`

The install root rule should remain:

```text
<install-root>/bin/csgclaw[.exe]
```

That preserves atomic bundle replacement while allowing platform-specific runtime contents.

## Affected Areas

### Packaging

Files likely involved:

- `scripts/package-release.sh`
- `scripts/fetch-boxlite-cli.sh`
- `Makefile`

Expected changes:

- always build `csgclaw` as an official bundle tree
- include `boxlite` only when the target requests it
- keep Windows output as a bundle zip, not a bare executable zip
- update local `make release` and `make package` guidance accordingly

### Release workflow

Files likely involved:

- `.github/workflows/release.yml`

Expected changes:

- replace global `package_mode` behavior with matrix-driven `include_boxlite`
- add new release targets
- ensure artifact naming remains stable enough for upgrade matching
- revisit `CGO_ENABLED=1`

The repository currently does not show an obvious cgo requirement for this flow. If cross-platform release builds are needed, `CGO_ENABLED=1` may become an unnecessary constraint.

### Config and startup defaulting

Files likely involved:

- `internal/config/config.go`
- `internal/config/sandbox_provider_default.go`
- `cli/serve/serve.go`
- config loader/saver tests

Expected changes:

- stop treating the default provider as a single unconditional constant in effective behavior
- resolve a dynamic default only when provider is unset
- ensure saved config uses canonical provider names

### Upgrade/install

Files likely involved:

- `internal/upgrade/download.go`
- `internal/upgrade/install.go`
- `internal/upgrade/upgrade.go`
- `cli/upgrade/upgrade.go`
- upgrade tests

Expected changes:

- accept `.zip` for Windows assets
- support extracting zip archives
- relax bundle validation so `bin/boxlite` is optional
- support `.exe` when detecting the installed executable layout
- keep error messages explicit when a runtime provider is configured but unavailable

### Documentation

Files likely involved:

- `README.md`
- `docs/tech/config.md`
- `docs/tech/config.zh.md`
- `docs/tech/cli.md`
- `docs/tech/cli.zh.md`

Expected changes:

- explain that official bundles may or may not include `boxlite`
- explain the dynamic default provider policy
- explain that Docker becomes the default when bundled `boxlite` is absent
- update upgrade/install expectations for Windows artifacts

## Incremental Implementation Plan

The changes should be implemented in small steps so each phase has a stable checkpoint.

### Step 1. Define the new bundle contract in code and docs

Objective:

- formalize that official bundles always use the same root layout
- make `bin/boxlite` optional

Suggested work:

- update this design into implementation comments if needed
- adjust upgrade bundle validation helpers first so the code can represent the new shape
- add tests for:
  - bundle with `bin/csgclaw` only
  - bundle with `bin/csgclaw` and `bin/boxlite`
  - invalid bundle missing `bin/csgclaw`

Why first:

- many later changes depend on the bundle contract becoming explicit and testable

### Step 2. Introduce dynamic sandbox provider defaulting

Objective:

- ensure bundles without `boxlite` default to Docker

Suggested work:

- add a helper that detects bundled `boxlite`
- use that helper when provider is unset
- preserve explicit user config exactly
- keep legacy provider alias canonicalization

Suggested tests:

- provider unset + bundled `boxlite` present -> effective provider `boxlite`
- provider unset + bundled `boxlite` absent -> effective provider `docker`
- provider explicitly set to `docker` + bundled `boxlite` present -> effective provider `docker`
- provider explicitly set to `boxlite` + bundled `boxlite` absent -> config remains `boxlite`; runtime failure should be surfaced later, not silently rewritten

### Step 3. Update packaging script for optional BoxLite

Objective:

- make `csgclaw` packaging emit official bundles for every target

Suggested work:

- replace the old `PACKAGE_MODE` branching with a clearer per-build capability input such as `INCLUDE_BOXLITE=1|0`
- always stage `csgclaw/bin/csgclaw[.exe]`
- fetch `boxlite` only when requested
- zip the whole `csgclaw/` tree on Windows

Suggested tests/checks:

- inspect archive structure for bundled-BoxLite targets
- inspect archive structure for Docker-default targets
- verify Windows zip contains `csgclaw/bin/csgclaw.exe`

### Step 4. Expand the release workflow matrix

Objective:

- publish the new platform set with the right bundle contents

Suggested work:

- move workflow configuration to per-target capability fields
- add new target entries
- keep artifact names stable
- confirm whether `CGO_ENABLED=0` can be used for release packaging

Verification:

- each matrix target produces the expected archive name
- each archive matches the intended bundle content policy

### Step 5. Upgrade support for Windows and optional BoxLite bundles

Objective:

- make official upgrade compatible with the new artifact set

Suggested work:

- extend asset selection to support `.zip` for Windows
- add zip extraction
- allow `bin/boxlite` to be optional
- support `.exe` in install-root detection

Suggested tests:

- select Windows asset correctly
- prepare and validate zip bundle correctly
- install from bundle root containing `csgclaw.exe`
- reject malformed zip or tar bundles

### Step 6. Improve user-facing error handling

Objective:

- avoid confusing failures when a configured provider is unavailable

Suggested work:

- improve startup error messages when:
  - provider is `boxlite`
  - bundled `boxlite` is absent
  - `PATH` fallback also fails
- mention the actionable fix:
  - switch to `docker`
  - or install `boxlite` separately if that platform later supports it

This is especially important for users who upgrade across bundle families.

### Step 7. Update documentation and release notes

Objective:

- make the new behavior discoverable and unsurprising

Suggested work:

- document the official bundle layouts
- document default provider selection
- document that users can always override `[sandbox].provider`
- document Windows expectations and Docker prerequisite

## Recommended Verification Strategy

Use targeted verification first, then broader coverage.

Suggested targeted tests:

```bash
go test ./internal/upgrade ./internal/config ./cli/serve
```

Suggested broader verification:

```bash
go test ./...
```

Suggested packaging checks:

- produce archives for at least one bundled-BoxLite target and one Docker-default target
- inspect the resulting archive trees manually or via test helpers

If the release workflow changes substantially, a dry-run packaging check outside GitHub Actions is recommended before editing the release tag flow.

## Risks and Edge Cases

### 1. Upgrading from a bundled-BoxLite install to a Docker-default install

A user may have:

- old config with `provider = "boxlite"`
- new bundle without `bin/boxlite`

In that case:

- dynamic defaulting does not apply because the provider is already explicit
- startup must fail clearly if `boxlite` cannot be resolved

This is correct behavior. Silent rewriting would hide user intent.

### 2. Windows support is not complete until upgrade and install semantics are updated

If Windows artifacts are published before upgrade/install logic is updated:

- the binary may run
- but `csgclaw upgrade` will remain incomplete or broken on Windows

That may be acceptable temporarily, but it should be treated as an explicit rollout phase, not an accidental gap.

### 3. Asset naming must remain deterministic

The upgrade code matches asset names directly. If release artifact naming drifts during the migration, upgrade will fail even if the artifact contents are correct.

### 4. Workflow runner availability may constrain Windows arm64 or macOS amd64

The target matrix in this document is the desired product behavior. The exact CI runner implementation may need to roll out in phases depending on available runners and cross-compilation support.

## Recommended Rollout Order

Recommended order:

1. Make bundle validation and provider-default rules support the new model.
2. Update packaging locally to emit both bundle families.
3. Update upgrade/install for optional `boxlite` and Windows artifacts.
4. Expand the GitHub release workflow matrix.
5. Update docs and release notes.

This order keeps the internal contract stable before the public release matrix expands.

## Summary

The core design decision is simple:

- official `csgclaw` releases always use one bundle layout
- `boxlite` becomes an optional bundled runtime dependency
- sandbox defaulting becomes bundle-aware only when the user has not chosen a provider

With that design:

- old platforms keep the zero-config BoxLite experience
- new platforms can default to Docker cleanly
- users retain full control through `config.toml`
- release, install, and upgrade logic can stay coherent instead of splitting into unrelated platform-specific behaviors
