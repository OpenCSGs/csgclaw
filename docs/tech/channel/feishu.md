# Feishu Channel Configuration

English | [中文](feishu.zh.md)

Feishu credentials are stored on Feishu participants, not in a standalone
`channels/feishu.toml` file. Use `csgclaw-cli participant bind` to write the
manager, worker, and admin identities into `~/.csgclaw/im/participants.json`.

CSGClaw does not read Feishu credentials from `config.toml`. The old
`channels/feishu.toml` path is not migrated automatically by this flow.

Control callbacks resolve the original task and requester from local delivery records.
Task cancellation and COT completion have independent states. COT creation and
completion failures are logged with delivery identifiers and error details.
Final COT events and completion requests are separate deliveries. Native COT client controls remain platform-managed.

The native COT stop button sends `/stop`. The channel handles this message as a control request for the task active when the message arrives, validates the requester, and uses the existing Engine cancellation path. It does not submit a new prompt or send an extra command acknowledgement.

## Commands

Bind the default human Feishu administrator:

```bash
csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind human \
  --admin \
  --open-id ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Bind a worker agent app. The secret is read from stdin and is not printed:

```bash
printf '%s' "$APP_SECRET" | csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind bot \
  --agent u-dev \
  --app-id cli_xxxxxxxxxxxxxxxx \
  --app-secret-stdin \
  --restart
```

Feishu shows tool activity and available thought events in a native COT message.
Reply text streams into independent message cards. Permission requests and user
questions use separate interactive cards; only the originating user may answer.
Use `/new` to reset the conversation. Long replies use consecutive cards.

The channel consumes existing Agent Engine events. Codex supports permission and
user-input requests; DSH currently supplies permission requests. Detached Codex
questions start one follow-up turn in the same conversation after submission.
COT updates use conservative local wire budgets: 1,024 bytes per encoded event,
16 events per request, and 16,000 bytes per complete JSON request body, including
escaping and identifiers. Thought and tool-argument deltas are split without
truncation; display titles and result summaries may be shortened with an ellipsis.
Oversized metadata fails locally. Partial append failures stop later fragments
without replaying successful fragments. These budgets are not verified Feishu
limits, and live API validation is still required.
COT creation or append failure leaves reply delivery available and produces a notice card.
If a final reply card cannot be created or updated after its bounded retries,
the dispatcher sends that final page as one independent replacement card. It
retains the original chat/thread routing and complete page content, with a stable
idempotency key and at most three attempts for transient failures. Failed
replacements do not spawn further replacements. Replacement pages share a
per-turn delivery lane: retries block later pages; permanent failure allows the
remaining pages to proceed. Idle conversation indexes are reclaimed after runs
and pending interactions finish, while detached questions retain their routing.
Streaming and interaction cards
are excluded. This does not recover pending deliveries after a process restart
or guarantee delivery while Feishu permissions or connectivity remain broken.
COT append requests have no replay key and are attempted once. Completion requests
retry transient failures up to three attempts without replaying events. Recognized
stop callbacks from locally recorded COT messages target their original turn and
can retry a failed completion. Delivery and
interaction routing state is process-local. Presentation has no format setting.

Bind the manager app:

```bash
printf '%s' "$APP_SECRET" | csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind bot \
  --agent u-manager \
  --app-id cli_xxxxxxxxxxxxxxxx \
  --app-secret-stdin \
  --restart
```

For manager, `--restart` recreates the manager runtime and returns
`restart_status=manager_recreated` when the recreate succeeds.

## Participant Shape

The persisted file keeps the normal participant store shape:

```json
{
  "participants": [
    {
      "id": "admin",
      "channel": "feishu",
      "type": "human",
      "name": "admin",
      "channel_user_ref": "ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "channel_user_kind": "open_id"
    },
    {
      "id": "dev",
      "channel": "feishu",
      "type": "agent",
      "name": "dev",
      "channel_user_kind": "app_id",
      "channel_app_config": {
        "app_id": "cli_xxxxxxxxxxxxxxxx",
        "app_secret": "your_feishu_app_secret"
      },
      "agent_id": "u-dev"
    }
  ]
}
```

`channel_app_config.app_secret` is stored on disk for runtime injection, but API
and CLI responses mask it as `present`.

## Naming Rules

- Feishu bot participants use canonical participant IDs such as `manager`,
  `dev`, or `qa`.
- The bound runtime agent remains `u-manager`, `u-dev`, or `u-qa` in
  `agent_id`.
- Feishu channel API calls and room membership use participant IDs, not agent
  IDs, Feishu `open_id`, or Feishu `app_id`.
- The default chat owner is the `feishu:admin` human participant's
  `channel_user_ref`.

## Message and Mention Semantics

- Group activation uses a structured mention that exactly matches the target
  bot's Feishu `open_id`. Plain text such as `@name` is not a reliable dispatch
  mechanism.
- After one CSGClaw-hosted bot successfully sends a structured mention to a
  different active bot through the Feishu channel, a process-local handoff feeds
  the message into the target binding's normal ingress, deduplication, and Agent
  Engine path.
- A bot mentioning itself only creates the visible Feishu message; it does not
  create another Agent Turn. Self-authored messages are filtered to prevent
  recursive execution. A manager must complete its own part in the current Turn
  and dispatch only to other bots.
- An ordinary quoted reply is not promoted to a Feishu topic. Only a real
  `thread_id` isolates the conversation and enables in-thread delivery.
- The Feishu channel attempts to load quoted content. If it cannot, the current
  message continues with only the quoted message ID.

See [the current Feishu/Agent Engine architecture](agent-engine-channel-integration.zh.md)
for the full data flow and safety boundaries.

## Security Note

Treat `app_secret` as a secret credential. Do not commit real values to public
repositories, logs, screenshots, or documentation examples.
