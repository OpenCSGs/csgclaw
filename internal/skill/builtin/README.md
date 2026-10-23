# Bundled skills

`embed/` is the single source of the bundled skill files. These files are embedded
in the binary, not duplicated in manager templates. Preserve upstream licenses.
After changing files run `python3 internal/skill/builtin/scripts/manifest.py`.
`TestSyncCatalog` verifies every manifest hash.

On server startup, skills are released into the configured CSGClaw data directory's
`skills/` catalog (normally `~/.csgclaw/skills`). Manager bootstrap also ensures
these skills exist in its workspace, including before runtime setup completes.
Workers add them explicitly through Profile → Skills → Local, using existing APIs.
No script is executed and no dependencies are installed during release.

The existing `.skillhub-lock.json` tracks builtin origin, bundle version and hash.
Matching versions skip file reads and writes. Missing bundles are restored.
On upgrade, untouched builtin copies are replaced through staging and rename;
local name conflicts and modified bundles are preserved. Modified bundles emit a
warning when an update is available. Skill enablement state is not changed.
Removing a default manager skill is temporary: bootstrap restores missing bundles.
Ordinary agent copies are independent and follow the existing skill lifecycle.

The archive's scripts, references and assets are retained, including audio assets.
macOS metadata is excluded and ZIP filename encoding is normalized to UTF-8.
Runtime dependencies and third-party API credentials must be configured separately.
The word-to-md legacy `.doc` conversion uses Windows Word COM where available.

Optional bundled-skill sync failures are logged and retried on the next bootstrap;
they do not abort server or manager startup. Activation journals under the skills
root and backups under `.builtin-staging/` recover interrupted replacements. Files
are activated before origin commits; the next sync adopts a fully activated
bundle or restores a displaced old bundle before retrying. Committed transactions
are cleaned without rehashing user files.

Explicit install, replace and delete operations cancel the affected activation
journals under the installation lock before changing origins or files. Failed
operations restore the journals; successful operations discard obsolete backups.
A durable cancellation marker prevents interrupted user mutations from reviving
an old bundle. A failure in one bundled skill does not stop other skills syncing;
Sync returns the collected failures for logging and later retries.

Manager recreation restores preserved skills and their origins before filling
bundled defaults. User-owned names keep their files, version and enablement state.
Template saves and remote uploads exclude root activation journals, cancellation
markers, temporary journals and staging/backup trees; similarly named resources
inside a skill remain part of that skill.
