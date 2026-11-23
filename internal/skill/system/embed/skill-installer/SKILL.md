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
| `install <slug>` | Install and enable the registry skill in this worker |

## Search and inspect

```bash
csgclaw-cli skill search gitlab --limit 10
csgclaw-cli skill get AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill versions AIWizards--gitlab-fullstack-pro --registry opencsg
```

Flags may appear before or after the query/slug (for example `skill search gitlab --limit 10`).

## Install from OpenCSG or ClawHub

Use the registry command directly. The worker's install directory is `$DSH_HOME/skills` or `$CODEX_HOME/skills`. An explicit `--skills-dir` or `CSGCLAW_SKILLS_DIR` pointing elsewhere is a files-only install.

```bash
csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg --version 1.0.0
csgclaw-cli skill install my-skill --force
```

## Install from skills.sh or GitHub

For a skills.sh page, GitHub repository or `npx skills add` request, run the bundled script **relative to this SKILL.md**, not the project's current directory. If the repository contains several skills, select the requested one; ask for its name if ambiguous.

```sh
node "<directory containing this SKILL.md>/scripts/install-skills-sh.mjs" vercel-labs/agent-skills --skill web-design-guidelines
```

The helper accepts `https://skills.sh/<owner>/<repo>/<skill>` directly. It uses Node.js (22.20+), npx and Git, downloads in a temporary project, imports the complete skill into this worker, and calls `csgclaw-cli skill enable` on both Codex and DSH. Do not install globally, write to `.agents/skills` in the user's project, or copy files into `skill-view` yourself.

Both fresh installs and reinstalls enable the skill on disk. If enable fails **after** installation, retry `csgclaw-cli skill enable <installed-name>` without downloading again. Do not restart the worker or promise that an existing session has reloaded the skill.

## Manager vs worker

- **Manager** may use this skill for `search`, `get`, and `versions` only.
- To install a skill for **another** agent, use the active conversation's task or message workflow. Ask that worker to follow the appropriate install flow above.
- Never install into another agent's filesystem from the manager sandbox.

## Operating rules

- Never guess or change slug casing.
- Prefer `skill get` / `skill versions` before install when version or moderation matters.
- Add `--output json` when structured output is needed (place global flags before the subcommand, e.g. `csgclaw-cli --output json skill search gitlab`).
- Run `csgclaw-cli skill -h` or `csgclaw-cli skill <subcommand> -h` for flags.
