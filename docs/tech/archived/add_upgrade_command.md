# `csgclaw upgrade` 方案

## 目标

新增一个顶级命令 `csgclaw upgrade`，用于：

- 比对当前版本和 `https://csgclaw.opencsg.com/releases/latest` 返回的最新版本
- 在存在新版本时，自动下载当前平台对应的 release bundle
- 校验并安装新版本
- 默认按“升级后重启服务”执行

同时支持：

```bash
csgclaw upgrade
csgclaw upgrade --check
csgclaw upgrade --no-restart
```

## 建议的命令语义

### `csgclaw upgrade`

- 检查是否有新版本
- 若无新版本，直接提示当前已是最新版本
- 若有新版本，下载并安装当前平台对应的 bundle
- 默认尝试重启本地 `csgclaw` 服务

### `csgclaw upgrade --check`

- 只检查版本，不下载、不安装、不重启
- 适合作为脚本探测或用户手动确认前置动作

### `csgclaw upgrade --no-restart`

- 下载并安装新版本
- 不重启服务
- 如果旧服务仍在运行，提示“新版本已安装，需手动重启后生效”

## 为什么要按 bundle 升级

当前 release 打包不是单一二进制，而是目录型 bundle。`csgclaw` release 内至少包含：

- `bin/csgclaw`
- `bin/boxlite-cli`

因此升级时不建议只替换 `csgclaw` 单个文件，否则可能出现：

- 主程序已升级，但 bundled `boxlite-cli` 还是旧版本
- 二进制与 bundle 内其他文件不匹配
- 不同安装方式下路径推断失真

建议 `upgrade` 仅面向“官方 release bundle 安装”提供完整自动升级能力；对源码构建或手工复制单文件的场景，只支持 `--check`，或明确报出“当前安装方式不支持自动升级”。

## 最新版本接口使用方式

接口：

```text
GET https://csgclaw.opencsg.com/releases/latest
```

返回关键字段：

- `version`: 最新版本号，例如 `v0.2.7`
- `assets`: 各平台构建产物
- `download_base_url`: 当前版本的下载根路径

建议策略：

1. 读取 `internal/version.Current()` 作为当前版本。
2. 使用 semver 规则比较当前版本与远端 `version`。
3. 忽略 `csgclaw-cli_*` 资产，只匹配 `csgclaw_*` 资产。
4. 根据 `runtime.GOOS` 和 `runtime.GOARCH` 选择目标文件，例如：
   `csgclaw_v0.2.7_darwin_arm64.tar.gz`
5. `download_url` / `download_base_url` 若为相对路径，则以 `https://csgclaw.opencsg.com` 补全。

## 推荐执行流程

### 1. 检查阶段

- 拉取 latest 元数据
- 比较版本
- 输出：
  - 当前版本
  - 最新版本
  - 是否可升级
  - 匹配到的目标 asset 名称

`--check` 到这里结束。

### 2. 下载阶段

- 下载目标 `tar.gz` 到临时目录
- 同时记录：
  - `name`
  - `size`
  - `sha256`
- 下载完成后做：
  - 文件大小校验
  - SHA256 校验

任一校验失败直接中止，不进入安装阶段。

### 3. 解压与预检查

- 解压到临时目录
- 校验 bundle 结构至少包含：
  - `csgclaw/bin/csgclaw`
  - `csgclaw/bin/boxlite-cli`
- 记录候选安装目录和备份目录

## 安装策略

建议使用“同目录原子替换”：

1. 通过 `os.Executable()` 找到当前运行的 `csgclaw` 路径。
2. 校验其是否符合官方 bundle 结构，例如可回溯到 `<install-root>/bin/csgclaw`。
3. 将解压后的新 bundle 放到同级临时目录。
4. 将旧目录重命名为备份目录，例如 `csgclaw.backup.<timestamp>`。
5. 将新目录重命名为正式安装目录。
6. 安装成功后按策略删除或延迟清理备份目录。

这样做的好处：

- 避免边覆盖边运行
- 保留失败回滚空间
- 一次替换完整 bundle，而不是只换单文件

## 重启策略

这是方案里最需要约束的部分。建议只处理“本地 daemon 服务重启”，不尝试无边界地重启所有运行方式。

### 默认行为

`upgrade` 默认等价于：

1. 安装新版本
2. 如果检测到 daemon 正在运行，则执行重启
3. 如果未检测到 daemon，则仅提示“升级完成，当前无运行中的服务”

### daemon 检测

复用现有约定：

- PID 文件：`~/.csgclaw/server.pid` 或 `serve --pid` 指定路径
- 配置文件：`--config` 或默认 `~/.csgclaw/config.toml`

建议首版只支持“默认 PID 文件路径”下的自动重启。因为如果用户历史上使用了自定义 `--pid`、`--log`、监听地址或其他启动参数，仅凭当前 upgrade 进程无法可靠恢复全部上下文。

### 重启实现

建议复用现有命令链路，而不是直接手写 server lifecycle：

1. 安装完成后，若检测到 daemon 运行：
2. 执行当前 CLI 的 `stop` 逻辑关闭旧进程
3. 使用新安装的 `csgclaw` 执行 `serve --daemon`
4. 等待 `/healthz` 成功，作为重启完成标志

这样有几个优点：

- 与现有 `serve` / `stop` 行为一致
- 复用现有 PID、日志、健康检查逻辑
- 降低 upgrade 命令自己管理进程的复杂度

