# CSGClaw 配置

[English](config.md) | 中文

`csgclaw serve` 会使用本地配置文件中的 server 访问方式、bootstrap 镜像、sandbox 隔离方式和可选通信通道，并在首次运行时自动补齐缺失的本地状态。

## Server 地址

`listen_addr` 是本地 HTTP server 监听的地址。

`advertise_base_url` 是 CSGClaw 传给 manager 和 worker box 的回连地址，box 会用它访问本地 HTTP server。设置后，CSGClaw 会直接使用该值，只去掉末尾的 `/`，不会再自动推断本机 IP。为空时，CSGClaw 才会回退到自动推断出的本机 IPv4 地址，并拼上监听端口。

当自动推断出的地址无法从 BoxLite box 内访问时，可以设置 `advertise_base_url`，例如使用局域网地址、隧道地址或 host alias。

当 sandbox provider 是 Docker Desktop 上的 `docker` 时，空的 `advertise_base_url` 会在生成 manager 和 worker 运行时配置时解析为 `http://host.docker.internal:<port>`。如果 Docker 需要使用其它回连地址，请显式设置 `advertise_base_url`。

`access_token` 用来保护需要认证的 API 路由，包括 PicoClaw participant bridge 路由。启用鉴权时，客户端必须发送 `Authorization: Bearer <access_token>`。

`no_auth` 控制 CSGClaw 是否跳过 bearer token 检查，默认值是 `false`。仅建议在可信的本地或开发环境中设置为 `true`。

`show_upgrade` 控制 Web UI 是否展示升级操作。默认值是 `true`；仅在当前部署不能自升级时设置为 `false`，例如托管的 Kubernetes 环境。

`config.toml` 中的字符串值可以通过 `${NAME}` 或 `$NAME` 引用环境变量。CSGClaw 读取配置时会展开这些变量；后续重写同一个值时，会尽量保留占位符形式。如果环境变量未设置，会展开为空字符串。

```toml
[server]
listen_addr = "0.0.0.0:${PORT}"
advertise_base_url = "http://${IP}:${PORT}"
access_token = "${ACCESS_TOKEN}"
no_auth = false
show_upgrade = true
```

## Model Provider 配置示例

### 本地 CSGHub-lite

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false
show_upgrade = true

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF"]

[bootstrap]
manager_image_override = ""
runtime_kind = "picoclaw_sandbox"

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
show_upgrade = true

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://api.openai.com/v1"
api_key = "sk-your-api-key"
models = ["gpt-5.4"]

[bootstrap]
manager_image_override = ""
runtime_kind = "picoclaw_sandbox"

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
show_upgrade = true

[bootstrap]
manager_image_override = ""
runtime_kind = "picoclaw_sandbox"

[sandbox]
provider = "boxlite"
```

Codex 和 Claude Code Profile 通过 Web UI 写入 agent state。CSGClaw 在 `serve` 时会嵌入启动 CLIProxyAPI，并绑定到私有 localhost 端口，因此不再需要配置固定的 CLIProxy base URL。

Worker 在创建时也可以显式选择 runtime kind。默认值是 `picoclaw_sandbox`。如果要创建 sandbox 里的 OpenClaw worker，可以使用 `csgclaw agent create --runtime openclaw_sandbox ...`；如果要创建 Codex worker，可以使用 `csgclaw agent create --runtime codex ...`。API 在 `POST /api/v1/agents` 的 `runtime_kind` 中接受同样的值。

`[bootstrap].manager_image_override` 留空时会使用代码内置的默认 manager image；只有在需要覆盖默认值时才设置它。
bootstrap manager 当前固定使用 `picoclaw_sandbox`；`openclaw_sandbox` 支持用于 worker，不支持作为 manager runtime。

本地鉴权由 CSGClaw 统一管理：

- Codex 会优先从 `~/.codex/auth.json` 自动导入。
- Claude Code 会在 macOS 上优先探测 Keychain，未找到时再走 OAuth。
- 服务启动时会把 Codex 和 Claude Code auth 导入或刷新到 CSGClaw 管理的 CLIProxy auth 目录中，并统一写成 CLIProxy 兼容 JSON。
- 手动登录命令是 `csgclaw model auth login codex` 和 `csgclaw model auth login claude-code`。
- `CSGCLAW_CLIPROXY_AUTH_DIR` 可覆盖 CLIProxy auth 目录，默认是 `~/.csgclaw/auth`。
- `CSGCLAW_CLIPROXY_AUTO_LOGIN=0` 可关闭自动导入和探测。
- `CSGCLAW_CLIPROXY_NO_BROWSER=1` 会打印 OAuth URL，而不是自动打开浏览器。
- `CSGCLAW_CLIPROXY_DISABLE_KEYCHAIN=1` 可关闭 Claude Keychain 探测。

当 worker 使用 Codex runtime 时，它的本地状态会统一放在 `~/.csgclaw/agents/<agent-name>/.codex/` 下。workspace 路径是 `~/.csgclaw/agents/<agent-name>/.codex/workspace`，shell home 路径是 `~/.csgclaw/agents/<agent-name>/.codex/home`，而 `auth.json`、`config.toml`、`stderr.log` 以及 runtime metadata 等由 Codex 管理的文件会放在 `~/.csgclaw/agents/<agent-name>/.codex/home` 下。这个路径会和 sandbox provider 的 home（例如 `~/.csgclaw/agents/<agent-name>/boxlite`）分开。

对于完整的 Codex worker profile，CSGClaw 会写入 `~/.csgclaw/agents/<agent-name>/.codex/home/config.toml`，其中 OpenAI 兼容代理 provider 始终使用 `wire_api = "responses"`，因为 Codex CLI 的 app-server 路径走的是 Responses API。生成的 provider 使用 HTTP Responses，而不是 Responses WebSocket。如果上游明确表示不支持 Responses 端点，或内置 CLIProxy 的 Codex/ClaudeCode Responses 后端在纯文本请求上返回 5xx，CSGClaw 会保持 Codex 侧的 Responses 配置不变，并在代理后面回退到上游 chat completions。原始上游 API key 不会写入这个文件，而是通过 `env_key = "OPENAI_API_KEY"` 注入到 runtime 环境变量。

当 worker 使用 Codex runtime 时，CSGClaw 会通过 `codex app-server --listen stdio://` 启动本地 `codex` CLI。你可以通过下面的环境变量覆盖二进制查找行为：

