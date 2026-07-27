# CSGHub CSGClaw server 镜像启用本地 Codex：最小改造

## 目标与结论

目标是在 CSGHub 创建的 **一个 `csgclaw-server` 容器**中运行：

```text
csgclaw serve
├─ Codex manager app-server
└─ Codex worker app-server(s)
```

实现先要完成两项**必需的镜像改动**：在 `csgclaw-server` 最终镜像中安装可执行的
Codex CLI，以及把未指定模板的新 worker 默认切到内建 Codex。然后才是选择 base image
和发布 server wrapper tag。CSGClaw Go 代码和 CSGBot 不需要重写，是因为上述镜像和默认
模板就能接入已有 runtime；它们不是本方案的改动主体。

理由是 CSGClaw 的 `codex` 已是本地 runtime：它在 CSGClaw 所在环境直接执行
`codex app-server --listen stdio://`。因此 CSGHub server sandbox 只要具备相同二进制
和默认模板，运行行为与个人电脑部署一致。`runtime_kind=codex` 不会进入
sandbox gateway，也不会为 manager/worker 调 CSGHub `Create`。

旧 PicoClaw/OpenClaw sandbox capability 保留，但不是这个 server 镜像的默认
manager/worker 路径。

## 改动清单

### 1. `csgclaw-server/Dockerfile`

这是本方案的核心改动。最终 server 镜像必须在**构建期**得到 Codex 原生二进制，不能把
安装留到容器启动后。

1. 将 `CSGCLAW_BASE_IMAGE` 更新为
   `opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/csgclaw:v0.3.18`。
2. 在 Dockerfile 顶部固定下载根地址：

   ```dockerfile
   ARG CSGCLAW_CODEX_DOWNLOAD_BASE_URL=https://csgclaw.opencsg.com/codex-cli/latest
   ```

   在 `FROM ${CSGCLAW_BASE_IMAGE}` 后为当前 stage 重新声明该 `ARG`（Docker 的作用域
   要求），安装 `curl`，并按 `TARGETARCH` 下载：

   ```text
   https://csgclaw.opencsg.com/codex-cli/latest/linux/<amd64|arm64>?package=codex-cli
   ```

   下载的是包含 musl 原生二进制的 tar.gz。构建脚本必须：映射 `amd64`/`arm64` 到 archive
   中的对应文件名、解包、以 `0755` 安装到 `/opt/codex/bin/codex`、软链到
   `/usr/local/bin/codex`，最后执行 `codex --version`。这一步失败必须让 image build
   失败。

   该 URL 只作为 Dockerfile build arg 的默认值；**不**通过 Makefile、CI pipeline variable
   或最终容器的环境变量暴露。每次构建取得当前 `latest`，因此并不承诺固定 Codex 版本。
3. 设置 `CSGCLAW_CODEX_PATH=/usr/local/bin/codex`，使 CSGClaw 在启动时优先使用镜像内
   CLI，不触发运行期自动安装。
4. 将基座中直接位于 `/usr/local/bin` 的 `csgclaw` / `csgclaw-cli` 规范化为
   `/opt/csgclaw/bin/...` 的官方 bundle 布局，并保留原命令路径的软链。
   `v0.3.18` 基座本身没有这个布局，而正式版本的 `csgclaw serve` 会校验它；不做这
   一步会在启动前直接退出，与 Codex 无关。

不要依赖 `csgclaw serve` 的运行期自动下载；它失败只会告警，可能造成 HTTP server
健康而 Codex manager 不可用。

这次 Dockerfile 改动不触及以下兼容层：

- UID/GID 1000 和 `/home/picoclaw/.csgclaw` PVC 布局；
- supervisor、tini、Python sandbox :8888；
- `CSGCLAW_AGENT_TEMPLATE_IMAGE`、hub 模板的 image 替换；
- `csgclaw-agent` 相关构建能力。

### 2. `csgclaw-server/config.toml`（仅控制默认新 worker）

只替换 worker 默认模板：

```toml
[bootstrap]
default_worker_template = "builtin.codex-worker"
```

不需要修改 `default_manager_template`。实际 manager 在
`internal/agent/service.go:465-488` 中固定为 Codex，不从这个配置选择 runtime。

`default_worker_template` 也不改变已有 worker，更不改变用户显式选择的模板。它只在
创建请求没有 `from_template` 时被 `templateRefForCreateSpec` 作为 worker 默认模板
（`internal/agent/service.go:1028-1045`）使用。当前 server 镜像写的是
`local/picoclaw-worker`，所以普通“新建 worker”会默认走 PicoClaw sandbox；改成
`builtin.codex-worker` 才使这个默认操作走容器内 Codex。

