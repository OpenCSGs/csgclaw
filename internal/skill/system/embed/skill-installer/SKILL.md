---
name: skill-installer
description: Find registry skills and install or reinstall skills in the current CSGClaw worker. Handles OpenCSG, ClawHub, skills.sh pages, GitHub skill repositories and npx skills add requests.
---

# Skill Installer (`csgclaw-cli skill`)

Use this skill for **all** registry skill search and install work.

Run commands inside the target worker. Install only the requested skill; use `--force` only for an explicitly requested reinstall.

## Registries

- **opencsg** (primary): `https://claw.opencsg.com` — searched first
- **clawhub** (official): `https://clawhub.ai` — used when opencsg has no hits

Slugs are **case-sensitive**. Copy exact `SLUG` and `REGISTRY` from search output.

| Subcommand | Purpose |
|------------|---------|
| `search <query>` | Search opencsg first; clawhub if empty (`--limit`) |
| `get <slug>` | Show one skill (`--registry`, `--version`) |
| `versions <slug>` | List published versions (`--registry`, `--limit`) |
| `install <slug>` | Download and install the registry skill files in this worker |

## Search and inspect

```bash
csgclaw-cli skill search gitlab --limit 10
csgclaw-cli skill get AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill versions AIWizards--gitlab-fullstack-pro --registry opencsg
```

Flags may appear before or after the query/slug (for example `skill search gitlab --limit 10`).

## Install from OpenCSG or ClawHub

Use the registry command directly. Read `Slug` (the installed directory name) and `SkillsDir` from the JSON result. The worker's install directory is `$DSH_HOME/skills` or `$CODEX_HOME/skills`. An explicit destination elsewhere is a files-only export: do not enable a same-named worker skill for it.

```bash
csgclaw-cli --output json skill install AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli --output json skill install AIWizards--gitlab-fullstack-pro --registry opencsg --version 1.0.0
csgclaw-cli --output json skill install my-skill --force
```

## Install from skills.sh or GitHub

For a skills.sh page, GitHub repository or npx skill installation request, use the bundled script **relative to this SKILL.md**, not the project's current directory. Keep the requested source; do not substitute a ClawHub search. If asked to pick any skill from skills.sh, you can use the example below. For ambiguous requests about a specific skill, ask for its name.

```sh
node "<directory containing this SKILL.md>/scripts/install-skills-sh.mjs" vercel-labs/agent-skills --skill web-design-guidelines
```

The helper accepts `https://skills.sh/<owner>/<repo>/<skill>` directly. It uses Node.js (22.20+), npx and Git, downloads in a temporary project and imports the complete skill into this worker. Use `Slug` from the final import result. Do not install globally, write to `.agents/skills` in the user's project, or copy files into `skill-view` yourself.

## Enable after either install path (Codex and DSH)

After **each successful worker install or reinstall**, call the **`csgclaw` MCP tool `skill_ensure_enabled`** once with `{"name":"<Slug from install result>"}`. This is an agent tool call, not a shell command. Do not call it if installation failed.

Only report “installed and enabled” after the tool returns `disk_synced: true`. If the tool is unavailable or fails, report “installed, not synced”; retry only the MCP call, without downloading again. Do not restart the worker or claim that the current session has reloaded the skill.

## Manager vs worker

- **Manager** may use this skill for `search`, `get`, and `versions` only.
- To install a skill for **another** agent, use the active conversation's task or message workflow. Ask that worker to follow the appropriate install flow above.
- Never install into another agent's filesystem from the manager sandbox.

## Operating rules

- Never guess or change slug casing.
- Prefer `skill get` / `skill versions` before install when version or moderation matters.
- Add `--output json` when structured output is needed (place global flags before the subcommand, e.g. `csgclaw-cli --output json skill search gitlab`).
- Run `csgclaw-cli skill -h` or `csgclaw-cli skill <subcommand> -h` for flags.
