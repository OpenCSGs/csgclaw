# CSGClaw 配置

[English](config.md) | 中文

`csgclaw onboard` 会写入 `csgclaw serve` 使用的本地配置文件。配置内容包括 server 访问方式、模型 provider、bootstrap 镜像、sandbox 隔离方式和可选通信通道。

## Server 地址

`listen_addr` 是本地 HTTP server 监听的地址。

`advertise_base_url` 是 CSGClaw 传给 manager 和 worker box 的回连地址，box 会用它访问本地 HTTP server。设置后，CSGClaw 会直接使用该值，只去掉末尾的 `/`，不会再自动推断本机 IP。为空时，CSGClaw 才会回退到自动推断出的本机 IPv4 地址，并拼上监听端口。

当自动推断出的地址无法从 BoxLite box 内访问时，可以设置 `advertise_base_url`，例如使用局域网地址、隧道地址或 host alias。

`access_token` 用来保护需要认证的 API 路由，包括 PicoClaw bot 路由。启用鉴权时，客户端必须发送 `Authorization: Bearer <access_token>`。

`no_auth` 控制 CSGClaw 是否跳过 bearer token 检查，默认值是 `false`。仅建议在可信的本地或开发环境中设置为 `true`。

`config.toml` 中的字符串值可以通过 `${NAME}` 或 `$NAME` 引用环境变量。CSGClaw 读取配置时会展开这些变量；后续重写同一个值时，会尽量保留占位符形式。如果环境变量未设置，会展开为空字符串。

```toml
[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"
no_auth = false
```

## Model Provider 配置示例

### 本地 CSGHub-lite

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF"]

[bootstrap]
manager_image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.24.0"

[sandbox]
provider = "boxlite-sdk"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

### 远程 LLM API

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://api.openai.com/v1"
api_key = "sk-your-api-key"
models = ["gpt-5.4"]

[bootstrap]
manager_image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.24.0"

[sandbox]
provider = "boxlite-sdk"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

### 通过 CLIProxyAPI 接入本地 Codex

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[models]
default = "codex.gpt-5.4"

[models.providers.codex]
base_url = "http://127.0.0.1:8317/v1"
api_key = "local"
models = ["gpt-5.4"]

[bootstrap]
manager_image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.24.0"

[sandbox]
provider = "boxlite-sdk"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

## Sandbox Provider

CSGClaw 通过配置的 sandbox provider 隔离 Worker 执行环境。默认 provider 是 `boxlite-sdk`，使用仓库内 vendored BoxLite Go SDK。

如果你已经预先安装了 `boxlite` 命令行程序，可以显式切换到基于 CLI 的 provider：

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

`boxlite_cli_path` 只在 `provider = "boxlite-cli"` 时使用。默认值 `boxlite` 会从 `PATH` 中查找；如果二进制安装在其他位置，可以配置为绝对路径。

`debian_registries` 用于控制 BoxLite 拉取 `debian:bookworm-slim` 时的仓库顺序。若省略或为空，默认顺序为 `harbor.opencsg.com`、`docker.io`。可通过 `onboard` 持久化自定义列表：

```bash
csgclaw onboard --debian-registries "harbor.opencsg.com,docker.io"
```

CSGClaw 会为每个 agent 调用 BoxLite CLI 时显式传入 `--home`，目录由 agent 目录和 `home_dir_name` 组成，例如 `~/.csgclaw/agents/<agent-id>/boxlite`。这个显式 home 对 CSGClaw 管理的 sandbox 生效，优先于 `BOXLITE_HOME`；你手动运行 `boxlite` 且不传 `--home` 时，`BOXLITE_HOME` 仍按 BoxLite 自身规则生效。

`boxlite-cli` provider 运行时不需要 vendored Go SDK。`boxlite-sdk` 是唯一需要特殊编译处理的 sandbox provider，因为它会带入 CGO、native SDK archive，以及更大的 embed runtime。仓库现在支持两种构建形态：

- `make build`、`make test`、`make package` 会带上 `boxlite_sdk` build tag，继续编译 SDK 版 `boxlite-sdk` provider。
- `make build-without-boxlite-sdk`、`make test-without-boxlite-sdk` 不带这个 build tag，产出的二进制只排除 SDK 版 `boxlite-sdk` provider；`boxlite-cli` 和其他非 SDK provider 仍然会被编译进去。

## Channel 配置

Channel 集成是可选的。默认情况下，CSGClaw 直接使用内置 Web UI；只有在你需要接入飞书等外部消息平台时，才需要增加 channel 配置。

Channel 相关配置通常放在顶层字段下，例如 `channels.feishu`。主配置文档主要说明通用的 server、model、bootstrap 和 sandbox 配置；实际使用时，再按需补充对应的 channel 配置块。

更详细的字段说明和示例，请参阅 [飞书 Channel 配置](channel/feishu.zh.md)。
