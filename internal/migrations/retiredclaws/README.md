# OpenClaw and PicoClaw retirement migration

This directory owns every rule specific to retiring OpenClaw and PicoClaw.
It contains the migration implementation and its tests.
Business packages call this module at their existing load boundaries and do not define retired runtime names, paths, manifest conversions or message envelopes.

## Entry points

| Entry point | Caller | Purpose |
| --- | --- | --- |
| `BootstrapDefaults`, `ObsoleteBootstrapKey` | `internal/config` | Clear retired defaults and request a config rewrite while preserving raw environment references. |
| `MigrateAgents` | `cli/serve` | Convert stored Agents before services load them; retain workspaces, import native MCP and memory, and request automatic startup as Codex. |
| `GeneratedBackupEntry` | `cli/serve`, `internal/onboard` | Allow backup errors only for old generated container plugin copies. |
| `LocalTemplates` | `internal/template` store factory | Back up and upgrade writable local registries. |
| `TemplateManifest` | Local and remote manifest readers | Adapt existing read-only and remote templates without writing remote data. |
| `InstructionFile`, `CopiedInstructions` | Template materialization | Convert the old singular instruction filename in copied workspaces. |
| `MessageMetadata` | `internal/im` session and message readers | Normalize historical activity envelopes without mutating the source snapshot. |

`MigrateAgents` must run under the application instance lock before serving requests.
It uses the storage owner's shared root-state lock so section updates cannot race the migration.
The caller lazily opens the configured sandbox provider and the Agent's existing sandbox home when a persisted sandbox handle requires inspection.
Every recorded sandbox is inspected through that provider and stopped when possible before its workspace is read or its Agent record is replaced.
Provider resolution, inspection, shutdown and confirmation failures are logged with the Agent ID and the original error, then migration continues automatically to Codex.
No command-line flag or external runtime recovery is required.
An Agent marked active without a sandbox identifier is also converted after logging that its previous sandbox cannot be checked.
A forced conversion creates a fresh snapshot even when an older completed retirement backup exists, preserving the current original records and sandbox identifiers for recovery.
Old sandboxes that could not be stopped remain outside CSGClaw management after conversion.
Cancellation, invalid stored data, backup failures and local read/write errors still preserve the original Agent records and report a migration failure.
Unconverted Agents remain unavailable and are not automatically started; a pending Manager is skipped by bootstrap.
Unrelated Agent saves preserve the original runtime kind and sandbox handle so local data failures can be retried on a later startup.
Configuration rewrites back up only the configuration file, so unreadable unrelated workspaces cannot block removal of retired bootstrap keys.
Identical configuration input reuses its complete file backup after a failed rewrite.
Normal Agent and local-template retirement steps reuse one completed snapshot per source root across retries.
A receipt inside the completed snapshot identifies it; partial copies are removed and never reused.
Existing unmarked backups are retained without alteration.
Readable configuration remains usable in memory if its migration backup or rewrite fails.
Local template migration errors also warn and fall back to the existing read-time adapters.
Generic backup, locking, typed-ID migration and JSON-preservation mechanics remain in their owning packages.

## Retention policy

Migrated Agents use Codex and persist `desired_state: running` so the existing configured-Agent startup pass starts them automatically.
The observed runtime state remains stopped until a real Codex process starts successfully.
Agents with incomplete model settings still require those settings to be completed, and startup failures are logged through the normal lifecycle.
Completed migrations do not override later user stops or other lifecycle changes.
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
