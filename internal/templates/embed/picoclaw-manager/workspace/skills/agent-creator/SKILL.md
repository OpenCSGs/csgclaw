---
name: agent-creator
description: Mandatory skill for provisioning any new CSGClaw agent, bot, robot, or worker. Use immediately when the user asks to create, add, set up, or provision an agent/bot/worker (including GitLab, frontend, QA, or other specialized workers), when dispatch needs a missing worker, or when asking which hub template fits. Always hub list + match + hub get + bot create --from-template with --env for secrets. Never run bot create without --from-template for a new worker. Do NOT use for task dispatch to existing workers or todo.json tracking only.
---

# Agent Creator

Guide users through hub template selection and agent creation. This skill owns **all new worker provisioning**.

Use `basics` only after create for room membership or IM mentions. Use `manager-worker-dispatch` only after the worker exists and the user wants task handoff.

## Routing Gate (mandatory)

Before running **any** `csgclaw-cli bot create` for a **new** worker:

1. Read this skill first.
2. Run `csgclaw-cli --output json hub list` and pick a template (do not skip even if the user named a capability like GitLab).
3. Run `csgclaw-cli --output json hub get <template-id>`.
4. Create with `--from-template` and required `--env` values.

If dispatch or any other skill says "create a worker", that means **this skill**, not `basics`.

## When to Use

Use this skill when:

- the user asks to create, add, set up, or provision an agent, bot, robot, or worker
- the user names a capability (GitLab, frontend, QA, review, etc.) and needs a matching worker
- `bot list` shows no suitable available worker for the required capability
- dispatch needs a new worker (pause dispatch, complete provisioning here, then resume with `basics` + dispatch)

Do **not** use this skill when:

- reusing an existing available worker (use `basics` + dispatch)
- only dispatching a task to workers that already exist
- only room/member/message CLI without creating anyone new

## Forbidden

Never run a bare worker create like:

```bash
# FORBIDDEN for new workers
csgclaw-cli bot create --name gitlab-worker --role worker --runtime picoclaw_sandbox
```

Never tell the worker secrets in chat instead of `--env`.

Never skip `hub list` / `hub get` because you think you already know the template id.

## Workflow

1. Confirm the user wants a **new** worker (or dispatch lacks one). If an available worker already matches, stop and reuse it.
2. `csgclaw-cli bot list --channel <current_channel>` — avoid duplicate names; ask reuse vs new if ambiguous.
3. `csgclaw-cli --output json hub list` — match by `name`, `description`, and `role`.
4. No match → say so plainly; do not fall back to bare `bot create`.
5. Multiple matches → short comparison; let the user choose.
6. `csgclaw-cli --output json hub get <template-id>` — read `image_env`.
7. Collect every `required=true` env with no `default`; never echo `secret=true` values.
8. Confirm `name`, optional `--id`, and `--description` (template description is a good default).
9. Create:

```bash
csgclaw-cli bot create \
  --name gitlab-worker \
  --description "GitLab issue and MR worker" \
  --role worker \
  --from-template <matched-template-id> \
  --env GITLAB_TOKEN=<user-provided> \
  --channel <current_channel>
```

10. Report bot id, template id, and env status. Use `basics` for `member create` if the user wants the worker in the room. Do **not** auto-dispatch unless asked.

## Commands

```bash
csgclaw-cli --output json hub list
csgclaw-cli --output json hub get builtin.gitlab-worker
csgclaw-cli bot list --channel <current_channel>
csgclaw-cli bot create --from-template <id> --env KEY=VALUE ... --channel <current_channel>
```

Template env vars with `default` are injected by the server; pass `--env` only for secrets and overrides.

## Operating Rules

- `--from-template` is **required** for every new worker created through this skill.
- Prefer `csgclaw-cli` over ad hoc HTTP.
- Put global flags (`--output json`, `--endpoint`, `--token`) **before** the subcommand, e.g. `csgclaw-cli --output json hub list` (not after `hub list`).
- Do not use `manager-worker-dispatch` until provisioning finishes.
- Creation success is not dispatch success.
