# 在 Web UI 显示「更新并重启」的简要修改方案

## 目标

- Web UI 左侧边栏底部继续显示当前版本号。
- 如果后台检测到有新版本，则在版本号同一行最右侧显示一个带红点的 `更新并重启` 按钮。
- 如果没有新版本，则不显示任何提示。
- 后台每隔 1 小时执行一次与 `csgclaw upgrade --check` 等价的检查逻辑。
- 用户点击按钮后，执行与 `csgclaw upgrade` 等价的升级逻辑，并完成重启。

## 现状与可复用部分

- 当前版本接口已经存在：`GET /api/v1/version`
  - 代码位置：[internal/api/handler.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/api/handler.go:142)
  - 前端已在 sidebar footer 展示版本号：
    - [web/static/app.js](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/app.js:1309)
    - [web/static/app.js](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/app.js:2438)
    - [web/static/styles.css](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/styles.css:2164)
- 服务端已经有 SSE 事件通道，可用于向 UI 推送更新状态：
  - 事件总线：[internal/im/events.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/im/events.go:5)
  - SSE 接口：[internal/api/handler.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/api/handler.go:1033)
  - 前端订阅逻辑：[web/static/app.js](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/web/static/app.js:37)
- 升级检查与安装逻辑已经存在：
  - 检查最新版本：[internal/upgrade/upgrade.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/upgrade/upgrade.go:39)
  - 下载与解压：[internal/upgrade/download.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/upgrade/download.go:21)
  - 安装 bundle：[internal/upgrade/install.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/internal/upgrade/install.go:24)
  - CLI 升级入口：[cli/upgrade/upgrade.go](/Users/russellluo/Projects/work/opencsg/projects/csgclaw/cli/upgrade/upgrade.go:38)

## 建议方案

### 1. 后台增加一个 Upgrade Manager

新增一个常驻的升级状态管理器，例如：

- `internal/upgrade/manager.go`

职责：

- 启动时立即执行一次检查。
- 之后每 1 小时检查一次。
- 在内存中保存当前升级状态，例如：
  - `current_version`
  - `latest_version`
  - `update_available`
  - `checking`
  - `upgrading`
  - `last_checked_at`
  - `last_error`
- 当 `update_available` 状态发生变化时，通过现有 SSE 总线推送事件给前端。

检查逻辑建议直接复用 `internal/upgrade.Client.Check(...)`，而不是 shell 执行 `csgclaw upgrade --check`。这样可以复用现有 semver、release asset 选择、错误处理逻辑，同时避免额外子进程。

## 2. 增加升级状态的数据结构与事件类型

建议新增一个 API 类型，例如放在：

- `internal/apitypes/types.go`

可新增：

- `UpgradeStatus`
- `UpgradeActionResponse`

同时扩展 SSE 事件载荷：

- 给 `im.Event` 增加 `Upgrade *apitypes.UpgradeStatus`
- 新增事件类型常量，例如 `upgrade.status_changed`

这样可以继续复用现有 `/api/v1/events`，不必单独再开一条 SSE 通道。

## 3. 增加两个 HTTP 接口

建议新增：

- `GET /api/v1/upgrade/status`
  - 用于前端初始化时拉取当前升级状态。
  - 返回 `UpgradeStatus`。
- `POST /api/v1/upgrade/apply`
  - 用于点击 `更新并重启` 后触发升级。
  - 返回 `202 Accepted` 更合适，因为升级和重启是异步过程。

这样前端不需要等下一次 SSE 才知道当前是否有可升级版本。

## 4. 升级执行方式要和“检查”分开

这里有一个关键点：

- `--check` 可以在当前 daemon 进程内直接调用 `internal/upgrade.Client.Check(...)`。
- `upgrade` 不能简单在当前 HTTP handler 里直接复用 `RestartIfRunning(...)`。

原因是：

