# CSGClaw 配置

[English](config.md) | 中文

`csgclaw onboard` 会写入 `csgclaw serve` 使用的本地配置文件。配置内容包括 server 访问方式、模型 provider、bootstrap 镜像、sandbox 隔离方式和可选通信通道。

## Bootstrap 来源

默认情况下，CSGClaw 会通过 `manager_image` 指定的 OCI 镜像引用启动 manager 和 worker box。

如果你已经提前准备好了本地 rootfs bundle，也可以改用 `manager_rootfs_path`。这适合使用镜像镜像站、离线包或 air-gapped 环境的部署场景。

`manager_image` 和 `manager_rootfs_path` 互斥，不能同时配置。

## Server 地址

`listen_addr` 是本地 HTTP server 监听的地址。

`advertise_base_url` 是 CSGClaw 传给 manager 和 worker box 的回连地址，box 会用它访问本地 HTTP server。设置后，CSGClaw 会直接使用该值，只去掉末尾的 `/`，不会再自动推断本机 IP。为空时，CSGClaw 才会回退到自动推断出的本机 IPv4 地址，并拼上监听端口。

当自动推断出的地址无法从 BoxLite box 内访问时，可以设置 `advertise_base_url`，例如使用局域网地址、隧道地址或 host alias。

## Model Provider 配置示例

### 本地 CSGHub-lite

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"

[models]
default = "csghub-lite.Qwen/Qwen3-0.6B-GGUF"

[models.providers.csghub-lite]
base_url = "http://127.0.0.1:11435/v1"
api_key = "local"
models = ["Qwen/Qwen3-0.6B-GGUF"]

[bootstrap]
manager_image = "ghcr.io/russellluo/picoclaw:2026.4.18"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

### 远程 LLM API

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"

[models]
default = "remote.gpt-5.4"

[models.providers.remote]
base_url = "https://api.openai.com/v1"
api_key = "sk-your-api-key"
models = ["gpt-5.4"]

[bootstrap]
manager_image = "ghcr.io/russellluo/picoclaw:2026.4.18"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

### 通过 CLIProxyAPI 接入本地 Codex

```toml
[server]
listen_addr = "0.0.0.0:18080"
advertise_base_url = "http://127.0.0.1:18080"
access_token = "your_access_token"

[models]
default = "codex.gpt-5.4"

[models.providers.codex]
base_url = "http://127.0.0.1:8317/v1"
api_key = "local"
models = ["gpt-5.4"]

[bootstrap]
manager_image = "ghcr.io/russellluo/picoclaw:2026.4.18"

[sandbox]
provider = "boxlite"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

## Sandbox Provider

CSGClaw 通过配置的 sandbox provider 隔离 Worker 执行环境。默认 provider 是 `boxlite`，使用仓库内 vendored BoxLite Go SDK。

如果你已经预先安装了 `boxlite` 命令行程序，可以显式切换到基于 CLI 的 provider：

```toml
[sandbox]
provider = "boxlite-cli"
home_dir_name = "boxlite"
boxlite_cli_path = "boxlite"
```

`boxlite_cli_path` 只在 `provider = "boxlite-cli"` 时使用。默认值 `boxlite` 会从 `PATH` 中查找；如果二进制安装在其他位置，可以配置为绝对路径。

CSGClaw 会为每个 agent 调用 BoxLite CLI 时显式传入 `--home`，目录由 agent 目录和 `home_dir_name` 组成，例如 `~/.csgclaw/agents/<agent-id>/boxlite`。这个显式 home 对 CSGClaw 管理的 sandbox 生效，优先于 `BOXLITE_HOME`；你手动运行 `boxlite` 且不传 `--home` 时，`BOXLITE_HOME` 仍按 BoxLite 自身规则生效。

`boxlite-cli` provider 运行时不需要 vendored Go SDK。不过源码编译和当前默认的 `boxlite` provider 仍会走 SDK 路径，所以 `make test` 等命令仍可能触发 BoxLite native library 的下载或链接。

目前 `manager_rootfs_path` 只支持默认的 `boxlite` provider；`boxlite-cli` provider 仍然要求传入镜像引用。

## Worker 覆盖示例

```json
{
  "id": "u-reviewer",
  "name": "reviewer",
  "description": "code review worker",
  "profile": "codex.gpt-5.4",
  "role": "worker"
}
```