### `--no-restart`

- 跳过 daemon 停止和拉起
- 返回成功，但明确提示：
  - 新版本已安装
  - 当前运行中的服务仍是旧版本
  - 需要手动执行 `csgclaw stop && csgclaw serve --daemon`

## 建议的边界

首版建议明确以下边界，避免一开始做成高复杂度的“万能自升级”：

- 支持平台：与 release 资产一致的 `darwin/linux` + 对应 `amd64/arm64`
- 支持安装形态：官方 bundle 安装
- 支持自动重启的运行形态：daemon 模式
- 不保证支持：
  - `go run` / 源码构建直接运行
  - 用户手动复制单个二进制后的非 bundle 安装
  - 前台运行中的原地自替换和自重启
  - 历史上用自定义 PID / log / 启动参数拉起的 daemon 完整恢复

这些场景可以先返回清晰错误或降级为“仅完成安装，不自动重启”。

## CLI 输出建议

### `upgrade --check`

```text
Current version: v0.2.5
Latest version:  v0.2.7
Update available: yes
Asset: csgclaw_v0.2.7_darwin_arm64.tar.gz
```

### `upgrade`

```text
Current version: v0.2.5
Latest version:  v0.2.7
Downloading csgclaw_v0.2.7_darwin_arm64.tar.gz
Verifying checksum
Installing new bundle
Restarting service
Upgrade completed: v0.2.7
```

### `upgrade --no-restart`

```text
Upgrade completed: v0.2.7
Restart skipped
Run `csgclaw stop` and `csgclaw serve --daemon` to apply the new version
```

## 代码落点建议

- `cli/upgrade/`
  - 新增命令实现
  - 参数解析：`--check`、`--no-restart`
  - 用户输出
- `internal/upgrade/`
  - latest API client
  - 版本比较
  - asset 选择
  - 下载、校验、解压、安装
  - 重启编排
- `cli/app.go`
  - 注册 `upgrade` 顶级命令

这样可以保持：

- `cmd/` 继续保持薄
- `cli/` 管命令行为
- `internal/` 管升级领域逻辑

## 测试建议

至少覆盖：

1. 版本比较：
   - 当前已是最新
   - 当前低于远端
   - 当前为 `dev` 时的行为
2. asset 选择：
   - 正确匹配 `goos/goarch`
   - 过滤掉 `csgclaw-cli_*`
   - 缺少匹配资产时返回明确错误
3. 下载校验：
   - size 不匹配
   - sha256 不匹配
4. 安装流程：
   - 非 bundle 安装拒绝自动升级
   - bundle 替换成功
   - 替换失败时可回滚
5. 重启流程：
   - 无 PID 文件
   - PID 文件存在但进程不存在
   - 正常 stop + serve --daemon
   - `--no-restart` 跳过重启

## 推荐实现顺序

建议按“先做最小闭环，再逐步引入安装与重启复杂度”的顺序推进。

### 阶段 1：先落地 `upgrade --check`

目标：

- 能拉取 latest 元数据
- 能比较当前版本与远端版本
- 能为当前 `goos/goarch` 选出正确的 `csgclaw_*` asset
- 能输出“是否有新版本”

建议先实现：

- `internal/upgrade/latest.go`
- `internal/upgrade/version.go`
- `internal/upgrade/assets.go`
- `cli/upgrade/` 的 `--check` 路径

这样可以先把最核心的协议、版本判断和平台匹配跑通，而且不涉及文件替换和进程管理。

### 阶段 2：补齐下载、校验、解压

目标：

- 下载目标 release archive
- 校验 size / sha256
- 解压到临时目录
- 校验 bundle 结构合法

建议实现：

- `internal/upgrade/download.go`
- 解压与 bundle 预检查逻辑

这一阶段先不要安装到正式目录，只把 release 消费链路打通。

### 阶段 3：实现安装替换，先支持 `--no-restart`

目标：

- 识别当前是否为官方 bundle 安装
- 将新 bundle 原子替换到安装目录
- 失败时支持回滚
- 支持 `upgrade --no-restart`

建议实现：

- `internal/upgrade/install.go`
- `cli/upgrade/` 的安装路径

这样可以先完成“检查 -> 下载 -> 安装”的完整升级闭环，但先不把重启问题混进来。

### 阶段 4：最后补默认重启

目标：

- 检测 daemon 是否运行
- 复用 `stop` 停掉旧进程
- 用新版本执行 `serve --daemon`
- 等待 `/healthz` 成功

建议实现：

- `internal/upgrade/restart.go`
- `cli/upgrade/` 默认 `upgrade` 路径

首版建议只支持默认 PID 路径下的自动重启，不要一开始就尝试恢复历史上的自定义 `--pid`、`--log` 或其他启动参数。

### 阶段 5：最后再补文档与 CLI 帮助

包括：

- `docs/tech/cli.md`
- `docs/tech/cli.zh.md`
- 命令 help 文案
- 常见失败场景提示

这样可以避免接口和行为还在变动时就过早扩写外部文档。

## 建议的首版结论

首版建议把目标收敛成：

- `upgrade --check`：完整可用
- `upgrade`：支持官方 bundle 安装 + daemon 自动重启
- `upgrade --no-restart`：支持官方 bundle 安装，但只安装不重启

这样实现简单、边界清晰，也和当前 `serve` / `stop` / release bundle 结构最一致。
