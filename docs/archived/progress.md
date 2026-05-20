# CSGClaw 架构对齐进展清单

本文对照 `docs/architecture.md` 与当前代码实现，梳理出可逐步推进的细粒度差异点。目标不是一次性大改，而是把后续改造拆成一批批可独立提交、可回归验证、每一步都保持可运行的增量任务。

## 使用原则

- 先加兼容层，再迁移调用方，最后删旧入口。
- 每一步只收敛一个明确差异点，避免跨层大重构。
- 每一步完成后至少补一条自动化验证，优先用现有 `go test ./...` 覆盖。
- 对外接口改造优先“新增不替换”，直到 WebUI/CLI/集成方都切到新路径后再清理旧路径。

## 当前实现概览

当前项目已经具备一个最小闭环，但与 `docs/architecture.md` 的目标架构相比，仍然更接近“单二进制 + 内嵌服务对象 + 面向当前 IM/PicoClaw 场景的专用 API”：

- CLI 入口集中在 `cmd/csgclaw/main.go`，当前对外入口以 `onboard`、`serve`、`stop` 与资源型子命令为主，内部仍保留 `_serve` 作为守护进程子进程入口。
- HTTP 路由与 handler 已拆到 `internal/api/`，`internal/server/` 主要负责 server lifecycle、mux 装配与 UI 托管。
- Agent 能力目前以 `worker` 为中心暴露，核心入口是 `GET/POST /api/v1/agents`。
- IM 能力目前以 `/api/v1/im/*` 命名空间和 SSE 为主，不是目标文档里的扁平 REST + WebSocket 结构。
- Web UI 静态资源已迁到顶层 `web/` 目录并继续由 Go embed 托管，但还不是独立构建工程。

## 差异清单

下面每个差异点都包含 4 部分：

- `目标`：`docs/architecture.md` 中的目标状态
- `现状`：当前代码实际状态
- `影响`：为什么值得单独拆出来处理
- `推荐增量步骤`：建议的最小推进单元

### A01. CLI 命令树与目标架构不一致

- `目标`：单独的 CLI 层，命令树为 `serve`、`stop`、`agent/*`、`room/*`、`user/*`、`message/*`，且非 `serve` 命令都通过 HTTP 调用服务端。
- `现状`：[`cmd/csgclaw/main.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cmd/csgclaw/main.go) 已切到 `cli.New().Execute(...)`；[`cli/`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli) 中已经补齐 `agent create/delete/status`、`room list/create/delete`、`user list/kick`，`message` 命令树和 `agent logs` 仍未实现。
- `影响`：CLI 和服务端生命周期耦合，后续很难平滑过渡到“远程可调用”的命令模型。
- `推荐增量步骤`：
  - [x] A01-1 新增 `serve` 命令并承接服务启动入口；后续已移除 `start` 兼容别名。
  - [x] A01-2 新增 `stop` 命令，先基于当前后台进程管理方式落地，哪怕暂时仍沿用现有状态目录。
  - [x] A01-3 引入 `cli/` 目录并抽出 root/serve/stop 的命令注册，`main.go` 只保留启动装配。
  - [x] A01-4 为后续资源型命令准备统一 HTTP client 和全局 flag 骨架。
  - [x] A01-5 补齐 `agent create`、`agent delete`，不要再把 `agent` 仅停留在 `status` 只读子命令。
  - [x] A01-6 新增 `room list/create/delete` 命令文件与注册逻辑，全部走 `/api/v1/rooms`。
  - [x] A01-7 新增 `user list/kick` 命令文件与注册逻辑，全部走 `/api/v1/users`。
  - [x] A01-8 更新 [`cli/app.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/app.go) 的 usage，明确暴露 `agent`、`room`、`user` 命令树，避免文档已完成但二进制帮助信息仍不可见。
  - [x] A01-9 补齐 `agent list`。
  - [ ] A01-10 补齐 `agent logs`。
补充核对（2026-04-01）：

- [`cli/agent.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/agent.go) 已补齐 `agent list`、`agent create`、`agent delete` 和原有 `agent status`，当前剩余缺口是 `agent logs`。
- [`cli/room.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/room.go) 与 [`cli/user.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/user.go) 已新增，分别承载 `room` 与 `user` 命令树。
- [`cli/app.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/app.go) 已注册 `room`、`user` 顶层入口，并在 usage 中暴露对应子命令。

### A02. CLI 还不是“纯 HTTP 客户端”

