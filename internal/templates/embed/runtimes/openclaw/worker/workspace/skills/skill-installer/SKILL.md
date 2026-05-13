---
name: skill-installer
description: Install OpenClaw workspace skills into `~/.openclaw/workspace/skills` from a curated list, a GitHub repo path, or a git repo URL. Use when a user asks to list installable skills, install a curated skill, or install a skill from another repo (including private repos).
metadata:
  short-description: Install curated skills from openai/skills or other repos
---

# Skill Installer

Helps install skills. By default these are from https://github.com/openai/skills/tree/main/skills/.curated, but users can also provide other GitHub locations or a direct git repo URL. Experimental skills live in https://github.com/openai/skills/tree/main/skills/.experimental and can be installed the same way.

Use the helper scripts based on the task:
- List skills when the user asks what is available, or if the user uses this skill without specifying what to do. Default listing is `.curated`, but you can pass `--path skills/.experimental` when they ask about experimental skills.
- Install from the curated list when the user provides a skill name.
- Install from another repo when the user provides a GitHub repo/path or a git clone URL (including private repos).
- If a git-based install fails because `git` is missing, install `git` for the current environment, then rerun the same install command automatically.

Install skills with the helper scripts.

## Communication

When listing skills, output approximately as follows, depending on the context of the user's request. If they ask about experimental skills, list from `.experimental` instead of `.curated` and label the source accordingly:
"""
Skills from {repo}:
1. skill-1
2. skill-2 (already installed)
3. ...
Which ones would you like installed?
"""

After installing a skill, tell the user: "Restart Codex to pick up new skills."

## Scripts

All of these scripts use network, so when running in the sandbox, request escalation when running them.

- `scripts/list-skills.py` (prints skills list with installed annotations)
- `scripts/list-skills.py --format json`
- Example (experimental list): `scripts/list-skills.py --path skills/.experimental`
- `scripts/install-skill-from-github.py --repo <owner>/<repo> --path <path/to/skill> [<path/to/skill> ...]`
- `scripts/install-skill-from-github.py --repo <git-clone-url> --path <path/to/skill> [<path/to/skill> ...]`
- `scripts/install-skill-from-github.py --repo <git-clone-url> --name <skill-name>` when the repo root is the skill
- `scripts/install-skill-from-github.py --url https://github.com/<owner>/<repo>/tree/<ref>/<path>`
- Example (experimental skill): `scripts/install-skill-from-github.py --repo openai/skills --path skills/.experimental/<skill-name>`

## Behavior and Options

- Defaults to direct download for public GitHub repos.
- GitHub download mode falls back to git sparse checkout on auth/permission errors.
- Non-GitHub git repos use git sparse checkout directly.
- Aborts if the destination skill directory already exists.
- Installs into `~/.openclaw/workspace/skills/<skill-name>`.
- Multiple `--path` values install multiple skills in one run, each named from the path basename unless `--name` is supplied.
- Options: `--ref <ref>` (default `main`), `--dest <path>`, `--method auto|download|git`.

## Missing `git` Recovery

When installing from a git repo, or when GitHub download mode falls back to git, treat a missing `git` executable as a recoverable dependency issue rather than a final failure.

- Detect missing `git` from errors such as `git: command not found`, `No such file or directory: 'git'`, or equivalent launcher errors from the runtime.
- Confirm first with `git --version` if the error is ambiguous.
- If `git` is missing, install it automatically for the current environment, then rerun the original install command.
- Request escalation before running package-manager commands, because install commands usually need network and elevated privileges.
- If multiple package managers appear available, prefer the platform-native default.

Use one of these installation commands based on the detected environment:

- Debian/Ubuntu: `apt-get update && apt-get install -y git`
- Fedora/RHEL/CentOS (dnf): `dnf install -y git`
- Fedora/RHEL/CentOS (yum): `yum install -y git`
- Alpine: `apk add --no-cache git`
- Arch: `pacman -Sy --noconfirm git`
- openSUSE: `zypper install -y git`
- macOS with Homebrew: `brew install git`

If no supported package manager is available, explain that automatic `git` installation is not supported in the current environment and stop there instead of pretending the skill install succeeded.

## Notes

- Curated listing is fetched from `https://github.com/openai/skills/tree/main/skills/.curated` via the GitHub API. If it is unavailable, explain the error and exit.
- Private GitHub repos can be accessed via existing git credentials or optional `GITHUB_TOKEN`/`GH_TOKEN` for download.
- Private non-GitHub git repos depend on existing git credentials supported by the remote.
- Git fallback tries HTTPS first, then SSH.
- The skills at https://github.com/openai/skills/tree/main/skills/.system are preinstalled, so no need to help users install those. If they ask, just explain this. If they insist, you can download and overwrite.
- Installed annotations come from `~/.openclaw/workspace/skills`.
