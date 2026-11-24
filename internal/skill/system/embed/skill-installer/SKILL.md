---
name: skill-installer
description: Find, add, install, reinstall or check an existing skill for the current CSGClaw worker (安装、添加、搜索、重装或查询技能). Use for the global library, the signed-in site's CSGHub market, skills.sh, GitHub skill URLs and npx skills requests. To author a new skill, use skill-creator instead.
---

# Install an existing skill

Use this workflow for catalog or registry skill search and installation. Do not switch sources silently. A Manager delegates installs to the target worker; it must not write another agent's files.

## 1. Find or inspect

- For a general request, call the `csgclaw` MCP tool `skill_search` with `{"query":"<skill name or concise capability keywords>"}`. This checks the **CSGClaw global library** first, including built-ins and discovered skills. A local match needs no network. Without a match it searches the signed-in CSGHub site and skills.sh concurrently.
- If the user names a specific source, pass `source: "global"`, `"site"` or `"skills_sh"`; only a local copy from that source may satisfy the search. For an exact skills.sh page, pass the full URL as `query` with `source:"skills_sh"`. A GitHub repository and known skill name can also be passed directly to the helper below. Never replace a specified source or version with a different one.
- Preserve each candidate's `source`, `key`, `site`, `ref` and `url`. Show distinct choices for ambiguous matches; install only the selection, or pick one if explicitly asked to choose. If no match, try a short synonym or translated keyword.
- Report source results independently: `ok` with no candidates means no match; `login_required` means sign in to the intended CSGHub site; `disabled`, `timeout`, `rate_limited`, `unsupported_source` and `unavailable` are not zero results. Keep usable candidates even if the other source failed. Do not request a user's token or bypass a network restriction.
- For “is it installed/enabled?” call **only** `skill_status({"name":"<installed-name>"})`. Report installed files, recorded source/version, desired enablement and disk sync. `pending` is not synced; `unknown` is not proof of sync. Don't change state to answer a question.

## 2. Install files in this worker

| Selected source | Installation action |
|---|---|
| Global library | Call `skill_install_from_catalog({"source":"global","key":"<key>"})`. The server copies the selected library entry to this worker. |
| Signed-in CSGHub site | Call `skill_install_from_catalog` with `source:"site"` and the candidate's exact `key`, `site` and `ref` when present. If login/site changes, search again. |
| skills.sh / GitHub | From this skill's directory run `node scripts/install-skills-sh.mjs <skills.sh-page-URL>` or `node scripts/install-skills-sh.mjs <owner/repo> --skill <name>`. Requires Node 22.20+, npx, Git and access to npm and the repository. The helper imports from a temporary project. |
| Explicit legacy OpenCSG/ClawHub **registry** slug | Use `csgclaw-cli --output json skill install <slug> --registry opencsg` or `--registry clawhub`. This is not the signed-in site's CSGHub repository market. |

Catalog MCP returns `name`; CLI and the script's final import result return `Slug`. Use that actual name. Only set `replace:true` (MCP) or `--force` (CLI/script) for an explicit reinstall. If installation fails, stop and report which source and step failed. A deliberate CLI export outside this worker's skill directory is a files-only operation.

## 3. Enable and report

After each successful worker install or reinstall, call `skill_ensure_enabled({"name":"<actual installed name>"})` **once** through the `csgclaw` MCP connection. Only report “installed and enabled” after `disk_synced:true`. If it fails, report “installed, not synced” and retry only enablement. Never restart the worker, write `skill-view` yourself or claim an existing session hot-reloaded.
