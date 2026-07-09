---
name: agent-creator
description: Mandatory skill for provisioning any new CSGClaw agent-backed participant or worker. Use immediately when the user asks to create, add, set up, or provision an agent, robot, worker, or user-facing "bot" (including GitLab, frontend, backend, QA, or other specialized workers), when dispatch needs a missing worker, or when asking which hub template fits. Always template list + match + template get + participant create --type agent --bind create --from-template with --env for secrets. Never run participant create --bind create without --from-template for a new worker. Do NOT use for task dispatch to existing workers.
---

# Agent Creator

Guide users through hub template selection and agent creation. This skill owns **all new worker provisioning**.

Use the managed CSGClaw rules in `AGENTS.md` after create for room membership or non-task IM mentions. Use `csgclaw-cli task create` after the worker exists and the user wants one-worker task handoff. Use `agent-teams` for multi-worker task handoff.

## Routing Gate (mandatory)

Before running **any** `csgclaw-cli participant create --type agent --bind create` for a **new** worker:

1. Read this skill first.
2. Run `csgclaw-cli --output json template list` and pick a template (do not skip even if the user named a capability like GitLab).
3. Run `csgclaw-cli --output json template get <template-id>`.
4. Create with `--from-template` and required `--env` values.

If dispatch or the managed CSGClaw rules say "create a worker", that means **this skill**, not the general room/member/message rules.

## When to Use

Use this skill when:

- the user asks to create, add, set up, or provision an agent, robot, worker, or user-facing "bot"
- the user names a capability (GitLab, frontend, backend, QA, review, etc.) and needs a matching worker
- `participant list` shows no suitable available worker for the required capability
- dispatch needs a new worker (pause dispatch, complete provisioning here, then resume with `csgclaw-cli task create` for one worker or `agent-teams` for multiple workers)

Do **not** use this skill when:

- reusing an existing available worker (use `csgclaw-cli task create` for one-worker task dispatch or `agent-teams` for multiple workers)
- only dispatching a task to workers that already exist
- only room/member/message CLI without creating anyone new

## Forbidden

Never run a bare worker create like:

```bash
# FORBIDDEN for new workers
csgclaw-cli participant create --type agent --bind create --name gitlab-worker --role worker
```

Never tell the worker secrets in chat instead of `--env`.

Never skip `template list` / `template get` because you think you already know the template id.

## Workflow

1. Confirm the user wants a **new** worker (or dispatch lacks one). If an available worker already matches, stop and reuse it.
2. `csgclaw-cli participant list --channel <current_channel> --type agent` — avoid duplicate names; ask reuse vs new if ambiguous.
3. `csgclaw-cli --output json template list` — match by `name`, `description`, and `role`.
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

10. Report participant id, template id, and env status. Use the managed CSGClaw rules in `AGENTS.md` for `member create` if the user wants the worker in the room. Do **not** auto-dispatch unless asked.
11. If creation fails with a runtime name conflict for the requested worker name, stop and report that the host has a stale runtime with that exact name. Do **not** silently rename `dev` to `dev-worker` or `dev-feishu`; that changes the user's requested identity and breaks the later Feishu bind.

## Commands

```bash
csgclaw-cli --output json template list
csgclaw-cli --output json template get builtin.gitlab-worker
csgclaw-cli participant list --channel csgclaw --type agent
csgclaw-cli participant create --type agent --bind create --id <slug> --agent-id u-<slug> --from-template <id> --channel csgclaw --channel-user-ref <slug> --channel-user-kind local_user_id --env KEY=VALUE ...
```

Template env vars with `default` are injected by the server; pass `--env` only for secrets and overrides.

## Operating Rules

- `--from-template` is **required** for every new worker created through this skill.
- For normal workers, pass `--id`, `--agent-id`, `--channel-user-ref`, and `--channel-user-kind`; do not rely on generated IDs.
- Prefer `csgclaw-cli` over ad hoc HTTP.
- Put global flags (`--output json`, `--endpoint`, `--token`) **before** the subcommand, e.g. `csgclaw-cli --output json template list` (not after `template list`).
- Do not start team orchestration until provisioning finishes.
- Creation success is not dispatch success.