- 当前服务本身就是被升级和重启的对象。
- 如果在当前 daemon 进程内直接执行“安装后 stop + serve --daemon”的完整流程，`stop` 会先把当前进程自己杀掉，后续 restart 步骤可能来不及执行完。

因此，`POST /api/v1/upgrade/apply` 建议采用“拉起外部 helper 进程”的方式：

- 方案 A，推荐：
  - 在 handler 中启动一个后台子进程，执行与 `csgclaw upgrade` 等价的命令。
  - 例如使用当前可执行文件拉起一个独立进程去完成升级。
  - HTTP 接口在 helper 成功启动后立即返回 `202`。
- 方案 B：
  - 抽取一层可复用的升级 orchestration，然后仍然由独立子进程调用。

核心原则是：

- “检查”在服务内做。
- “安装并重启”交给服务外的独立进程做。

这样最接近现有 CLI 行为，也最安全。

## 5. 配置路径需要贯通

当前 `upgrade` CLI 在重启时会使用 `globals.Config` 传递 `--config`。

如果 Web UI 所在服务不是用默认配置文件启动，而是通过 `csgclaw serve --config xxx` 启动，那么升级 helper 也必须拿到同一个 config path，否则重启后的实例可能加载错配置。

建议：

- 在 `cli/serve` 启动 `server.Run(...)` 时，把当前 config path 一并传入。
- `Upgrade Manager` 或 upgrade apply handler 持有该 config path。
- 执行升级 helper 时显式拼上 `--config <path>`。

## 6. 前端修改点

前端只改 sidebar footer 区域即可：

- 初始化时继续请求 `/api/v1/version`。
- 同时请求 `/api/v1/upgrade/status`。
- 继续复用现有 SSE 订阅；收到 `upgrade.status_changed` 时更新本地状态。
- 当 `update_available === true` 时，在版本号右侧显示按钮。
- 当 `update_available === false` 时，只显示版本号。
- 用户点击后：
  - `POST /api/v1/upgrade/apply`
  - 按钮进入 disabled / loading 状态，避免重复点击
  - UI 文案可保持简单，不需要复杂进度条
  - 服务重启期间前端会断开 SSE 和请求，这属于预期行为

样式建议：

- 在现有 `.sidebar-footer` 内改成左右布局。
- 新增一个轻量按钮样式，例如：
  - 灰白底
  - 圆角
  - 红色小圆点
  - hover 有轻微背景变化
- 保持与截图一致，不影响现有 sidebar 收缩行为。

## 7. 推荐的代码落点

- 后端
  - `internal/upgrade/manager.go`
  - `internal/apitypes/types.go`
  - `internal/im/events.go`
  - `internal/api/handler.go`
  - `internal/api/router.go`
  - `internal/server/http.go`
  - `cli/serve/serve.go`
- 前端
  - `web/static/app.js`
  - `web/static/styles.css`

## 8. 测试建议

至少补以下测试：

- `internal/upgrade`
  - 定时检查状态变化
  - 有新版本 / 无新版本 / 检查失败
- `internal/api`
  - `GET /api/v1/upgrade/status`
  - `POST /api/v1/upgrade/apply`
  - SSE 事件中包含 upgrade 状态
- `web`
  - 有更新时显示按钮
  - 无更新时不显示按钮
  - 点击后进入禁用或 loading 状态

## 9. 实施顺序

1. 先补后端 `Upgrade Manager` 与状态接口。
2. 再接入 SSE 推送 `upgrade.status_changed`。
3. 再增加 `POST /api/v1/upgrade/apply` 的 helper 进程方案。
4. 最后修改 sidebar footer 的 UI 与样式。

## 10. 一句话结论

最小且稳妥的做法是：

- 用服务内 `Upgrade Manager` 每小时复用 `internal/upgrade.Client.Check(...)` 做检查；
- 用现有 SSE 把 `update_available` 推给前端；
- 用户点击后，不在当前 daemon 内直接重启自己，而是拉起一个独立 helper 进程执行与 `csgclaw upgrade` 等价的升级流程。
