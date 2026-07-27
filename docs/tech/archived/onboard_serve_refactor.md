# Onboard / Serve Refactor

## Goal

Keep `csgclaw onboard` as the single explicit bootstrap entrypoint, but allow `csgclaw serve` to auto-run the same onboarding flow when local bootstrap state is missing.

## Steps

1. Extract the current onboarding logic from `cli/onboard` into a shared application-level function, for example `internal/onboard.EnsureState(...)`.

2. Move these responsibilities into that function:
   - create or complete local config
   - ensure IM bootstrap state (`admin`, `manager`, bootstrap room, cleared invite draft data)
   - ensure manager bot and backing manager agent state

3. Make the shared onboarding function idempotent.
   It should be safe to call repeatedly without overwriting valid user config or recreating existing bootstrap state unnecessarily.

4. Add bootstrap state detection, for example `internal/onboard.DetectState(...)`.
   The detection should check config, IM bootstrap state, and manager bot/agent state instead of only checking whether the config file exists.

5. Update `csgclaw onboard` to become a thin wrapper around `internal/onboard.EnsureState(...)`.

6. Update `csgclaw serve` to detect missing bootstrap state before startup.
   If onboarding is incomplete, `serve` should log that it is auto-bootstrapping and then call the same shared onboarding function.

7. Reduce bootstrap-specific logic inside `serve`.
   After this refactor, `serve` should focus on starting the server and starting configured agents, while bootstrap state creation stays centralized in the shared onboarding flow.

8. Add or update tests for:
   - explicit `onboard`
   - `serve` auto-onboarding on a fresh state
   - repeated `serve` / `onboard` runs remaining idempotent
   - existing user config not being overwritten

9. Update CLI and config docs to describe:
   - `onboard` as the canonical bootstrap command
   - `serve` auto-running onboarding when needed
   - the exact scope of what onboarding creates
