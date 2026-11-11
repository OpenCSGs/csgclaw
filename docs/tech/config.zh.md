# CSGClaw 配置

[English](config.md) | 中文

`csgclaw serve` 会使用本地配置文件中的 server 访问方式、bootstrap 模板和可选通信通道，并在首次运行时自动补齐缺失的本地状态。

## 已退役运行时的升级迁移

启动时会先迁移已有的 OpenClaw、PicoClaw 配置，再解析模板、加载 Agent。
已移除的内置模板默认值会恢复为当前 Codex 默认值，废弃的 bootstrap 运行时、镜像字段会在保存配置时删除。
已有的自定义模板选择、受支持运行时、凭据和其他设置会保留。

旧 Agent 记录会迁移到 Codex，保留身份、模型配置、资源与聊天记录。
迁移后的 Agent 会请求运行，由正常启动流程在模型配置完整时启动 Codex。
原工作目录通过 `local_workspace_dir` 继续使用，用户文件和自定义 Skill 均保留。
旧沙箱句柄、镜像、运行时专属选项及运行时专属环境变量会从活动 Agent 状态中清除。
Agent 尚无 MCP 配置快照时，会导入旧原生配置，并转换超时单位及工作目录路径。
已有显式 MCP 选择优先保留。
可读的个人记忆仅在当前 Codex 记忆文档不存在时导入。

本地模板先备份再升级落盘，已有远端模板及只读注册表在读取时适配。
新模板仅支持 `codex` 和 `dsh`。
历史活动元数据在加载消息时规范化，工具记录继续按活动消息处理。

状态与配置重写前会创建同级 `.csgclaw_backup_<日期>_<序号>` 备份。
应用数据目录外的独立 `--config` 文件会创建同级文件备份。
本地注册表备份使用相同命名规则，放在注册表目录旁。
备份可跳过容器生成且不可读的插件副本，用户工作目录仍要求完整备份成功。
已落盘的迁移不会在重复启动时再次重写。

## Codex 运行模式

在启动 CSGClaw server 前设置 `CSGCLAW_EXECUTION_MODE`：

- `standard`：允许使用运行环境提供的工具读取和修改数据，默认使用此值。
- `read_only`：允许分析对话内容和使用允许的只读工具，禁止执行命令和访问本地文件。

环境变量未设置或为空时使用 `standard`，无效值会使 server 启动失败。该配置对整个进程生效，并覆盖已有 Agent 和模板中的 `execution_mode`。Web UI 隐藏运行模式选择框。修改环境变量后需要重新启动 server。

```sh
CSGCLAW_EXECUTION_MODE=read_only csgclaw serve
```

### 模型推理能力

推理选项按所选模型的能力生成，并通过服务商目录中的 `reasoning_efforts` 字段下发。
原生 Codex 模型目录与 Web UI 使用同一套受支持档位。
Codex 模型使用经过核对的专属档位，带日期的模型快照使用相同能力。
GPT-5.6 和 GPT-6 Sol/Luna 支持 `none`、`low`、`medium`、`high`、`xhigh` 和 `max`。
GPT-6 Astra 和 GPT-6.1 Sol 支持 `low` 到 `max`，不支持 `none` 或 `minimal`。
GPT-5.4 和 GPT-5.5 支持 `none` 到 `xhigh`，不支持 `minimal` 或 `max`。
未知 Codex 模型在能力确认前只显示模型默认选项。
切换模型时，推理选项重置为模型默认。
已有配置或显式请求的档位不会被静默转换为其他档位，不受支持的请求可能被上游拒绝。

## Server 地址

`listen_addr` 是本地 HTTP server 监听的地址。

`advertise_base_url` 是 CSGClaw 传给 manager 和 worker box 的回连地址，box 会用它访问本地 HTTP server。设置后，CSGClaw 会直接使用该值，只去掉末尾的 `/`，不会再自动推断本机 IP。为空时，CSGClaw 才会回退到自动推断出的本机 IPv4 地址，并拼上监听端口。

外部客户端需要使用局域网地址或公开隧道地址时，可以设置 `advertise_base_url`。

设置该值后，Web UI 登录流程也会将它作为受信任的公开地址，用于认证回调和登录后的返回地址。反向代理或 sandbox gateway 必须将 `<advertise_base_url>/api/v1/auth/callback` 路由回 CSGClaw server。


`access_token` 用来保护需要认证的 API 路由，包括 participant channel 路由。启用鉴权时，客户端必须发送 `Authorization: Bearer <access_token>`。

`no_auth` 控制 CSGClaw 是否跳过 bearer token 检查，默认值是 `false`。仅建议在可信的本地或开发环境中设置为 `true`。