若前端/API 始终显式传 `from_template = "builtin.codex-worker"`，则连这一个
`config.toml` 改动也不需要。

其余配置行保持原样，尤其是：

```toml
[server]
advertise_base_url = "${CSGCLAW_ADVERTISE_BASE_URL}"

[sandbox]
provider = "csghub"
```

不能将 `advertise_base_url` 固定为 `127.0.0.1`。当前 CSGClaw 同时用它：

- 给 Codex profile 构造 CSGClaw LLM bridge URL；
- 给独立 sandbox 中的 OpenClaw/PicoClaw 生成 `CSGCLAW_BASE_URL` 回调地址。

OpenClaw/PicoClaw 若在另一个容器，`127.0.0.1` 是它自己的 loopback，无法回调
server。最小方案沿用 CSGBot 注入的外部可达 Gateway URL；Codex 也通过这个已有地址
访问 CSGClaw LLM bridge。

### 3. 保持 `docker-entrypoint.sh` 的启动契约

entrypoint 当前每次启动都会把镜像内的 `config.toml` 和 `hub/` 复制进 PVC，并要求
现有的模型/CSGHub 环境变量。最小方案不改变这些行为。

这也意味着：必须修改并发布镜像内的 `config.toml`；在运行中手改 PVC 的 config 会被
下一次启动覆盖。

### 4. 构建与发布

`Makefile` 的 `CSGCLAW_BASE_IMAGE_NAME` 默认值为 `v0.3.18`，并把它传给 Dockerfile
的 `CSGCLAW_BASE_IMAGE`。Codex 下载地址不在 Makefile 中配置，始终使用 Dockerfile
顶部的默认 URL。

GitLab 的 `image:csgclaw-server-sandbox` 是普通 `.ci/images/*.env` 机制的例外：创建
protected `main` pipeline 时必须传入：

```text
CSGCLAW_BASE_IMAGE_NAME=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/csgclaw:<base-tag>
CSGCLAW_SERVER_SANDBOX_IMAGE_NAME=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsg_public/csgclaw-server-sandbox:<server-tag>
```

CI 从已提交的 `csgclaw-agent-sandbox.env` 读取兼容模板使用的 agent image；调用者不需
传第三个变量。构建成功只会推送 server wrapper image；仍须在目标环境的 CSGBot 配置中
将 `[sandbox.csgclaw-server].image` 更新为新 tag 并部署，之后新创建的 sandbox 才会采用
新镜像。

## 实施范围外（在上述改动完成后保持不变）

| 路径/仓库 | 原因 |
| --- | --- |
| `csgclaw/` | 本地 Codex runtime、session/bridge 和 CSGHub gateway provider 都已具备。 |
| `csgbot/` | 继续创建外层 server sandbox，并继续提供当前的环境变量、PVC、健康检查和旧 sandbox 配置。 |
| `csgclaw-server/docker-entrypoint.sh` | 当前配置复制和环境变量校验可继续使用。 |
| `csgclaw-server/hub/` | 默认路径改为 builtin Codex template，不需要修改 local hub。 |
| `csgclaw-agent/` | 保留旧 OpenClaw/PicoClaw sandbox capability；默认 Codex 路径不会启动它。 |

## 验收

1. 使用无缓存构建新 server 镜像，确认日志从
   `https://csgclaw.opencsg.com/codex-cli/latest/linux/<arch>?package=codex-cli` 下载
   archive、打印 `codex --version`，且不再出现 Node/npm 或 Docker Hub 的 Codex 安装步骤；
   再以 `--entrypoint /usr/local/bin/codex` 运行 `codex --version`，确认 CLI 在最终镜像
   可执行。
2. 使用现有 CSGBot 创建 CSGClaw server sandbox，确认容器用户可写
   `/home/picoclaw/.csgclaw`。
3. 确认 manager 为 `runtime_kind=codex`；未显式选择模板的新建 worker 也为
   `runtime_kind=codex`（若未改 worker 默认模板，则显式选择 `builtin.codex-worker`）。
4. 观察 CSGHub 审计：创建上述 manager/worker 时不应产生新的 sandbox `Create`；
   仅有 CSGBot 创建的外层 server sandbox。
5. 发送一次真实消息，确认 Codex 能通过现有
   `CSGCLAW_ADVERTISE_BASE_URL` 到达 CSGClaw LLM bridge 并完成响应。

若第 5 步失败，先检查镜像中 Codex 的 stderr、AI Gateway 的 Responses 接口和
`CSGCLAW_ADVERTISE_BASE_URL` 的容器可达性；这些不是 CSGClaw runtime 重写问题。