- `目标`：除 `serve` 外，CLI 不直接操作本地服务对象、文件系统或 Boxlite。
- `现状`：除 `serve`/`stop` 外，当前 CLI 资源命令已经统一通过 [`cli/http_client.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/http_client.go) 发起 HTTP 请求；已覆盖 `CreateAgent/DeleteAgent/ListRooms/CreateRoom/DeleteRoom/ListUsers/KickUser`，`GetAgentLogs` 仍未补齐。
- `影响`：一旦补 `agent/room/user/message` 命令，如果继续直接调内部服务，会和目标架构背离得更远。
- `推荐增量步骤`：
  - [x] A02-1 在 `cli/` 层先定义最小 HTTP client 接口，即使最开始只服务 `agent status` 或 `room list` 这类只读命令。
  - [x] A02-2 第一个资源命令只走 HTTP，不允许偷接内部 package。
  - [x] A02-3 给 CLI 增加 endpoint/token/output/config 四个全局入口，先允许部分 flag 未使用，但参数形态先稳定下来。
  - [x] A02-4 在 [`cli/http_client.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/http_client.go) 中补齐 `CreateAgent/DeleteAgent/GetAgentLogs/ListRooms/CreateRoom/DeleteRoom/ListUsers/KickUser` 等方法，避免后续命令继续各自拼 URL。
  - [x] A02-5 把各资源命令的请求/响应格式固定到 CLI 层，并补 table/json 的最小渲染实现；至少让 `room list`、`user list` 和 `agent status` 的输出风格一致。
  - [x] A02-6 为新增资源命令补 CLI 级测试，优先覆盖“是否发往正确 HTTP 路径”和“全局 flag 是否生效”，防止只接上命令名但没有真正走 HTTP。
  - [x] A02-7 在 [`cli/http_client.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/http_client.go) 中补齐 `GetAgentLogs` 方法
  - [x] A02-8 在 `cli/http_client.go` 中补齐 `ListAgents` 方法。

补充核对（2026-04-01）：

- [`cli/http_client.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/http_client.go) 已提供 `ListAgents`、`CreateAgent`、`DeleteAgent`、`GetAgentLogs`、`ListRooms`、`CreateRoom`、`DeleteRoom`、`ListUsers`、`KickUser` 等资源方法；当前剩余缺口是 `agent logs` 子命令入口尚未接上该方法。
- [`cli/http_client.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/http_client.go) 已统一错误提取逻辑，并补了房间/用户表格渲染，资源命令的输出风格已基本对齐。
- [`cli/app_test.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/app_test.go) 已补覆盖新增命令、usage 文案以及 API 错误消息提取的 CLI 级测试。

### A03. HTTP 健康检查路径不一致

- `目标`：`GET /api/v1/health`
- `现状`：[`internal/server/http.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/server/http.go) 暴露的是 `GET /healthz`。
- `影响`：这是最容易先做兼容新增的差异，适合作为 API 对齐的第一步。
- `推荐增量步骤`：
  - [ ] A03-1 新增 `GET /api/v1/health`，返回与 `/healthz` 相同内容。
  - [ ] A03-2 将后台启动后的健康检查优先改为访问新路径，同时保留旧路径。
  - [ ] A03-3 Web 文档切到新路径后，再评估是否保留 `/healthz` 作为兼容接口。

### A04. Agent API 资源模型不一致

- `目标`：`/api/v1/agents` 承载 create/list/get/delete/logs。
- `现状`：当前只有 `GET/POST /api/v1/agents`，而且语义上偏“创建 worker box + 同步 IM”。
- `影响`：这是服务端 API 的核心结构差异，也是 CLI 无法按目标设计推进的直接原因。
- `推荐增量步骤`：
  - [x] A04-1 新增 `GET /api/v1/agents`，直接返回 `svc.List()`。
  - [x] A04-2 新增 `POST /api/v1/agents`，复用现有 `CreateRequest`，支持 `role=worker`。
  - [x] A04-3 新增 `GET /api/v1/agents/:id`，先基于 `svc.Agent(id)` 实现详情查询。
  - [x] A04-4 在 `agent.Service` 中补删除接口，再新增 `DELETE /api/v1/agents/:id`。
  - [ ] A04-5 先定义日志查询接口与占位实现，再补齐真实 log source，避免先把 CLI/API 形态卡死。

### A05. 当前 Agent 域实现仍是单文件大服务

- `目标`：`internal/agent/` 下按职责拆分为 agent/log/store 等文件。
- `现状`：[`internal/agent/service.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/agent/service.go) 同时承载模型定义、状态持久化、runtime 管理、worker 创建、gateway box 组装等逻辑。
- `影响`：继续在这个文件上加 `delete/status/logs` 会进一步放大修改面。
- `推荐增量步骤`：
  - [x] A05-1 先把仅数据结构和常量抽到独立文件，不改行为。
  - [x] A05-2 再把状态读写 `load/save` 抽到 store 文件。
  - [x] A05-3 最后再拆 runtime/box 相关逻辑，保证每次改动都只做搬迁不改语义。

