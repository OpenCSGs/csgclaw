# CSGClaw 配置

[English](config.md) | 中文

`csgclaw serve` 会使用本地配置文件中的 server 访问方式、bootstrap 镜像、sandbox 隔离方式和可选通信通道，并在首次运行时自动补齐缺失的本地状态。

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
manager_image_override = ""

[sandbox]
provider = "boxlite"
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
manager_image_override = ""

[sandbox]
provider = "boxlite"
```

### 动态 Codex 或 Claude Code Profile

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false

[bootstrap]
manager_image_override = ""

[sandbox]
provider = "boxlite"
```

Codex 和 Claude Code Profile 通过 Web UI 写入 agent state。CSGClaw 在 `serve` 时会嵌入启动 CLIProxyAPI，并绑定到私有 localhost 端口，因此不再需要配置固定的 CLIProxy base URL。

Worker 在创建时也可以显式选择 runtime kind。默认值是 `picoclaw-sandbox`。如果要创建 Codex worker，可以使用 `csgclaw agent create --runtime codex ...`，或者在 `POST /api/v1/agents` 里传 `runtime_kind: "codex"`。

`[bootstrap].manager_image_override` 留空时会使用代码内置的默认 manager image；只有在需要覆盖默认值时才设置它。

本地鉴权由 CSGClaw 统一管理：

- Codex 会优先从 `~/.codex/auth.json` 自动导入。
- Claude Code 会在 macOS 上优先探测 Keychain，未找到时再走 OAuth。
- 服务启动时会把 Codex 和 Claude Code auth 导入或刷新到 CSGClaw 管理的 CLIProxy auth 目录中，并统一写成 CLIProxy 兼容 JSON。
- 手动登录命令是 `csgclaw model auth login codex` 和 `csgclaw model auth login claude-code`。
- `CSGCLAW_CLIPROXY_AUTH_DIR` 可覆盖 CLIProxy auth 目录，默认是 `~/.csgclaw/auth`。
- `CSGCLAW_CLIPROXY_AUTO_LOGIN=0` 可关闭自动导入和探测。
- `CSGCLAW_CLIPROXY_NO_BROWSER=1` 会打印 OAuth URL，而不是自动打开浏览器。
- `CSGCLAW_CLIPROXY_DISABLE_KEYCHAIN=1` 可关闭 Claude Keychain 探测。

当 worker 使用 Codex runtime 时，它的本地状态会统一放在 `~/.csgclaw/agents/<agent-name>/.codex/` 下。workspace 路径是 `~/.csgclaw/agents/<agent-name>/.codex/workspace`，shell home 路径是 `~/.csgclaw/agents/<agent-name>/.codex/home`，而 `auth.json` 这类 Codex 自己管理的文件会直接放在 `~/.csgclaw/agents/<agent-name>/.codex` 下。这个路径会和 sandbox provider 的 home（例如 `~/.csgclaw/agents/<agent-name>/boxlite`）分开。

当 worker 使用 Codex runtime 时，CSGClaw 会在启动前自动解析 `codex-acp`；如果本地不存在，则会按需下载。你可以通过下面的环境变量覆盖默认行为：

- `CSGCLAW_CODEX_ACP_PATH`：指定本地 `codex-acp` 可执行文件路径
- `CSGCLAW_CODEX_ACP_VERSION`：固定下载版本
- `CSGCLAW_CODEX_ACP_BASE_URL`：指定下载源

## Sandbox Provider

CSGClaw 通过配置的 sandbox provider 隔离 Worker 执行环境。当前内置支持的 provider 包括：

- `boxlite`：通过本地 `boxlite` CLI 运行 Worker。
- `docker`：通过本地 Docker CLI 运行 Worker。
- `csghub`：在远端 CSGHub sandbox 运行 Worker（目前仅支持在 [AgenticHub](https://opencsg.com/agentichub) 里使用）。

默认源码构建和官方 release bundle 已经统一到基于 CLI 的 provider：

```toml
[sandbox]
provider = "boxlite"
```

对于 `provider = "boxlite"`，CSGClaw 会优先解析与 `csgclaw` 同 bundle 的 `boxlite`，只有 bundle 缺失时才回退到 `PATH`。

`debian_registries_override` 用于在你需要覆盖内置顺序时，控制 BoxLite 拉取 `debian:bookworm-slim` 的仓库顺序。若省略或为空，CSGClaw 会使用默认顺序 `harbor.opencsg.com`、`docker.io`。当 CSGClaw 写入 `config.toml` 时，会把该字段保留为空数组，方便直接原地修改：

```toml
[sandbox]
provider = "boxlite"
debian_registries_override = []
```

CSGClaw 会为每个 agent 调用 BoxLite CLI 时显式传入 `--home`，固定使用 `~/.csgclaw/agents/<agent-id>/boxlite` 作为每个 agent 的 runtime home。这个显式 home 对 CSGClaw 管理的 sandbox 生效，优先于 `BOXLITE_HOME`；你手动运行 `boxlite` 且不传 `--home` 时，`BOXLITE_HOME` 仍按 BoxLite 自身规则生效。

`boxlite` provider 运行时不需要 vendored Go SDK。当前源码构建和 release 打包都走同一条 BoxLite CLI 路径：

- `make build`、`make test`、`make run`、`make package` 都使用标准的 `boxlite` 路径。
- `boxlite` 是内置的 BoxLite sandbox provider，同时仓库也保留 `csghub` 等其他非 BoxLite provider。

如果要使用 Docker 作为 sandbox provider，可以这样配置：

```toml
[sandbox]
provider = "docker"
```

当 `provider = "docker"` 时，CSGClaw 会调用本地 `docker` CLI。默认情况下会从 `PATH` 解析 `docker`。如果你需要指定特定二进制路径，可以设置 `docker_cli_path`：

```toml
[sandbox]
provider = "docker"
docker_cli_path = "/usr/local/bin/docker"
```

## Channel 配置

Channel 集成是可选的。默认情况下，CSGClaw 直接使用内置 Web UI；只有在你需要接入飞书等外部消息平台时，才需要增加 channel 配置。

`config.toml` 只保留通用的 server、model、bootstrap 和 sandbox 配置。飞书凭证放在所选 `config.toml` 旁边的独立 `channels/feishu.toml` 文件中；`config.toml` 里的旧 `[channels.feishu]` 配置块不会被读取。

更详细的字段说明和示例，请参阅 [飞书 Channel 配置](channel/feishu.zh.md)。