- `CSGCLAW_CODEX_PATH`：指定本地 `codex` 可执行文件路径。Windows 支持 npm 的 `codex.cmd`/`codex.bat` shim 和原生 `codex.exe`；CSGClaw 会通过 `cmd.exe` 启动命令 shim，但不支持 `codex.ps1`。
- `CSGCLAW_CODEX_ACP_PATH`：迁移期间的兼容回退项，也指向同一个 `codex` 可执行文件路径

## OpenClaw Runtime

CSGClaw 的 bootstrap manager 默认使用 PicoClaw。若要创建 sandbox 中的 OpenClaw worker，请在创建 worker 时显式指定 runtime：

```bash
csgclaw agent create --name alice --runtime openclaw_sandbox
```

推荐镜像形态是基于 OpenClaw slim 二次封装，并把 CSGClaw 管理的插件烘焙到 `/home/node/openclaw-plugins`（例如 `csgclaw-extension` 和外部 channel 插件）。运行时状态仍由 `~/.csgclaw/agents/<agent>/.openclaw/openclaw.json` 提供；不要把空的宿主机目录挂载到 `/home/node/openclaw-plugins`，否则会遮住镜像内已经烘焙好的插件。

CSGClaw 为普通 OpenAI-compatible profile 生成保守的 OpenClaw bridge 模型元数据。
默认使用 `openai-completions`、`input: ["text"]`，不声明 reasoning effort 支持，也不写入 `agents.defaults.thinkingDefault`。
这样可以避免 OpenClaw 向未通过 CSGClaw 声明能力的 provider 发送图片或 reasoning payload。
使用 Codex profile 时，CSGClaw 会把 bridge 模型声明为 `openai-codex-responses`，启用 `input: ["text", "image"]`，写入 `low`、`medium`、`high`、`xhigh` reasoning 档位，并写入 streaming usage 兼容元数据。
Codex 的 `reasoningEffortMap` 会把 `minimal` 映射到 `low`，其他档位按同名值透传。

## Sandbox Provider

CSGClaw 通过配置的 sandbox provider 隔离 Worker 执行环境。当前内置支持的 provider 包括：