### A06. IM HTTP 路由命名空间与目标设计不一致

- `目标`：扁平资源路由 `/api/v1/rooms`、`/api/v1/users`、`/api/v1/messages`、`/api/v1/ws`，不再使用 `/im` 前缀。
- `现状`：当前核心入口是 `/api/v1/im/bootstrap`、`/api/v1/im/events`、`/api/v1/im/messages`、`/api/v1/im/conversations`、`/api/v1/im/conversations/members`，另有 `/api/v1/im/rooms` 和 `/api/v1/im/rooms/invite` 作为别名。
- `影响`：目标架构强调资源名本身足够表达语义，而当前 IM API 仍然暴露了内部 bootstrap/conversation 模型。
- `推荐增量步骤`：
  - [x] A06-1 先增加 `/api/v1/messages` 作为 `/api/v1/im/messages` 的同义入口。
  - [x] A06-2 增加 `/api/v1/rooms` 的 `GET/POST/DELETE` 骨架；初期可以先只做 `GET/POST`，并把底层仍映射到 conversation。
  - [x] A06-3 增加 `/api/v1/users` 的 `GET/DELETE` 骨架；删除动作初期可只支持“从 IM 标记离线/移出会话”中的一种最小语义。
  - [x] A06-4 在新路由稳定后，把 WebUI 从 `/api/v1/im/*` 切换到扁平路由。

### A07. IM 实时通道实现为 SSE，不是 WebSocket

- `目标`：`WS /api/v1/ws`
- `现状`：当前实时流是 `GET /api/v1/im/events` SSE；PicoClaw bridge 也是 SSE。
- `影响`：这是行为层差异，比“改路径”风险更高，不适合一开始就替换。
- `推荐增量步骤`：
  - [ ] A07-1 先保留 SSE，不动现有前端。
  - [ ] A07-2 在服务端引入独立实时层接口，先把“事件生产”和“SSE 输出”解耦。
  - [ ] A07-3 增加 `WS /api/v1/ws`，与 SSE 并存，先让 WebUI 支持二选一。
  - [ ] A07-4 WebUI 切到 WebSocket 后，再决定是否下线 `/api/v1/im/events`。

### A08. IM 域模型名称与目标文档仍有偏差

