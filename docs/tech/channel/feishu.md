# Feishu Channel Configuration

English | [中文](feishu.zh.md)

Feishu credentials are stored on Feishu participants, not in a standalone
`channels/feishu.toml` file. Use `csgclaw-cli participant bind` to write the
manager, worker, and admin identities into `~/.csgclaw/im/participants.json`.

CSGClaw does not read Feishu credentials from `config.toml`. The old
`channels/feishu.toml` path is not migrated automatically by this flow.

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

## Security Note

Treat `app_secret` as a secret credential. Do not commit real values to public
repositories, logs, screenshots, or documentation examples.