- `boxlite`：通过本地 `boxlite` CLI 运行 Worker。
- `docker`：通过本地 Docker CLI 运行 Worker。
- `csghub`：在远端 CSGHub sandbox 运行 Worker（目前仅支持在 [AgenticHub](https://opencsg.com/agentichub) 里使用）。

官方 bundle 目前有两种布局：

- `csgclaw/bin/csgclaw` 加 `csgclaw/bin/boxlite`
- 只有 `csgclaw/bin/csgclaw`

如果 `[sandbox].provider` 省略或为空，CSGClaw 默认使用 `docker`。

这也是为什么生成出来的配置文件可以把 provider 留空，直接跟随默认值：

```toml
[sandbox]
provider = ""
```

你也可以随时显式覆盖默认值：

```toml
[sandbox]
provider = "boxlite"
```

```toml
[sandbox]
provider = "docker"
```

对于 `provider = "boxlite"`，CSGClaw 会优先解析与 `csgclaw` 同 bundle 的 `boxlite`，只有 bundle 缺失时才回退到 `PATH`。如果两者都找不到，启动会直接报带操作建议的错误，而不是静默改写 provider。

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

当前平台上的默认行为如下：

- provider 留空时会解析成 `docker`。
- Windows 用户需要确保本地 Docker 可用并且能从 `PATH` 找到；如果路径特殊，可以显式设置 `[sandbox].docker_cli_path`。

## Hub 配置

CSGClaw 可以从一个或多个 hub registry 读取 agent 模板。registry 配置是可叠加的：内置、本地和远端 registry 可以同时存在于同一个 `config.toml` 中。

即使省略 `[hub]`，CSGClaw 也会默认启用三个 registry：`builtin`（内置只读）、`local`（本地发布）和 `official`（官方远端 `https://hub.opencsg.com`）。你在 `config.toml` 里只写了部分 registry 时，缺失的默认项会自动合并进来，因此删掉 `builtin` 也不会导致启动失败。

```toml
[hub]
default_registry = "builtin"
default_publish_registry = "local"
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.picoclaw-worker"

[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

[[hub.registries]]
name = "local"
kind = "local"
path = "~/.csgclaw/hub"
enabled = true

[[hub.registries]]
name = "official"
kind = "remote"
url = "https://hub.opencsg.com"
enabled = true

[[hub.registries]]
name = "team"
kind = "remote"
url = "https://hub.example.com"
token = "${CSGCLAW_HUB_TOKEN}"
enabled = true
```

字段说明：

- `default_registry`：当某个命令需要一个默认读取源 registry 时，使用这个值。
- `default_publish_registry`：当发布命令没有显式传入 registry 时，使用这个值作为默认发布目标。
- `default_manager_template`：当某个流程需要隐式选择 manager 模板时，使用这个值。
- `default_worker_template`：当某个流程需要隐式选择 worker 模板时，使用这个值。
- `name`：registry 标识符，供 CLI 和 API 使用。
- `kind`：可选值为 `builtin`、`local` 或 `remote`。
- `path`：用于 `local` registry。
- `url` 和 `token`：用于 `remote` registry。
- `enabled`：控制该 registry 是否参与 hub 相关操作；如果省略，默认值为 `true`。

内置 `builtin` registry 是只读的。发布模板时应选择可写的 `local` 或 `remote` registry。

## Skill 注册表配置

`csgclaw skill` 默认同时搜索两个 skill 注册表：

- **opencsg**（主）：`https://claw.opencsg.com`，优先
- **clawhub**（官方）：`https://clawhub.ai`

`skill search` 先查 opencsg，有结果即返回；仅当 opencsg 无结果时才查 clawhub.ai。

`get` / `install` 可用 `--registry opencsg` 或 `--registry clawhub` 指定；省略时先查 opencsg，再查 clawhub。

当前 registry 支持的 API：

- `GET /api/v1/search` — `csgclaw skill search`
- `GET /api/v1/skills/:slug` — `csgclaw skill get`（OpenCSG 响应含 `versions[]`）
- `GET /api/v1/skills/:slug/versions` — `csgclaw skill versions`（clawhub.ai 分页；OpenCSG 回退到 get 的 `versions[]`）
- `GET /api/v1/skills/:slug/versions/:version` — `csgclaw skill get --version`
- `GET /api/v1/download/:slug` 或 `GET /api/v1/download?slug=` — 安装时下载

请用 `search` 浏览 skill；当前 registry 没有 catalog 列表接口（无 `GET /api/v1/skills`）。

```toml
[skill]
base_url = "https://claw.opencsg.com"
official_base_url = "https://clawhub.ai"
token = "${SKILL_TOKEN}"
non_suspicious_only = true
```

- `base_url`：主注册表（opencsg）；也可通过 `SKILL_BASE_URL` 覆盖（兼容旧名 `CLAWHUB_BASE_URL`）。
- `official_base_url`：次注册表（clawhub.ai），默认 `https://clawhub.ai`；设为 `""` 可关闭双库搜索；也可通过 `SKILL_OFFICIAL_BASE_URL` 覆盖（兼容 `CLAWHUB_OFFICIAL_BASE_URL`）。
- `token`：只读命令可选，后续 publish 需要；也可通过 `SKILL_TOKEN` 设置（兼容 `CLAWHUB_TOKEN`）。
- 旧版 `[clawhub]` 配置段仍可读取，用于兼容。
- `non_suspicious_only`：省略时默认为 `true`。

## Channel 配置

Channel 集成是可选的。默认情况下，CSGClaw 直接使用内置 Web UI；只有在你需要接入飞书等外部消息平台时，才需要增加 channel 配置。

`config.toml` 只保留通用的 server、model、bootstrap 和 sandbox 配置。飞书凭证保存在 `~/.csgclaw/im/participants.json` 的 Feishu participant 上，并通过 `csgclaw-cli participant bind` 写入；participant-backed 流程不读取旧 `[channels.feishu]` 配置块或 `channels/feishu.toml`。

更详细的字段说明和示例，请参阅 [飞书 Channel 配置](channel/feishu.zh.md)。
