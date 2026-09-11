---
name: agent-creator
description: Provision a new CSGClaw agent-backed participant only when the user explicitly asks to create, add, set up, or provision an agent, robot, worker, or user-facing bot. Always template list + match + template get + participant create --type agent --bind create --from-template with --env for secrets, preserving the exact listed template ID. Never infer provisioning from an ordinary work request and do not use this skill for existing workers.
---

# Agent Creator

Guide users through hub template selection and agent creation. This skill owns **all new worker provisioning**.

Use the CSGClaw administration rules in `AGENTS.md` after creation only when the user explicitly asks for room membership or a message.

## Routing Gate (mandatory)

Before running **any** `csgclaw-cli participant create --type agent --bind create` for a **new** worker:

1. Read this skill first.
2. Run `csgclaw-cli --output json template list` and pick a template (do not skip even if the user named a capability like GitLab).
3. Run `csgclaw-cli --output json template get <template-id>` using the exact ID returned by `template list`.
4. Create with that exact ID in `--from-template` and add required `--env` values. Remote template IDs use `<namespace>/<name>` such as `Agentic/gitlab-assistant`; do not rewrite them as URLs or `official.namespace/name`.

## When to Use

Use this skill when:

- the user asks to create, add, set up, or provision an agent, robot, worker, or user-facing "bot"
- the user explicitly asks for a new worker with a named capability such as GitLab, frontend, backend, QA, or review

Do **not** use this skill when:

- reusing an existing available worker
- completing an ordinary task that the Manager can perform directly
- only room/member/message CLI without creating anyone new

## Forbidden

Never run `participant create --type agent` for a new CSGClaw worker without `--bind create` or `--bind reuse`.

Never omit `--from-template` when using `--bind create` for a new worker.

Never create a CSGClaw worker as a participant-only shell.
That produces a private-chat identity without a runnable Agent.

Never tell the worker secrets in chat instead of `--env`.

Never skip `template list` / `template get` because you think you already know the template id.

## Workflow

1. Confirm the user explicitly wants a **new** worker. If the request is ordinary work, leave this skill and complete that work directly. If an available worker already matches and the user's intent is ambiguous, ask whether they want to reuse it or create another.
2. `csgclaw-cli participant list --channel <current_channel> --type agent` — avoid duplicate names; ask reuse vs new if ambiguous.
3. `csgclaw-cli --output json template list` — match by `name`, `description`, and `role`; preserve the returned `id` exactly. Remote IDs use `<namespace>/<name>`.
4. No match → say so plainly; do not fall back to bare `participant create --bind create`.
5. Multiple matches → short comparison; let the user choose.
6. `csgclaw-cli --output json template get <template-id>` — read `image_env`.
7. Collect every `required=true` env with no `default`; never echo `secret=true` values.
8. Confirm `name`, optional `--id`, and `--description` (template description is a good default).
9. Create. For a user-facing worker name like `dev`, keep the identity stable: participant id `dev`, agent id `u-dev`, and CSGClaw local user ref `dev`. When this skill is invoked before Feishu setup, create the base worker in the `csgclaw` channel first; the Feishu skill will bind Feishu credentials afterwards.

```bash
csgclaw-cli participant create --type agent --bind create \
  --id gitlab-worker \
  --agent-id u-gitlab-worker \
  --name gitlab-worker \
  --description "GitLab issue and MR worker" \
  --role worker \
  --from-template <matched-template-id> \
  --channel csgclaw \
  --channel-user-ref gitlab-worker \
  --channel-user-kind local_user_id \
  --env GITLAB_TOKEN=<user-provided> \
```

10. Report participant id, template id, and env status. Use the administration rules in `AGENTS.md` for `member create` only if the user also asked to add the worker to a room. Do **not** assign work automatically.
11. If creation fails with a runtime name conflict for the requested worker name, stop and report that the host has a stale runtime with that exact name. Do **not** silently rename `dev` to `dev-worker` or `dev-feishu`; that changes the user's requested identity and breaks the later Feishu bind.

## Commands

```bash
csgclaw-cli --output json template list
csgclaw-cli --output json template get builtin.gitlab-worker
csgclaw-cli --output json template get Agentic/gitlab-assistant
csgclaw-cli participant list --channel csgclaw --type agent
csgclaw-cli participant create --type agent --bind create --id <slug> --agent-id u-<slug> --from-template <id> --channel csgclaw --channel-user-ref <slug> --channel-user-kind local_user_id --env KEY=VALUE ...
```

Template env vars with `default` are injected by the server; pass `--env` only for secrets and overrides.

## Operating Rules

- `--bind create` and `--from-template` are **required** for every new worker created through this skill.
- Always pass the exact `template list` ID to `template get` and `--from-template`: `builtin.*` and `local.*` remain registry-qualified IDs, while remote templates use `<namespace>/<name>`.
- For normal workers, pass `--id`, `--agent-id`, `--channel-user-ref`, and `--channel-user-kind`; do not rely on generated IDs.
- Prefer `csgclaw-cli` over ad hoc HTTP.
- Put global flags (`--output json`, `--endpoint`, `--token`) **before** the subcommand, e.g. `csgclaw-cli --output json template list` (not after `template list`).
- Creation success does not imply any room membership, message, or work assignment.
