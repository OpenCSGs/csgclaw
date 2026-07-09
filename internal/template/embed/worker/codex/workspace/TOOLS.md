# TOOLS.md - Local Tool Notes

This file records workspace-specific notes for tools and skills. It does not
grant or remove tool permissions.

## Runtime

- Workspace path: the CSGClaw-managed Codex worker workspace on the host.
- CSGClaw provides channel access through the runtime configuration.
- Codex runs on the host machine and does not require a sandbox image.

## Skills

- Local skills are under `skills/`.
- Read a skill's `SKILL.md` before following it.
- Prefer local skills before installing or fetching external skills.

## Safety

- Ask before destructive filesystem changes.
- Ask before sending messages or making external changes on the user's behalf.
- Keep secrets out of logs, memory, and chat replies.
