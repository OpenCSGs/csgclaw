# Feishu Channel Configuration

English | [中文](feishu.zh.md)

This document explains the standalone Feishu channel configuration file.

CSGClaw uses this file to store Feishu bot application credentials for target CSGClaw agents, plus the human Feishu administrator `open_id` used by setup flows.

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

## Feishu App Credential Entries

Each nested table such as `[bots.u-dev]` defines one Feishu bot application credential entry. The `bots.` prefix is the current on-disk format, but the key after `bots.` is a target CSGClaw **agent ID**, not a participant ID.

The table key is the CSGClaw agent ID:

- `u-manager` is the reserved manager agent ID.
- Other target agent IDs should follow the `u-{name}` format, such as `u-dev` or `u-qa`.

For each target agent ID:

- `app_id` is the Feishu bot application's App ID.
- `app_secret` is the Feishu bot application's App Secret.

In other words, `u-dev`, `u-manager`, and `u-qa` are CSGClaw agent IDs used to select Feishu credentials. Channel API calls and room membership should use participant IDs, such as `dev`, `manager`, or `qa`, when those differ from agent IDs.

## Naming Rules

- `u-manager` is reserved for the manager agent used by CSGClaw.
- Custom target agent IDs should use the `u-{name}` pattern.
- Do not use a participant ID, such as `manager` or `dev`, as a credential table key unless it is also the real target agent ID.
- Do not use a human user's `open_id` as a credential table key.
- Do not place Feishu app `app_id` or `app_secret` under `admin_open_id`.

## Example Interpretation

Given the sample structure:

- `admin_open_id` identifies one real Feishu user.
- `u-manager` identifies the reserved CSGClaw manager agent.
- `u-dev` identifies a CSGClaw worker agent backed by one Feishu bot app.
- `u-qa` identifies another CSGClaw worker agent backed by another Feishu bot app.

Each credential entry must have its own Feishu `app_id` and `app_secret`.

## Security Note

Treat `app_secret` as a secret credential. Do not commit real values to public repositories, logs, screenshots, or documentation examples.