- `目标`：对外强调 rooms/users/messages。
- `现状`：[`internal/im/service.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/im/service.go) 内部已经把 Room/DM 抽象成统一 `Conversation`，HTTP 层仍大量暴露 conversation/bootstrap 概念。
- `影响`：内部统一模型本身没问题，但对外 API 和文档语义需要稳定，否则 CLI 和 WebUI 后续都要反复适配。
- `推荐增量步骤`：
  - [x] A08-1 保留内部 `Conversation`，只先改 HTTP DTO 和 handler 命名。
  - [x] A08-2 增加 `ListRooms`/`DeleteRoom`/`ListUsers`/`KickUser` 这类面向资源的 service 方法，内部仍可委托给 conversation/user 存储。
  - [x] A08-3 已将 `internal/im` 主模型和主 service 方法切到 `Room`/`CreateRoom`/`AddRoomMembers`/`DeleteRoom`/`Room`，旧 `Conversation*` API 仅保留兼容包装。

### A09. 当前 IM 缺少目标架构中的读接口

- `目标`：至少有 rooms list、users list、messages history。
- `现状`：当前有 `bootstrap` 聚合读取，但缺少目标文档中独立的资源查询接口。
- `影响`：如果不先补只读接口，CLI 和未来外部集成只能继续依赖 bootstrap 聚合数据。
- `推荐增量步骤`：
  - [x] A09-1 新增 `GET /api/v1/rooms`，先返回当前会话列表。
  - [x] A09-2 新增 `GET /api/v1/users`，先返回当前用户列表。
  - [x] A09-3 新增 `GET /api/v1/messages?room_id=...` 或兼容参数形式，先返回单会话消息历史。
  - [x] A09-4 WebUI 仍可继续用 bootstrap，等资源接口稳定后再逐步拆分初始化逻辑。

### A10. 当前 IM 缺少目标架构中的删除能力

- `目标`：`DELETE /api/v1/rooms/:id`、`DELETE /api/v1/users/:id`
- `现状`：当前只有创建会话、发消息、加成员，没有 room 删除和 user kick。
- `影响`：这是典型增量功能，适合独立完成，不依赖大重构。
- `推荐增量步骤`：
  - [x] A10-1 先补 `im.Service.DeleteConversation`，处理删除会话及持久化。
  - [x] A10-2 再补 `DELETE /api/v1/rooms/:id`。
  - [x] A10-3 再补 `KickUser` 的最小定义，当前语义定为“全局移除用户，并从所有会话与历史消息中清理；若会话剩余成员少于 2 人则一并删除”。
  - [x] A10-4 最后暴露 `DELETE /api/v1/users/:id`，并补前后端行为约束测试。

### A11. 目录结构与目标架构差异较大

- `目标`：有 `internal/api/`、`cli/`、`web/`、`migrations/` 等清晰分层目录。
- `现状`：HTTP handler 已迁到 `internal/api/`，CLI 命令装配已在 `cli/`，前端静态资源已迁到顶层 `web/`，但 `migrations/` 目录仍未引入。
- `影响`：这属于结构性重排，应该放在行为稳定之后推进，否则容易和功能改动缠在一起。
- `推荐增量步骤`：
  - [x] A11-1 先建立 `internal/api/` 并迁移一个最简单的 handler，不改路由行为。
  - [x] A11-2 再把剩余 HTTP handler 从 `internal/server/http.go` 拆出去。
  - [x] A11-3 建立 `cli/` 并迁移命令装配。
  - [x] A11-4 然后处理前端目录迁移。
  - [ ] A11-5 最后处理 `migrations/` 目录引入。

### A12. 配置文件格式与目标设计不一致

- `目标`：`config.yaml`
- `现状`：[`internal/config/config.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/config/config.go) 使用 `~/.csgclaw/config.toml`。
- `影响`：这是低优先级差异。当前 TOML 已可用，强行立刻改 YAML 会制造一次无直接收益的迁移。
- `推荐增量步骤`：
  - [ ] A12-1 先给配置层增加格式无关的 load/save 抽象。
  - [ ] A12-2 支持“优先读 YAML，回退读 TOML”，而不是直接替换。
  - [ ] A12-3 等 CLI 全局 `--config` 和迁移说明准备好后，再考虑默认写 YAML。

### A13. 存储目录布局还不是“每个域独立子目录”

- `目标`：文件系统存储由各域拥有自己的子目录，并可配合 migrations 初始化。
- `现状`：默认状态文件已切到 `~/.csgclaw/agents/state.json` 和 `~/.csgclaw/im/state.json`；目录初始化仍主要散落在配置/服务代码里。
- `影响`：域级子目录已就位，后续日志、索引和迁移脚本可以按域扩展；剩余问题主要是还没有独立的 `migrations/` runner。
- `推荐增量步骤`：
  - [x] A13-1 在 `config` 层引入域目录路径计算函数。
  - [x] A13-2 新默认写入域子目录。
  - [x] A13-3 清理旧平铺状态文件兼容逻辑，统一只使用域目录路径。

### A14. 缺少 `migrations/` 初始化机制

- `目标`：通过显式 migration/init 脚本准备文件系统布局。
- `现状`：当前靠 `onboard` 和运行时 `os.MkdirAll` 隐式创建目录与状态文件。
- `影响`：在单机最小实现里问题不大，但会让“首次初始化”和“运行时容错”边界不清晰。
- `推荐增量步骤`：
  - [ ] A14-1 先建立 `migrations/` 目录并写文档，不接入执行。
  - [ ] A14-2 提取当前隐式目录初始化逻辑，形成一个可重入的 init 函数。
  - [ ] A14-3 再决定是否由 `onboard`/`serve` 显式触发 migration runner。

### A15. Web UI 目录与目标设计不一致

