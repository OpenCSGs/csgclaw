---
name: skill-installer
description: Find, add, install, reinstall or check an existing skill for the current CSGClaw worker (安装、添加、搜索、重装或查询技能). Use for the global library, the signed-in site's CSGHub market, skills.sh, GitHub skill URLs and npx skills requests. To author a new skill, use skill-creator instead.
---

# Install an existing skill

Use this workflow for catalog or registry skill search and installation. Do not switch sources silently. A Manager delegates installs to the target worker; it must not write another agent's files.

## 1. Find or inspect

- Start an install request with `csgclaw` MCP `skill_search`. For “帮我安装这个技能 team/demo”, use **`{"query":"team/demo"}` with no `source`**. A bare `namespace/name` does not identify GitHub, skills.sh, a legacy registry, or a particular CSGHub site.
- Default order: **this worker's installed skills (including disabled) → CSGClaw global library → signed-in OpenCSG site → skills.sh**. Stop at the first match. For an `installed` result, directly call `skill_ensure_enabled` with `{"name":"<key>","selection":"<selection>"}` and report its result. Do not download or copy the skill again.
- If the user names a specific source, pass `source: "global"`, `"site"` or `"skills_sh"`; only a local copy from that source may satisfy the search. For an exact skills.sh page, pass the full URL as `query` with `source:"skills_sh"`. A GitHub repository and known skill name can also be passed directly to the helper below. Never replace a specified source or version with a different one.
- Preserve each candidate's `source`, `key`, `selection`, `site`, `ref` and `url`. Show distinct choices for ambiguous matches. If an exact identifier returns no candidate, report the miss or ask for a link; do not invent a site, try the legacy CLI, or construct a GitHub clone command. Use synonyms only for capability/keyword searches.
- Report source results accurately: `ok` with no candidates means no match; `login_required` means sign in to the intended CSGHub site; `disabled`, `timeout`, `rate_limited`, `unsupported_source` and `unavailable` are not zero results. Do not automatically switch to a public source on an error.
- For “is it installed/enabled?” call **only** `skill_status({"name":"<installed-name>"})`. Report installed files, recorded source/version, desired enablement and disk sync. `pending` is not synced; `unknown` is not proof of sync. Don't change state to answer a question.

## 2. Install files in this worker

| Selected source | Installation action |
|---|---|
| Global library | Call `skill_install_from_catalog` with the returned `source:"global"`, `key` and exact `selection` token. If the source changed, search again and review the new candidate; never retry by name alone. |
| Signed-in CSGHub site | Call `skill_install_from_catalog` with `source:"site"` and the candidate's exact `key`, `site` and `ref` when present. If login/site changes, search again. |
| skills.sh / GitHub | From this skill's directory run `node scripts/install-skills-sh.mjs <skills.sh-page-URL>` or `node scripts/install-skills-sh.mjs <owner/repo> --skill <name>`. Requires Node 22.20+, npx, Git and access to npm and the repository. The helper imports from a temporary project. |
| Explicit legacy OpenCSG/ClawHub **registry** slug | Use `csgclaw-cli --output json skill install <slug> --registry opencsg` or `--registry clawhub`. This is not the signed-in site's CSGHub repository market. |

Catalog MCP returns `name`; CLI and the script's final import result return `Slug`. Use that actual name. A request to install/reinstall a same-origin disabled skill means the user wants it enabled: reuse a matching installation or replace for a reinstall, then enable it without asking again.

On a **same-name, different (or unknown) source** conflict, stop. Show the installed source and the requested source and **ask the user whether to overwrite**. Wait for their reply; an install/reinstall request alone is not confirmation to switch origins. Do not enable the old skill as a substitute. Only after a positive reply, retry with `replace:true` and the returned `confirmation` (CLI/helper: `--force --confirmation <token>`). If the user declines, leave files and enablement unchanged. For other install failures, report the failed source and step. A deliberate CLI export outside this worker is files-only.

## 3. Enable and report

After each successful worker install or reinstall, call `skill_ensure_enabled({"name":"<actual installed name>"})` **once** through the `csgclaw` MCP connection. Only report “installed and enabled” after `disk_synced:true`. If it fails, report “installed, not synced” and retry only enablement. Never restart the worker, write `skill-view` yourself or claim an existing session hot-reloaded.
