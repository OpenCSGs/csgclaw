# 飞书 Channel 配置

[English](feishu.md) | 中文

本文说明独立的飞书 channel 配置文件格式。

CSGClaw 通过这个文件保存目标 CSGClaw agent 对应的飞书机器人应用凭证，以及初始化流程使用的真人飞书管理员 `open_id`。

## 配置结构

飞书凭证从所选 CSGClaw `config.toml` 旁边的 `channels/feishu.toml` 读取。使用默认配置路径时，文件是 `~/.csgclaw/channels/feishu.toml`。

CSGClaw 不再从 `config.toml` 读取飞书凭证。

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

`admin_open_id` 是一个真人用户的飞书 `open_id`。

这个字段用于表示在飞书侧管理或协调 CSGClaw 的管理员用户。它不是机器人的 App ID，也不是机器人的凭证。

## 飞书应用凭证条目

每个子表，例如 `[bots.u-dev]`，都表示一个飞书机器人应用凭证条目。`bots.` 前缀是当前磁盘格式，`bots.` 后面的 key 是目标 CSGClaw **agent ID**，不是 participant ID。

子表的 key 是 CSGClaw agent ID：

- `u-manager` 是保留的 manager agent ID。
- 其他目标 agent ID 应遵循 `u-{name}` 格式，例如 `u-dev`、`u-qa`。

对于每个目标 agent ID：

- `app_id` 是该飞书机器人应用的 App ID。
- `app_secret` 是该飞书机器人应用的 App Secret。

也就是说，`u-dev`、`u-manager`、`u-qa` 是用于选择飞书凭证的 CSGClaw agent ID。Channel API 调用和房间成员应使用 participant ID，例如 `dev`、`manager`、`qa`，当它们与 agent ID 不同时不能混用。

## 命名规则

- `u-manager` 保留给 CSGClaw 的 manager agent 使用。
- 自定义目标 agent ID 应使用 `u-{name}` 格式。
- 不要把 `manager`、`dev` 这类 participant ID 当作凭证表 key，除非它同时也是实际目标 agent ID。
- 不要把真人用户的 `open_id` 用作凭证子表的 key。
- 不要把飞书应用的 `app_id` 或 `app_secret` 填到 `admin_open_id` 里。

## 示例解读

按照示例结构：

- `admin_open_id` 标识一个真人飞书用户。
- `u-manager` 标识 CSGClaw 保留的 manager agent。
- `u-dev` 标识一个由某个飞书机器人应用驱动的 CSGClaw worker agent。
- `u-qa` 标识另一个由不同飞书机器人应用驱动的 CSGClaw worker agent。

每个凭证条目都必须配置自己独立的飞书 `app_id` 和 `app_secret`。

## 安全说明

`app_secret` 属于敏感凭证，不应把真实值提交到公开仓库、日志、截图或文档示例中。
