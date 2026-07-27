# 飞书 Channel 配置

[English](feishu.md) | 中文

飞书凭证保存在 Feishu participant 上，不再保存在独立的
`channels/feishu.toml` 文件中。请使用 `csgclaw-cli participant bind` 将
manager、worker 和 admin 身份写入 `~/.csgclaw/im/participants.json`。

CSGClaw 不从 `config.toml` 读取飞书凭证。旧的 `channels/feishu.toml` 路径不会在本流程中自动迁移。

## 命令

绑定默认的飞书真人管理员：

```bash
csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind human \
  --admin \
  --open-id ou_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

绑定 worker agent 的飞书应用。secret 从 stdin 读取，不会打印：

```bash
printf '%s' "$APP_SECRET" | csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind bot \
  --agent u-dev \
  --app-id cli_xxxxxxxxxxxxxxxx \
  --app-secret-stdin \
  --restart
```

绑定 manager 应用：

```bash
printf '%s' "$APP_SECRET" | csgclaw-cli participant bind \
  --channel feishu \
  --feishu-kind bot \
  --agent u-manager \
  --app-id cli_xxxxxxxxxxxxxxxx \
  --app-secret-stdin \
  --restart
```

对 manager 使用 `--restart` 时会重建 manager runtime；重建成功后返回 `restart_status=manager_recreated`。

## Participant 结构

落盘文件仍保持普通 participant store 结构：

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

`channel_app_config.app_secret` 会真实保存在磁盘上用于 runtime 注入，但 API 和 CLI 响应会统一脱敏为 `present`。

## 命名规则

- Feishu bot participant 使用 canonical participant ID，例如 `manager`、`dev` 或 `qa`。
- 绑定的 runtime agent 仍通过 `agent_id` 表示，例如 `u-manager`、`u-dev` 或 `u-qa`。
- Feishu channel API 调用和房间成员使用 participant ID，不使用 agent ID、飞书 `open_id` 或飞书 `app_id`。
- 默认群主来自 `feishu:admin` human participant 的 `channel_user_ref`。

## 安全说明

`app_secret` 属于敏感凭证，不应把真实值提交到公开仓库、日志、截图或文档示例中。