`local_single_user = true` 将仅监听回环地址的独立 Web 服务声明为单人本地实例。
启用后，Agent 记忆在站内私聊默认开启，可在 Agent 的记忆页关闭。
默认值为 `false`，共享免鉴权部署使用默认值。

记忆保存在 `~/.csgclaw/agents/<agent-id>/memory/<user-id>/`，包含 `state.json` 和纯正文 `MEMORY.md`。
实例级记忆状态保存在 `~/.csgclaw/state.json` 的 `agent_memory` 小节，仅在首次初始化或实例模式变化时更新。
Runtime 重建或切换会保留记忆，永久删除 Agent 时会清理对应记忆。
记忆页在查看时自动更新，编辑过程中暂停刷新，保留未保存的草稿。
“重置记忆”在确认后删除当前全部记忆（含手动编辑），并从历史私聊重新生成。
“清空记忆”在确认后立即删除记忆并停止当前学习，聊天记录保留，后续私聊仍可产生新记忆。
通过 Markdown 编辑三类记忆，未分类内容会归入已确认事实。
历史初始化按时间顺序分段遍历全部符合条件的私聊，累积提炼记忆，并随每段结果保存读取位置，以便重启后继续。
长消息的后续正文会在下一段继续读取。
日常聊天仍然每轮独立触发后台记忆判断，不合并批处理。
后台记忆提炼与 Agent 聊天共用流式模型网关、服务商认证和连接重试机制。

`show_upgrade` 控制 Web UI 是否展示升级操作。默认值是 `true`；仅在当前部署不能自升级时设置为 `false`，例如托管的 Kubernetes 环境。

Server 根据当前运行版本推导更新通道：`v0.5.0` 这类纯三段版本使用 `release`，预发布、开发、本地构建以及其他非纯三段版本使用 `beta`。Web UI 中的版本通道切换是一次性安装操作，不会持久化到 `config.toml`。

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

当 OpenCSG AIGateway 的模型目录包含 `auto` 时，CSGClaw 默认优先选择 `auto`。
这适用于当前登录的 OpenCSG 提供方，以及指向生产或 stg AIGateway 的自定义提供方。
已明确选择的模型会保留；如果目录中没有 `auto`，则使用第一个可用模型。

回复底部显示[本轮 token 消耗](token-usage.zh.md)；选择 `auto` 时隐藏模型名称。

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
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.codex-worker"

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
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.codex-worker"

```

### 图片生成模型

Agent 可以在档案中单独选择可选的图片生成服务商和模型，与对话模型分开配置。
当前接入 Codex Runtime 的 CSGClaw 网页对话生图工具，并复用所选服务商的凭证。
服务商声明的 `text-to-image` 模型单独缓存在 `image_models` 中；缺少任务元数据时，也会识别已适配的 GPT Image 型号。
连接检查会刷新该缓存，但目录声明不代表当前账号或上游适配器已通过真实生图请求。
图片适配器使用 OpenAI Images API 并请求 base64 输出；仅暴露其他原生请求格式的服务商需要由兼容网关完成适配。

### 视频生成模型

Agent 可以在档案中单独选择视频生成服务商和模型。服务商声明的 `text-to-video` 或 `image-to-video` 模型缓存在 `video_models` 中。
视频适配器使用 AIGateway 的 OpenAI-compatible 异步接口：`POST /v1/videos` 创建任务，`GET /v1/videos/{id}` 查询状态，完成后通过 `GET /v1/videos/{id}/content` 获取视频。Codex Runtime 通过 `csgclaw_generate_video` 工具调用该能力；生成结果作为会话视频附件保存和投递。

### 动态 Codex 或 Claude Code Profile

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"
no_auth = false
show_upgrade = true

[bootstrap]
default_manager_template = "builtin.manager-codex"
default_worker_template = "builtin.codex-worker"

```

Codex 和 Claude Code Profile 通过 Web UI 写入 agent state。CSGClaw 在 `serve` 时会嵌入启动 CLIProxyAPI，并绑定到私有 localhost 端口，因此不再需要配置固定的 CLIProxy base URL。

Worker 支持 `codex` 和 `dsh` 运行时。
Manager 使用内置 Codex CLI。
创建 Worker 时可指定 `csgclaw agent create --runtime codex ...` 或 `--runtime dsh`。
API 创建 Agent 时接受相同的运行时名称。

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

