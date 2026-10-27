# OpenClaw and PicoClaw retirement migration

This directory owns every rule specific to retiring OpenClaw and PicoClaw.
It contains the migration implementation and its tests.
Business packages call this module at their existing load boundaries and do not define retired runtime names, paths, manifest conversions or message envelopes.

## Entry points

| Entry point | Caller | Purpose |
| --- | --- | --- |
| `BootstrapDefaults`, `ObsoleteBootstrapKey` | `internal/config` | Clear retired defaults and request a config rewrite while preserving raw environment references. |
| `MigrateAgents` | `cli/serve` | Convert stored Agents before services load them; retain workspaces, import native MCP and memory, and stop migrated workers. |
| `GeneratedBackupEntry` | `cli/serve`, `internal/onboard` | Allow backup errors only for old generated container plugin copies. |
| `LocalTemplates` | `internal/template` store factory | Back up and upgrade writable local registries. |
| `TemplateManifest` | Local and remote manifest readers | Adapt existing read-only and remote templates without writing remote data. |
| `InstructionFile`, `CopiedInstructions` | Template materialization | Convert the old singular instruction filename in copied workspaces. |
| `MessageMetadata` | `internal/im` session and message readers | Normalize historical activity envelopes without mutating the source snapshot. |

`MigrateAgents` must run under the application instance lock before serving requests.
It uses the storage owner's shared root-state lock so section updates cannot race the migration.
The caller lazily opens the configured sandbox provider and the Agent's existing sandbox home when a persisted sandbox handle requires inspection.
Every recorded sandbox is inspected through that provider, stopped if necessary, and confirmed stopped or absent before its workspace is read or its Agent record is replaced.
Saved lifecycle state alone is not accepted as confirmation.
If provider access, shutdown, backup or confirmation fails, startup logs a warning and continues without replacing retired Agent records or discarding their sandbox handles.
Unmigrated Agents remain visible as unavailable and are not automatically started; a pending Manager is skipped by bootstrap.
Unrelated Agent saves preserve the original runtime kind and sandbox handle so a later startup can retry.
After restoring provider access, restart CSGClaw to retry the migration.
An Agent marked active without a sandbox identifier is left unchanged with a warning; it does not prevent the application from starting.
Configuration rewrites back up only the configuration file, so unreadable unrelated workspaces cannot block removal of retired bootstrap keys.
Identical configuration input reuses its complete file backup after a failed rewrite.
Agent and local-template retirement steps reuse one completed snapshot per source root across retries.
A receipt inside the completed snapshot identifies it; partial copies are removed and never reused.
Existing unmarked backups are retained without alteration.
Readable configuration remains usable in memory if its migration backup or rewrite fails.
Local template migration errors also warn and fall back to the existing read-time adapters.
Generic backup, locking, typed-ID migration and JSON-preservation mechanics remain in their owning packages.

## Retention policy

Migrated workers use Codex and remain explicitly stopped until the user starts them.
Agent identity, model configuration, MCP selections, custom Skills, messages and user files are retained.
Existing MCP snapshots and current Codex memory take precedence over retired native files.
Remote templates and user-authored script contents are not rewritten remotely.
Completed on-disk migrations are idempotent.

## Removing the migration

Keep the module until the supported upgrade window no longer includes these runtime records and old remote templates have been republished.
Message and read-only template adapters remain read-time compatibility hooks.
Before removing them, rewrite all retained session lines, thread histories and spillover blobs that still use old envelopes, including inactive conversations.
Agent migration completion alone does not prove that historical messages are ready for removing the adapter.
Remove this directory and remove its business integration calls in the same change.
Locate every integration with `rg 'migrations/retiredclaws|retiredclaws\.' cli internal --glob '*.go'`.
Remove the retired-bootstrap branch and its `RewriteRequired` assignment in the config loader.
Use the required instruction filename directly when removing the template instruction helpers.
Pass no retired-cache predicate to generic backup and typed-ID migration helpers.
Remove the retired-config hookup test in `cli/serve/serve_test.go` with the integration it exercises.
Keep generic fixes for state-section preservation, standalone config backups, stopped host status and instructions-only templates.
Run `go test ./...`, build the application, and verify startup and current Codex/DSH workflows.
