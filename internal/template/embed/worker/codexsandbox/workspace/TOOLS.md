# TOOLS.md - Local Tool Notes

This file records workspace-specific notes for tools and skills. It does not
grant or remove tool permissions.

## Runtime

- Workspace path in the container: `~/.codex-sandbox/workspace`
- Project mount path in the container: `~/.codex-sandbox/workspace/projects`
- Bridge config path in the container: `~/.codex-sandbox/config.json`
- Codex home path in the container: `~/.codex-sandbox/codex-home`
- CSGClaw provides model access through runtime configuration. Feishu/Lark
  channel access is available when runtime app config is bound.

## Skills

- Local skills are under `skills/`.
- Read a skill's `SKILL.md` before following it.
- Prefer local skills before installing or fetching external skills.

## Safety

- Ask before destructive filesystem changes.
- Ask before sending messages or making external changes on the user's behalf.
- Keep secrets out of logs, memory, and chat replies.