- `目标`：前端在独立 `web/` 工程目录中构建，再由服务端托管构建产物。
- `现状`：当前静态资源已迁到 [`web/static/index.html`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/index.html)、[`web/static/app.js`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/app.js)、[`web/static/styles.css`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/styles.css)，并通过 [`web/ui.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/ui.go) 继续由服务端 embed 托管；但还没有独立前端构建脚手架。
- `影响`：只要前端继续迭代，这个目录迟早会成为构建和测试瓶颈；但它不应该阻塞 API/CLI 对齐。
- `推荐增量步骤`：
  - [ ] A15-1 先保持 UI 资源不动，只补对新 API 的兼容。
  - [ ] A15-2 等后端路由稳定后，再引入独立 `web/` 目录和最小构建脚手架。
  - [ ] A15-3 服务端改为优先托管构建产物，保留当前 embed 方案作为 fallback 直到新前端稳定。

### A16. `internal/server` 同时承担 HTTP 路由与 UI 托管，边界偏粗

- `目标`：HTTP API 层与静态 UI 托管在职责上清晰分开。
- `现状`：[`internal/server/http.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/server/http.go) 已只保留 server lifecycle、API mux 装配与 UI 托管接线；HTTP handler 已集中到 [`internal/api/handler.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/api/handler.go)、[`internal/api/router.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/api/router.go) 和 [`internal/api/picoclaw.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/api/picoclaw.go)。
- `影响`：继续在这里叠加新 API，会让 `server` 包逐渐变成“所有适配层”的汇总处。
- `推荐增量步骤`：
  - [x] A16-1 先把纯 HTTP handler 移到 `internal/api/`。
    完成于 2026-04-01；兼容策略：原有 HTTP 路径保持不变，只调整包内落点。
  - [x] A16-2 `internal/server` 只保留 server lifecycle、mux 装配、静态资源托管。
    完成于 2026-04-01；兼容策略：`server.Run` 继续负责订阅 IM bus 并把事件桥接到 API 层，不改变调用方式。
  - [x] A16-3 PicoClaw bridge 的 HTTP 入口再决定放 `api` 还是单独子包。
    完成于 2026-04-01；当前决定：继续保留在 `internal/api/`，但拆到独立的 [`internal/api/picoclaw.go`](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/api/picoclaw.go) 以明确这是 bridge 适配入口，而不是通用 REST handler。

### A17. 目标架构未体现 PicoClaw bridge，但当前实现强依赖它

- `目标`：目标文档只描述 Agent Manager、IM System、Web UI 三块核心能力。
- `现状`：当前 HTTP 层明确暴露 `/api/bots/*`，且 worker/manager box 的启动逻辑直接围绕 PicoClaw gateway。
- `影响`：这不是“实现瑕疵”，而是“当前产品边界已经超出架构文档”。需要先补文档决策，再动代码。
- `推荐增量步骤`：
  - [ ] A17-1 先明确 PicoClaw bridge 是长期保留的正式能力，还是过渡集成层。
  - [ ] A17-2 如果长期保留，先更新 `docs/architecture.md` 补充这条集成边界。
  - [ ] A17-3 如果只是过渡层，就在 `docs/progress.md` 后续步骤里单独标记其去留策略，避免 API 对齐时误删。

### A18. 文档之间存在“目标态”和“现状态”混写

- `目标`：`docs/architecture.md` 描述目标架构，`docs/api.md` / `docs/usage.md` 描述当前可用实现，二者边界清楚。
- `现状`：仓库里已经有现状文档，但缺少一份明确的“差异清单 + 迁移顺序”。
- `影响`：后续如果不维护这份差异清单，代码改到一半时很容易再次失去迁移上下文。
- `推荐增量步骤`：
  - [x] A18-1 建立本文件，固定“目标 vs 现状 vs 下一步”的表达方式。
  - [ ] A18-2 每完成一个增量步骤，就在本文件对应项下补充完成时间、PR/提交号、兼容策略。
  - [ ] A18-3 当某个差异已经完全收敛时，再同步回写 `docs/architecture.md` 或 `docs/api.md`。

## 推荐实施顺序

建议按下面顺序推进，尽量保证每一步都能独立上线和回退：

1. 先做只增不删的接口兼容层：A03、A04-1~A04-3、A06-1、A09。
2. 再做 CLI 骨架对齐：A01、A02。
3. 再做 IM/Agent 的缺失资源能力：A04-4~A04-5、A06-2~A06-4、A10。
4. 等行为稳定后，再做结构重排：A05、A08、A11、A16。
5. 最后处理高成本迁移项：A07、A12、A13、A14、A15、A17。

## 每一步的验收基线

后续每次增量修改，建议至少满足下面 5 条：

- [ ] `go test ./...` 通过。
- [ ] WebUI 现有主流程仍可打开、加载 bootstrap、发送消息。
- [ ] 旧接口在兼容期内仍可用，除非本次任务明确声明要移除。
- [ ] 新接口至少有一条 handler 级或 service 级测试覆盖。
- [ ] 文档同步更新当前步骤的完成状态和兼容策略。
