# Feishu Channel Configuration

English | [中文](feishu.zh.md)

This document explains the standalone Feishu channel configuration file.

CSGClaw uses this file to map one human Feishu administrator and multiple Feishu bot applications into local CSGClaw identities.

## Configuration Structure

Feishu credentials are loaded from `channels/feishu.toml` next to the selected CSGClaw `config.toml`. With the default config path, the file is `~/.csgclaw/channels/feishu.toml`.

CSGClaw does not read Feishu credentials from `config.toml`.

```toml
[global]
admin_open_id = "ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

[bots.u-dev]
app_id = "cli_xxxxxxxxxxxxxxxx"
app_secret = "your_feishu_app_secret"

[bots.u-manager]
app_id = "cli_xxxxxxxxxxxxxxxx"
app_secret = "your_feishu_app_secret"

[bots.u-qa]
app_id = "cli_xxxxxxxxxxxxxxxx"
app_secret = "your_feishu_app_secret"
```

## `admin_open_id`

`admin_open_id` is the Feishu `open_id` of a real human user.

Use this field for the human administrator who manages or coordinates CSGClaw from Feishu. It is not a bot app ID and it is not a bot credential.

## Bot Entries

Each nested table such as `[bots.u-dev]` defines one Feishu bot application for one CSGClaw bot identity.

The table key is the CSGClaw bot ID:

- `u-manager` is a reserved ID.
- Other bot IDs should follow the `u-{name}` format, such as `u-dev` or `u-qa`.

For each bot ID:

- `app_id` is the Feishu bot application's App ID.
- `app_secret` is the Feishu bot application's App Secret.

In other words, `u-dev`, `u-manager`, and `u-qa` are CSGClaw bot IDs, while the values inside each table are Feishu bot credentials.

## Naming Rules

- `u-manager` is reserved for the manager bot used by CSGClaw.
- Custom bot IDs should use the `u-{name}` pattern.
- Do not use a human user's `open_id` as a bot table key.
- Do not place bot `app_id` or `app_secret` under `admin_open_id`.

## Example Interpretation

Given the sample structure:

- `admin_open_id` identifies one real Feishu user.
- `u-manager` identifies the reserved CSGClaw manager bot.
- `u-dev` identifies a CSGClaw bot backed by one Feishu bot app.
- `u-qa` identifies another CSGClaw bot backed by another Feishu bot app.

Each bot entry must have its own Feishu `app_id` and `app_secret`.

## Security Note

Treat `app_secret` as a secret credential. Do not commit real values to public repositories, logs, screenshots, or documentation examples.
