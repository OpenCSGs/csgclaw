---
name: skill-installer
description: Search, inspect, and install ClawHub-compatible skills with csgclaw-cli skill. Use when the user asks to find skills on opencsg or clawhub.ai, list versions, or install into this sandbox workspace/skills directory.
---

# Skill Installer (`csgclaw-cli skill`)

Use this skill for **all** registry skill search and install work. Do **not** use PicoClaw `find_skills` or `install_skill` (disabled).

Run commands via `exec` inside the current agent container.

## Registries

- **opencsg** (primary): `https://claw.opencsg.com` — searched first
- **clawhub** (official): `https://clawhub.ai` — used when opencsg has no hits

Slugs are **case-sensitive**. Copy exact `SLUG` and `REGISTRY` from search output.

| Subcommand | Purpose |
|------------|---------|
| `search <query>` | Search opencsg first; clawhub if empty (`--limit`) |
| `get <slug>` | Show one skill (`--registry`, `--version`) |
| `versions <slug>` | List published versions (`--registry`, `--limit`) |
| `install <slug>` | Install into **this** sandbox `workspace/skills/<slug>/` |

## Search and inspect

```bash
csgclaw-cli skill search gitlab --limit 10
csgclaw-cli skill get AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill versions AIWizards--gitlab-fullstack-pro --registry opencsg
```

Flags may appear before or after the query/slug (for example `skill search gitlab --limit 10`).

## Install (this sandbox)

Default install path: `~/.picoclaw/workspace/skills/` (PicoClaw) or `~/.openclaw/workspace/skills/` (OpenClaw).

```bash
csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg
csgclaw-cli skill install AIWizards--gitlab-fullstack-pro --registry opencsg --version 1.0.0
csgclaw-cli skill install my-skill --force
```

Use `--skills-dir` only when the workspace layout is non-standard.

## Manager vs worker

- **Manager** may use this skill for `search`, `get`, and `versions` only.
- To install a skill for **another** agent, dispatch that worker (see `basics` / `manager-worker-dispatch`) and ask it to run `csgclaw-cli skill install` in **its** container using this same skill.
- Never install into another agent's filesystem from the manager sandbox.

## Operating rules

- Never guess or change slug casing.
- Prefer `skill get` / `skill versions` before install when version or moderation matters.
- Add `-o json` when structured output is needed.
- Run `csgclaw-cli skill -h` or `csgclaw-cli skill <subcommand> -h` for flags.
