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
2. 缺失或与 `version` 不一致时同步 embed 模板 `agent.toml` 的 `image.ref`（见 [Embed 模板](#embed-模板)）
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

`build-all` 会构建 Web UI、递增各 embed 模板的 `version` 并同步 `image.ref`、编译 `csgclaw`（使 embed 与镜像 tag 一致），再 docker build 所有带 `Dockerfile` 的模板（**不产生 `dist/`**）：

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

## Embed 模板

内置 PicoClaw 模板位于 `internal/templates/embed/<name>/`，通过 `go:embed` 直接嵌入（`agent.toml`、`workspace/` 等）。每个 docker embed 模板在 `agent.toml` 中有 `version` 字段与对应的 `image.ref`。

**本地（PR 前）**：`make build-all` 会先递增 `version`、同步 `image.ref`，再编译 `csgclaw`（使 embed 与镜像 tag 一致），最后构建 Docker 镜像。

**GitLab CI（main）**：仅读取已提交的 `version` / `image.ref` 构建并 push 镜像，**不会**修改 `agent.toml`；当本次 push 范围内（`CI_COMMIT_BEFORE_SHA..HEAD`）embed `agent.toml` 发生变化或 `version` 相对 compare base 改变时触发构建。

| 目标 | 说明 |
|------|------|
| `make sync-docker-embed-image-refs` | 按当前 `version` 同步 `image.ref`（不递增） |
| `make bump-docker-embed-version` | 递增全部 docker embed 模板的 `version` 并同步 `image.ref` |
| `make ensure-docker-embed-manifests` | `image.ref` 缺失或与 `version` 不一致时调用 `sync-docker-embed-image-refs`（`make build` / `make test` 使用） |

带 `Dockerfile` 的模板由 `scripts/list-docker-embed-templates.sh` 发现（当前为 `picoclaw-manager` 和 `picoclaw-worker`）。

若 `image.ref` 为空或与 `version` 不一致，请执行：

```bash
make sync-docker-embed-image-refs
```

## Docker embed 镜像（可选）

本地 Docker 镜像构建为**可选项**，可能较慢。常用入口是 `make build-all`；只需单个镜像时使用下方独立 target。

### 何时会构建镜像

| 命令 | 是否构建 Docker 镜像 |
|------|----------------------|
| `make` / `make build` | 否 |
| `make build-all` | 是 |
| `make build-docker-embed-runtime-embed` | 是（递增 version + 构建镜像，不重建二进制） |

### 镜像构建目标

| 目标 | 说明 |
|------|------|
| `make build-docker-embed-images` | 递增 version 并构建所有带 `Dockerfile` 的 embed 模板 |
| `make build-picoclaw-manager-image` | 递增 manager version 并仅构建 manager 镜像 |
| `make build-picoclaw-worker-image` | 递增 worker version 并仅构建 worker 镜像 |
| `make build-docker-embed-runtime-embed` | `build-docker-embed-images` 的别名 |

兼容别名包括 `build-picoclaw-runtime-embed`、`sync-picoclaw-embed-image-refs`、`bump-picoclaw-embed-version` 等。

### 常用变量

```bash
# registry（默认值）
ACR_REGISTRY=opencsg-registry.cn-beijing.cr.aliyuncs.com

# 上游 picoclaw 基础镜像默认见 embed Dockerfile 的 ARG PICOCLAW_IMAGE
# 可选覆盖：PICOCLAW_BASE_IMAGE=registry.example/opencsghq/picoclaw:tag make build-all

# 示例：PR 前本地镜像测试（会递增 version 并更新 image.ref，如 0.1.0 -> 0.1.1）
make build-all
```

产物镜像命名：

```text
${ACR_REGISTRY}/opencsghq/picoclaw-manager:<agent.toml version>
${ACR_REGISTRY}/opencsghq/picoclaw-worker:<agent.toml version>
```

镜像构建成功后，对应的 `agent.toml` 会原地更新（`version` 与 `image.ref`）。

构建上下文为仓库根目录；Dockerfile 需要 `bin/csgclaw-cli`（linux），由 `stage-docker-embed-cli` 生成。

### BoxLite 与本地 Docker 镜像

`make build-docker-embed-images` 构建的镜像保存在 **Docker** 镜像存储中。**BoxLite 不会直接使用 Docker 镜像**；需通过 `boxlite pull …` 拉取，或使用 BoxLite 可访问的 registry。Sandbox 配置见 [docs/config.zh.md](config.zh.md)。

## 测试、格式化与清理

| 目标 | 说明 |
|------|------|
| `make test` | 先 `ensure-docker-embed-manifests`，再 `go test ./...`（不修改已提交的 version/ref） |
| `make fmt` | 格式化 `cli/`、`cmd/`、`internal/`、`web/` 下的 Go 源码 |
| `make clean` | 删除 `bin/`、`dist/`、`.gocache/` |

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

CI 发布流程见 `.github/workflows/release.yml`（tag）与 GitLab CI（tag 打 release 包；**main 分支**按已提交的 `version` 构建并 push picoclaw 镜像，不修改 `agent.toml`）。Tag release 直接使用 tag commit 中已提交的 `agent.toml`。

## 相关文档

- [docs/config.zh.md](config.zh.md) — sandbox 提供者（BoxLite、Docker、CSGHub）
- [docs/web/development.zh.md](web/development.zh.md) — Web UI 开发
- [docs/architecture.md](architecture.md) — 系统结构
- `Makefile` — target 与默认值的权威来源
