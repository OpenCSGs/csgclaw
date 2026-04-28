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
manager_image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.27.0"

[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
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
manager_image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.27.0"

[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

### 动态 Codex 或 Claude Code Profile

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[bootstrap]
manager_image = "opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.4.27.0"

[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

Codex 和 Claude Code Profile 通过 Web UI 写入 agent state。CSGClaw 在 `serve` 时会嵌入启动 CLIProxyAPI，并绑定到私有 localhost 端口，因此不再需要配置固定的 CLIProxy base URL。

## Sandbox Provider

CSGClaw 通过配置的 sandbox provider 隔离 Worker 执行环境。默认构建形态使用 `boxlite-cli`，通过外部 CLI 进程运行 BoxLite；启用 SDK 的构建形态仍默认使用 `boxlite-sdk`，对应仓库内 vendored BoxLite Go SDK。

默认源码构建和官方 release bundle 已经统一到基于 CLI 的 provider：

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
debian_registries = ["harbor.opencsg.com", "docker.io"]
```

对于 `provider = "boxlite-cli"`，CSGClaw 会优先解析与 `csgclaw` 同 bundle 的 `boxlite`，只有 bundle 缺失时才回退到 `PATH`。

`debian_registries` 用于控制 BoxLite 拉取 `debian:bookworm-slim` 时的仓库顺序。若省略或为空，默认顺序为 `harbor.opencsg.com`、`docker.io`。可通过 `onboard` 持久化自定义列表：

```bash
csgclaw onboard --debian-registries "harbor.opencsg.com,docker.io"
```

CSGClaw 会为每个 agent 调用 BoxLite CLI 时显式传入 `--home`，目录由 agent 目录和 `home_dir_name` 组成，例如 `~/.csgclaw/agents/<agent-id>/boxlite`。这个显式 home 对 CSGClaw 管理的 sandbox 生效，优先于 `BOXLITE_HOME`；你手动运行 `boxlite` 且不传 `--home` 时，`BOXLITE_HOME` 仍按 BoxLite 自身规则生效。

`boxlite-cli` provider 运行时不需要 vendored Go SDK。`boxlite-sdk` 是唯一需要特殊编译处理的 sandbox provider，因为它会带入 CGO、native SDK archive，以及更大的 embed runtime。仓库现在支持两种构建形态：

- `make build`、`make test`、`make run`、`make onboard`、`make package` 使用默认的 `boxlite-cli` 构建形态。产出的二进制只排除 SDK 版 `boxlite-sdk` provider；`boxlite-cli` 和其他非 SDK provider 仍然会被编译进去。
- `make build-with-boxlite-sdk`、`make test-with-boxlite-sdk`、`make run-with-boxlite-sdk`、`make onboard-with-boxlite-sdk` 会带上 `boxlite_sdk` build tag，继续编译 SDK 版 `boxlite-sdk` provider。

## Channel 配置

Channel 集成是可选的。默认情况下，CSGClaw 直接使用内置 Web UI；只有在你需要接入飞书等外部消息平台时，才需要增加 channel 配置。

Channel 相关配置通常放在顶层字段下，例如 `channels.feishu`。主配置文档主要说明通用的 server、model、bootstrap 和 sandbox 配置；实际使用时，再按需补充对应的 channel 配置块。

更详细的字段说明和示例，请参阅 [飞书 Channel 配置](channel/feishu.zh.md)。
