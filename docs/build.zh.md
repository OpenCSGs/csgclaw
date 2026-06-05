# 构建指南

[English](build.md) | 中文

本文说明仓库 `Makefile` 中的本地开发、测试、打包，以及可选的 PicoClaw embed 镜像构建命令。

请在仓库根目录执行以下命令。随时可运行：

```bash
make help
```

## 前置条件

- Go 工具链（版本见 `go.mod`）
- Web UI 需要 `pnpm`（`scripts/web-pnpm.sh` 会检查安装）
- 本地构建 embed 模板镜像时需要 Docker
- 使用 BoxLite sandbox 运行或测试时需要 `boxlite` CLI（见 [docs/config.zh.md](config.zh.md)）

## 默认构建

默认目标是 `build`：

```bash
make
# 等价于：
make build
```

会依次构建：

1. Web UI 产物到 `web/static-dist/`
2. 缺失时的 embed 模板 `dist/` 目录（见 [Embed dist](#embed-dist)）
3. `bin/csgclaw`（服务端 CLI）
4. 当前平台的 `bin/csgclaw-cli`

**不会构建 Docker 镜像。** 需要完整本地环境（含 embed 镜像）时使用 `make build-all`。

构建完成后可运行：

```bash
./bin/csgclaw serve
# 或
make run
```

## 完整构建

`build-all` 先执行 `build`，再 docker build 所有带 `Dockerfile` 的 embed 模板并 patch 镜像 ref：

```bash
make build-all
```

需要本地 `picoclaw-manager` / `picoclaw-worker` 镜像时使用，可能较慢。

## Web UI

| 目标 | 说明 |
|------|------|
| `make web-install` | 安装 Web UI 依赖（`pnpm install --frozen-lockfile`） |
| `make web-dev` | 启动 Vite 开发服务器 |
| `make build-web` | 构建 Web UI 到 `web/static-dist/` |

缺少 `node_modules` 时，`make build-web` 会自动执行 `web-install`。

前端结构与验证说明见 [docs/web/development.zh.md](web/development.zh.md)。

## Go 二进制

| 目标 | 输出 | 说明 |
|------|------|------|
| `make build-server-bin` | `bin/csgclaw`、`bin/csgclaw-cli` | 当前平台两个二进制；CLI 使用 `CGO_ENABLED=0` |
| `make stage-docker-embed-cli` | `bin/csgclaw-cli`（linux，宿主机架构） | 打入 PicoClaw Docker 镜像的 Linux CLI |

`csgclaw-cli` 使用 `CGO_ENABLED=0` 构建，以便在 musl 环境的 PicoClaw / BoxLite 沙箱中运行。Release CI 同样使用该设置（`scripts/release-build-all.sh`）。

## Embed dist

内置 PicoClaw 模板源码位于 `internal/templates/embed/<name>/`。供 `go:embed` 使用的运行时文件在各模板的 `dist/` 目录，由本地或 CI 生成。

| 目标 | 说明 |
|------|------|
| `make prepare-docker-embed-dist` | 将模板源码复制到 `embed/*/dist/`（不打 Docker） |
| `make patch-docker-embed-image-refs` | 修补 `dist/agent.toml` 中的镜像 ref（默认 tag `:dev`） |
| `make stage-docker-embed-dist` | `prepare` + `patch` |
| `make ensure-docker-embed-dist` | 缺失时自动 stage dist（`build-all` 使用） |

带 `Dockerfile` 的模板由 `scripts/list-docker-embed-templates.sh` 发现（当前为 `picoclaw-manager` 和 `picoclaw-worker`）。

若 `make build-server-bin` 报错 `pattern embed/.../dist: no matching files found`，请执行：

```bash
make stage-docker-embed-dist
```

## Docker embed 镜像（可选）

本地 Docker 镜像构建为**可选项**，可能较慢。常用入口是 `make build-all`；只需单个镜像时使用下方独立 target。

### 何时会构建镜像

| 命令 | 是否构建 Docker 镜像 |
|------|----------------------|
| `make` / `make build` | 否 |
| `make build-all` | 是 |
| `make build-docker-embed-runtime-embed` | 是（仅镜像 + patch refs，不重建二进制） |

### 镜像构建目标

| 目标 | 说明 |
|------|------|
| `make build-docker-embed-images` | 构建所有带 `Dockerfile` 的 embed 模板 |
| `make build-picoclaw-manager-image` | 仅构建 manager 镜像 |
| `make build-picoclaw-worker-image` | 仅构建 worker 镜像 |
| `make build-docker-embed-runtime-embed` | 构建 linux CLI、docker build 全部模板、patch refs |

兼容别名包括 `build-picoclaw-runtime-embed`、`stage-picoclaw-embed-dist` 等。

### 常用变量

```bash
#  registry 与 tag（默认值）
ACR_REGISTRY=opencsg-registry.cn-beijing.cr.aliyuncs.com
DOCKER_EMBED_IMAGE_TAG=dev
PICOCLAW_BASE_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.27

# 示例：自定义 tag 构建 worker 镜像
make build-picoclaw-worker-image DOCKER_EMBED_IMAGE_TAG=local
```

产物镜像命名：

```text
${ACR_REGISTRY}/opencsghq/picoclaw-manager:${DOCKER_EMBED_IMAGE_TAG}
${ACR_REGISTRY}/opencsghq/picoclaw-worker:${DOCKER_EMBED_IMAGE_TAG}
```

构建上下文为仓库根目录；Dockerfile 需要 `bin/csgclaw-cli`（linux），由 `stage-docker-embed-cli` 生成。

### BoxLite 与本地 Docker 镜像

`make build-docker-embed-images` 构建的镜像保存在 **Docker** 镜像存储中。**BoxLite 不会直接使用 Docker 镜像**；需通过 `boxlite pull …` 拉取，或使用 BoxLite 可访问的 registry。Sandbox 配置见 [docs/config.zh.md](config.zh.md)。

## 测试、格式化与清理

| 目标 | 说明 |
|------|------|
| `make test` | 先 `stage-docker-embed-dist`，再 `go test ./...` |
| `make fmt` | 格式化 `cli/`、`cmd/`、`internal/`、`web/` 下的 Go 源码 |
| `make clean` | 删除 `bin/`、`dist/`、`.gocache/` 及 embed 生成的 `dist/` 内容 |

针对性测试：

```bash
go test ./internal/agent/ -run TestName
go test ./...
```

## 打包与发布

维护者常用目标：

| 目标 | 说明 |
|------|------|
| `make package` | 构建 Web UI 并打包当前平台二进制 |
| `make package-all` | 完整构建并打包 `csgclaw` 与 `csgclaw-cli` |
| `make release` | 跨平台 release bundle（darwin/linux，arm64/amd64） |

CI 发布流程见 `.github/workflows/release.yml` 及 GitLab 中的 embed dist / 镜像 job。

## 相关文档

- [docs/config.zh.md](config.zh.md) — sandbox 提供者（BoxLite、Docker、CSGHub）
- [docs/web/development.zh.md](web/development.zh.md) — Web UI 开发
- [docs/architecture.md](architecture.md) — 系统结构
- `Makefile` — target 与默认值的权威来源