当 worker 使用 Codex runtime 时，它的本地状态会统一放在 `~/.csgclaw/agents/<agent-name>/.codex/` 下。workspace 路径是 `~/.csgclaw/agents/<agent-name>/.codex/workspace`，shell home 路径是 `~/.csgclaw/agents/<agent-name>/.codex/home`，而 `auth.json`、`config.toml`、`stderr.log` 以及 runtime metadata 等由 Codex 管理的文件会放在 `~/.csgclaw/agents/<agent-name>/.codex/home` 下。本地运行时目录不依赖沙盒提供者。

CSGClaw 会为每个 Codex runtime 启用 [Codex 本地 memory](https://learn.chatgpt.com/docs/customization/memories)，包括生成 memory 和在后续会话中复用 memory。
Memory 按 Agent 隔离存放在 `~/.csgclaw/agents/<agent-name>/.codex/home/memories/`，不会与宿主机 Codex 或其他 Agent 共享。
可以在 Agent Profile 页面的 Memory Tab 中同时启用或停用 memory 生成与复用；默认启用。
Memory 文件属于自动生成的 runtime state，必须始终生效的 Agent 规则仍应写在 `AGENTS.md` 或纳入版本管理的文档中。
重建 Codex Runtime 时会保留持久 Codex 状态，包括 workspace、原生会话历史和 thread 映射、memory 文件及索引、goals、可选认证、plugins、rules、hooks 和 installation ID。
重建时会替换自动生成的配置和模型目录、进程 metadata、queues、logs、临时文件、locks 和 shell snapshots。
删除 Agent 时仍会删除完整 Agent home，包括 memory。

对于完整的 Codex worker profile，CSGClaw 会写入 `~/.csgclaw/agents/<agent-name>/.codex/home/config.toml`，其中 OpenAI 兼容代理 provider 始终使用 `wire_api = "responses"`，因为 Codex CLI 的 app-server 路径走的是 Responses API。
当 profile 同时提供模型名称和 base URL 时，CSGClaw 会先清理 Agent 生成配置中继承的 `model_providers`，再写入自身管理的代理 provider。
已有 Agent 重新生成配置时也会执行清理，避免未使用的宿主 provider 导致只读模式的严格配置检查失败，宿主 Codex 配置保持不变。
启用 Codex 记忆时，CSGClaw 会设置 `memories.min_rollout_idle_hours = 1`，使用 Codex 支持的最短空闲窗口，让已结束的房间线程更早具备后台提取资格。
由于 Codex 会跳过当前活跃线程，CSGClaw 会在房间对话轮次成功结束后创建不可见 checkpoint，每个会话每小时最多一次，同时保持原线程继续服务当前房间。
Checkpoint 空闲满一小时后，CSGClaw 会在 Runtime 私有的 maintenance thread 上发送不可见维护轮次，因此即使用户始终不进入其他房间，也能启动记忆提取。
Checkpoint 和维护输出不会发送到房间，但后台提取与合并请求会消耗模型额度，并且仍可能被 Codex 的额度保护延后。
维护触发成功后，CSGClaw 会保留 checkpoint 30 分钟供异步记忆流水线完成，然后将其删除，避免 Codex thread store 持续增长。
生成的 provider 使用 HTTP Responses，而不是 Responses WebSocket。
如果上游明确表示不支持 Responses 端点，或内置 CLIProxy 的 Codex/ClaudeCode Responses 后端在纯文本请求上返回 5xx，CSGClaw 会保持 Codex 侧的 Responses 配置不变，并在代理后面回退到上游 chat completions。
当 Responses 不受支持时，runtime 校验会继续探测 chat completions 回退，因此错误的 base URL 不会仅因为 `/responses` 返回 `404` 就通过启动校验。
当正在使用的模型 provider 修改 base URL、API key 或 headers 时，CSGClaw 会在保存前检查新连接，并在检查失败时保留之前可用的配置。
原始上游 API key 不会写入这个文件，而是通过 `env_key = "OPENAI_API_KEY"` 注入到 runtime 环境变量。

当 worker 使用 Codex runtime 时，CSGClaw 会通过与 CSGClaw 可执行文件同目录打包的 Codex CLI 执行 `codex app-server --listen stdio://`。不会使用 `PATH`、`CSGCLAW_CODEX_PATH` 或 `CSGCLAW_CODEX_ACP_PATH`；如该内置文件缺失，请重新安装 CSGClaw。

## 运行环境与旧沙盒配置迁移

Codex 和 DSH Agent 都作为本地进程，在 CSGClaw 所在环境中运行。
无需安装 Docker 或 BoxLite，界面不再提供 Agent 沙盒开关或沙盒提供者设置。
Codex 运行权限控制和 Electron 安全沙盒仍按原策略生效。

启动时会移除废弃的 `[sandbox]` 配置段，包括 `provider`、`storage_path`、`docker_cli_path`、`home_dir_name` 和 `debian_registries_override`。
如果旧 OpenClaw/PicoClaw Agent 尚未完成迁移，原提供者配置会临时保存在 `[migrations.retired_claws.sandbox]`，供后续重试使用。
Agent 转换成功后会移除这部分内部迁移配置。
迁移失败只记录警告，不会阻止 CSGClaw 或其他本地 Agent 启动。
完成的迁移不会重复执行，失败的半成品备份会清理。

旧的 `[sandbox].provider = "csghub"` 同时控制了与容器运行无关的平台行为。
升级会通过独立的部署字段保留该行为：

```toml
[server]
deployment_mode = "csghub"
```

该字段仅用于 CSGHub 部署，普通本地安装无需配置。
显式指定的部署模式优先于旧提供者配置。
Agent 身份、模型设置、工作文件、记忆、Skills 和聊天记录都会保留。
清理无效的 `sandbox_enabled` 元数据时，仍会保留原生运行时句柄。

## Hub 配置

CSGClaw 从内置、本地和官方 registry 读取 agent 模板。`config.toml` 只允许配置内置和本地 registry；官方远端 registry 由应用管理。

即使省略 `[hub]`，CSGClaw 也会启用 `builtin`（内置只读）、`local`（本地发布）和应用管理的 `official` 远端。官方远端始终在请求时根据当前 OpenCSG 登录环境解析：生产环境使用 `https://hub.opencsg.com`，stg 环境使用 `https://opencsg-stg.com`。

```toml
[hub]
default_registry = "builtin"
default_publish_registry = "local"
[[hub.registries]]
name = "builtin"
kind = "builtin"
enabled = true

[[hub.registries]]
name = "local"
kind = "local"
path = "~/.csgclaw/hub"
enabled = true
```

字段说明：

- `default_registry`：当某个命令需要一个默认读取源 registry 时，使用这个值。
- `default_publish_registry`：当发布命令没有显式传入 registry 时，使用这个值作为默认发布目标。
- `name`：registry 标识符，供 CLI 和 API 使用。
- `kind`：在 `config.toml` 中可选值为 `builtin` 或 `local`。
- `path`：用于 `local` registry。
- `enabled`：控制该 registry 是否参与 hub 相关操作；如果省略，默认值为 `true`。

内置 `builtin` registry 是只读的；`local` 是可配置的写入目标。Web UI 会发布到应用管理、与当前 OpenCSG 登录站点对应的 `official` registry。`config.toml` 中的 remote registry 条目会被忽略，并在 CSGClaw 下次保存配置时移除。

已登录时，始终优先使用当前登录站点及其 access token 选择并访问 official registry，即使环境变量已经存在。未登录时，`CSGHUB_API_BASE_URL` 用于选择 official registry，`CSGHUB_USER_TOKEN` 用于鉴权并读取私有模板；未设置 base URL 时回退到生产 registry。`TEMPLATE_ID` 应使用 `<namespace>/<name>` 格式的模板仓库路径。

旧模板 Hub 地址 `https://csgclaw.opencsg.com` 和 `config.toml` 中的自定义远端地址不再使用；official registry 始终跟随登录环境。

## Skill 注册表配置

`csgclaw skill` 默认同时搜索两个 skill 注册表：

- **opencsg**（主）：`https://claw.opencsg.com`，优先
- **clawhub**（官方）：`https://clawhub.ai`

`skill search` 先查 opencsg，有结果即返回；仅当 opencsg 无结果时才查 clawhub.ai。

`get` / `install` 可用 `--registry opencsg` 或 `--registry clawhub` 指定；省略时先查 opencsg，再查 clawhub。

Web UI 通过本地 CSGClaw Server 请求远端 Skill catalog。Server 仍按当前登录环境解析 OpenCSG Hub：生产环境使用 `https://hub.opencsg.com`，stg 环境使用 `https://opencsg-stg.com`；显式配置的 official Hub URL 会保持不变。CLI `csgclaw skill` registry 与此独立，无论生产或 stg 登录环境都使用 `https://claw.opencsg.com`。

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

`config.toml` 只保留通用的 server、model 和 bootstrap 配置。飞书凭证保存在 `~/.csgclaw/im/participants.json` 的 Feishu participant 上，并通过 `csgclaw-cli participant bind` 写入；participant-backed 流程不读取旧 `[channels.feishu]` 配置块或 `channels/feishu.toml`。

更详细的字段说明和示例，请参阅 [飞书 Channel 配置](channel/feishu.zh.md)。

未配置 `[bootstrap].default_worker_template` 或配置为空时，默认 Worker 模板为 `builtin.codex-worker`。显式配置的模板保持不变。
