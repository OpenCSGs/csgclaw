# CSGHub Sandbox 中 CSGClaw 的界面原地升级方案

## 状态

提案。本文定义 CSGHub `csgclaw-server-sandbox` 中由 Supervisor 托管的
CSGClaw 如何支持与个人电脑安装相同的“在 UI 中检查、下载、安装并重启”体验。
实现横跨 `csgclaw`、`sandbox-runtime` 和 CSGBot；本文不改变本地 BoxLite
或普通官方 bundle 的升级语义。

## 1. 目标与边界

用户在 CSGClaw Web UI 点击升级后，应能完成：

1. 检查并展示最新 CSGClaw release；
2. 下载与当前 Linux 架构匹配的官方 bundle；
3. 原子替换该 sandbox 自己的 CSGClaw bundle；
4. 由 Supervisor 拉起新 binary；
5. 浏览器短暂断开后重新连接，并明确显示成功或失败；
6. sandbox pod 重启后仍运行已升级的版本。

不在本方案范围内：

- 用 UI 更新 `csgclaw-server-sandbox` wrapper 镜像、Python sandbox、Codex
  CLI 或 agent sandbox 镜像；这些仍由镜像发布流程管理。
- 在同一 sandbox 之间共享升级结果；每个 CSGHub sandbox 的版本和 bundle
  独立。
- 为任意进程管理器执行用户给出的 shell command。升级代码不得接受任意
  `restart_command` 或可注入的命令字符串。

## 2. 当前实现与不能直接复用的原因

当前 server wrapper 的实际路径如下：

```text
/usr/local/bin/csgclaw
  -> /opt/csgclaw/bin/csgclaw

Supervisor
  -> /usr/local/bin/csgclaw --config /home/picoclaw/.csgclaw/config.toml serve
```

Dockerfile 通过 marker 把 `/opt/csgclaw` 伪装成官方 bundle，以满足 release
build 的启动校验；Supervisor 则以前台 `serve` 管理该进程。

这使得现有 `csgclaw upgrade` 不能安全工作：

- 自动安装会在 bundle 的父目录创建同级 staging 与 backup。运行用户是 UID
  1000，而 `/opt` 不是该用户可写目录，安装会在创建 staging 时失败。
- 当前重启逻辑只读取 `~/.csgclaw/server.pid` 并执行 `stop` / `serve --daemon`。
  Supervisor 的前台服务没有该 PID 文件，因此会被视为“未运行，需要手动
  重启”。
- 容器根文件系统是临时的。即使放宽 `/opt` 权限并在运行中替换成功，pod
  重建后仍会回到镜像内的旧 binary。
- server wrapper 当前 `show_upgrade = false`。该设置应继续是默认值，直到
  本文定义的持久 bundle 与受控重启都已实现。

因此不能只把 `show_upgrade` 改为 `true`，也不应通过 `chmod /opt` 让当前
bundle 可写。

## 3. 目标架构

每个 server sandbox 已有一个独立 PVC subpath，挂载到
`/home/picoclaw/.csgclaw`。该卷同时承载配置、Hub 内容和 upgrade 状态；新增
bundle 目录后，布局为：

```text
镜像（只读）
  /opt/csgclaw-bundle-seed/csgclaw/
    .csgclaw-bundle.json
    bin/csgclaw
    bin/csgclaw_dir/csgclaw-cli

每个 sandbox 的 PVC（可写、持久）
  /home/picoclaw/.csgclaw/
    config.toml
    hub/
    bundle/
      csgclaw/                         # 当前运行 bundle
      csgclaw.backup.<timestamp>/      # upgrade 期间保留
    logs/
      upgrade-helper.log
      upgrade-helper-status.json

固定 launcher
  /usr/local/bin/csgclaw
    -> /home/picoclaw/.csgclaw/bundle/csgclaw/bin/csgclaw
  /usr/local/bin/csgclaw-cli
    -> /home/picoclaw/.csgclaw/bundle/csgclaw/bin/csgclaw_dir/csgclaw-cli
```

`/usr/local/bin` 中的 symlink 可以在镜像构建时创建，即使 PVC target 尚未
存在。entrypoint 在启动 Supervisor 前保证 target 已存在且通过 bundle 结构
校验。

```text
UI POST /api/v1/upgrade/apply
        |
        v
旧 csgclaw server --spawn--> upgrade helper
        |                         |
        |                    下载、校验、替换 PVC bundle
        |                         |
        |<----- SIGTERM ---------+  （只在成功安装后）
        v
Supervisor 发现前台 server 退出
        |
        v
同一路径重新 exec，symlink 已指向 PVC 中的新 bundle
        |
        v
新 server 健康检查成功，UI 重新连接并显示结果
```

## 4. `sandbox-runtime` 修改

### 4.1 Dockerfile：只读 seed，不再以 `/opt/csgclaw` 作为运行安装根

修改 `csgclaw-server/Dockerfile`：

1. 将 base image 中的 `csgclaw` 和 `csgclaw-cli` 移到
   `/opt/csgclaw-bundle-seed/csgclaw/`，并写入正式 marker：
   `{"app":"csgclaw","layout":"official-bundle"}`。
2. 创建 `/usr/local/bin/csgclaw` 与 `/usr/local/bin/csgclaw-cli` 到 PVC
   bundle target 的 symlink，不再指向 `/opt/csgclaw`。
3. 保留 seed 为 root 所有、只读；UID 1000 只需要写 PVC 中的
   `bundle/`，不需要写 `/opt`。
4. 删除或改正所有将 `/opt/csgclaw` 称为实际运行 bundle 的注释；若兼容
   旧脚本仍需要该路径，使用只读 symlink 指向 seed，不能指向可变 bundle。

这样 `internal/upgrade.installBundle` 使用的同级 staging、backup 和 rename
均发生在可写的：

```text
/home/picoclaw/.csgclaw/bundle/
```

而不是扩大 `/opt` 的写权限。后者会使同一容器中的 UID 1000 可以替换
`/opt/codex` 等无关路径。

### 4.2 entrypoint：仅首次 seed，绝不覆盖已升级版本

修改 `csgclaw-server/docker-entrypoint.sh`，在复制 `config.toml` 和 Hub
目录后、启动 Supervisor 前执行：

1. 设定 `BUNDLE_PARENT="$CFG_DIR/bundle"`、
   `BUNDLE_ROOT="$BUNDLE_PARENT/csgclaw"`。
2. 若 `BUNDLE_ROOT` 不存在，复制 seed 到同一父目录的临时目录，校验 marker
   和 `bin/csgclaw`，再 rename 为 `BUNDLE_ROOT`。
3. 若目录存在，验证 marker 和 executable；验证失败则启动失败，不自动用新
   seed 覆盖。自动覆盖会掩盖损坏并丢失用户已升级的版本。
4. 以 UID/GID 1000 确保 `bundle/` 可写；失败时在 Supervisor 启动前明确报错。

当前 entrypoint 在每次启动时重写 `config.toml` 与 `hub/`，但不会删除
`bundle/`。新目录与已有的 PVC 生命周期兼容，不需要改变 CSGBot 的 volume
mount 路径。

### 4.3 Supervisor：显式声明受托管升级模式

在 `[program:csgclaw-server]` 的 environment 中加入：

```ini
CSGCLAW_UPGRADE_RESTART_MODE="supervisor-parent"
```

该值仅供 CSGClaw server 及它启动的 upgrade helper 继承；不作为容器级别的
自由命令配置。现有 `autorestart=true`、`stopasgroup=true` 和
`killasgroup=true` 保持不变。

不要让 helper 直接运行 `supervisorctl restart csgclaw-server`。Supervisor
按进程组停止服务时可能同时终止作为 server 子进程的 helper，造成状态文件和
回滚流程无法收尾。

### 4.4 UI 开关

在第 5 节的 CSGClaw 能力、完整性校验与端到端测试全部落地前，保持：

```toml
[server]
show_upgrade = false
```

落地后将 server wrapper 的配置改为 `show_upgrade = true`。这只决定 UI
可见性；API 端仍必须独立做能力与授权校验。

## 5. `csgclaw` 修改

### 5.1 受控重启策略

在 `internal/upgrade` 增加显式的 restart strategy，而不是让 CLI 猜测
Supervisor 或执行任意 shell：

```go
type RestartMode string

const (
    RestartModeDaemon           RestartMode = "daemon"
    RestartModeSupervisorParent RestartMode = "supervisor-parent"
)
```

默认仍是 `daemon`，完全保持个人电脑现有语义。只有 API helper 以
`supervisor-parent` 模式启动时，才允许以下流程：

1. `StartApplyHelper` 读取当前 server 的 PID（`os.Getpid()`），并将它作为
   `CSGCLAW_UPGRADE_RESTART_PARENT_PID` 传给 helper；同时传递经过验证的 mode。
2. helper 完成下载、校验、原子替换和 runtime asset 同步后，才向该 PID 发送
   `SIGTERM`。
3. 前台 server 已注册 `SIGTERM`，会正常退出；Supervisor 的
   `autorestart=true` 再以固定 launcher 路径启动新版本。

`--no-restart` 必须跳过这一步。直接在容器 shell 中执行 `csgclaw upgrade`
时没有由 API 传入的 parent PID，也必须退回“安装完成，需要人工重启”，而不能
向任意父进程发信号。

实现文件建议：

- `internal/upgrade/helper.go`：传递受控 mode 与 parent PID；
- `internal/upgrade/restart.go`：抽取 restart strategy；
- `internal/upgrade/restart_supervisor_unix.go`：仅 Unix 的 parent PID
  校验与 `SIGTERM`；Windows 不编译该实现；
- `cli/upgrade/upgrade.go`：在安装成功后调用 strategy，并分别呈现
  `restarted`、`restart_requested` 和 `manual_restart_required`；
- `internal/api/handler.go`：只允许被授权且具备该策略的 apply 请求。

`RestartResult` 应新增 `RestartRequested`，不能把“已向 Supervisor 请求
重启”错误显示为“新服务已确认重启”。

### 5.2 持久升级事务与回滚

当前 `installBundle` 成功 rename 后会立刻删除 backup。对可人工重新部署的
PC 安装可接受，但 sandbox UI 升级需要能处理新 binary 无法启动的情况。

本方案要求将安装拆为两个阶段：

1. **prepare/activate**：复制到 staging，将旧 bundle rename 为 backup，
   将新 bundle rename 为 current；将持久事务记录写到 PVC。
2. **finalize**：新 server 通过本地 `/healthz` 后删除 backup 和事务记录。

helper 在向旧 server 发 `SIGTERM` 后保持运行，使用 localhost health check
等待新版服务。成功时写入 `completed`；超时或启动失败时写入可读错误及 backup
路径。失败的自动回滚需要作为同一实现的一部分：恢复 backup 后请求 Supervisor
再次启动，并在最终状态中标记 `rolled_back`。若无法证明回滚已成功，保留 backup
并将状态标为 `manual_recovery_required`，绝不能静默删除。

状态文件应放在现有的 `~/.csgclaw/logs/`，并至少包含：

```json
{
  "status": "restarting|completed|failed|rolled_back|manual_recovery_required",
  "from_version": "vX.Y.Z",
  "to_version": "vA.B.C",
  "backup_path": "...",
  "message": "...",
  "updated_at": "..."
}
```

新 server 启动后消费该状态，将结果送回 `UpgradeManager` 与
`upgrade.status_changed` SSE 事件。浏览器应把服务断开视为预期，并轮询
`/healthz` / `/api/v1/upgrade/status` 直到完成或超时。

### 5.3 API 权限与能力校验

`POST /api/v1/upgrade/apply` 不能只依赖 Web UI 是否显示。请求必须同时满足：

1. `show_upgrade` 已启用；
2. 当前运行的是已验证 marker 的持久 PVC bundle；
3. restart mode 为受支持的 `supervisor-parent` 或既有 `daemon`；
4. 外层 CSGHub gateway 已完成租户授权，且 CSGClaw access-token 语义没有
   被 `no_auth` 绕过；
5. 当前没有正在执行的 upgrade transaction。

不满足时返回明确的 `409 Conflict` / `503 Service Unavailable`，并在 UI 中
显示“当前部署不支持原地升级”，而不是启动一个必定失败的 helper。

### 5.4 发布物完整性是上线前置条件

当前下载逻辑只校验发布元数据中的文件大小，SHA-256 校验仍处于临时禁用状态。
在将升级按钮暴露给多租户 sandbox 前，必须：

1. 让 release 元数据为每个 bundle 发布 SHA-256；
2. 恢复 `PrepareRelease` 对缺失 hash 的拒绝；
3. 恢复流式下载中的 SHA-256 计算与比较；
4. 为 hash 缺失、hash 不匹配、size 不匹配和恶意 archive 分别添加测试。

如果发布链路允许，后续应改用签名 manifest；但 SHA-256 元数据校验是本方案
最小可接受的上线条件。

## 6. CSGBot 影响

CSGBot 目前已经把每个 CSGClaw server sandbox 的独立 PVC subpath 挂载到
`/home/picoclaw/.csgclaw`。因此本方案不需要为自升级新增卷或修改 volume
subpath 算法。

需要确认并补测试的契约是：

- 同一 sandbox 的 stop/start 或 pod 重建复用同一 PVC subpath；
- 真正的 sandbox recreate 使用新 subpath，因此会从 wrapper image 的 seed
  开始；
- CSGClaw server wrapper 新版本不会覆盖已存在的 `bundle/csgclaw`；
- CSGBot 通过 `[sandbox.csgclaw-server].image` 切换 wrapper 镜像的既有发布
  流程保持不变。

这意味着首次启用该能力时，已有 sandbox 需要重建一次以获得新的 wrapper
entrypoint 和 Supervisor environment；之后 UI 升级无需重新发布 wrapper。

## 7. 分阶段实施

### Phase 0：发布链路准备

- 发布带稳定 SHA-256 元数据的 CSGClaw release；
- 恢复并验证 CSGClaw 下载完整性校验；
- 定义可用于测试的 release metadata endpoint 注入方式，生产默认仍固定到
  官方 HTTPS endpoint。

### Phase 1：持久 bundle bootstrap

- 修改 `sandbox-runtime/csgclaw-server/Dockerfile` 与 entrypoint；
- 在新 wrapper image 中验证首次 seed、pod 重启保留、损坏 bundle 拒绝启动；
- 保持 `show_upgrade = false`，此阶段不暴露 UI 操作。

### Phase 2：Supervisor restart strategy

- 实现受控 parent-PID restart mode 与状态机；
- 完成普通 daemon 行为的回归测试，确保 PC 升级不受影响；
- 实现 upgrade 后健康确认、backup 保留与自动/人工恢复状态；
- 在集成镜像中验证旧 PID 退出、新 PID 启动、PVC bundle 版本变化。

### Phase 3：API/UI 开放

- API 对 `show_upgrade`、能力和互斥事务做服务端校验；
- 启用 wrapper 的 `show_upgrade = true`；
- UI 显示“下载中、重启中、已完成、已回滚、需要人工恢复”并处理短暂断线；
- 灰度至 staging sandbox，验证后再启用 production image。

## 8. 验证清单

### CSGClaw 单元与集成测试

- 普通官方 bundle 的 `upgrade`、`--check`、`--no-restart` 结果不变。
- `supervisor-parent` mode 只有 API helper 能构造；缺 mode、缺 PID、PID 非法或
  `--no-restart` 都不发送信号。
- 安装、asset 同步完成前不会发终止信号。
- 新增 transaction 状态可被新 server 恢复；失败、回滚和人工恢复状态都能通过
  API/SSE 读取。
- SHA-256 缺失、不匹配及 archive 路径穿越都不会替换 current bundle。

### sandbox-runtime 容器测试

- 使用空 PVC 启动时从 seed 创建 bundle，且 CSGClaw 可以启动。
- 在 PVC 内安装测试 release 后，Supervisor 拉起新 PID，`--version` 与
  `/api/v1/version` 均为目标版本。
- 重启 container/pod 并复用 PVC，版本仍为目标版本。
- 用包含不同 seed 的新 wrapper 重启同一 PVC，不覆盖用户已升级的 bundle。
- 新 binary 无法健康启动时，验证 backup、rollback 和 `manual_recovery_required`
  的最终状态。
- UID 1000 不需要、也不能通过升级流程写入 `/opt/csgclaw-bundle-seed` 或
  `/opt/codex`。

### CSGBot 端到端测试

- 创建 CSGClaw sandbox 后确认其 PVC 挂载包含 `bundle/`。
- 对同一 sandbox 调用 UI upgrade，验证 CSGHub gateway 在短暂重启后恢复。
- sandbox recreate 后确认使用新 wrapper seed，而不是前一个 sandbox 的 bundle。

## 9. 上线与回退

上线前先发布带该能力、但保持 `show_upgrade = false` 的 wrapper image。仅在
staging 上完成 Phase 1–2 验证后再打开 UI。

出现问题时可立即把 wrapper 的 `show_upgrade` 关闭；这会阻止新 transaction，
不修改已持久化 bundle。对于单个失败 sandbox，保留的 transaction record 与
backup 是恢复依据。平台级回退仍可通过 CSGBot 切换回旧
`csgclaw-server-sandbox` image 并重建 sandbox 完成。

## 10. 需要确认的产品决策

1. sandbox 用户是否有权自行选择任意公开 CSGClaw release，还是只能升级到
   平台允许的版本范围？
2. 升级成功后是否应保留 backup 一段固定时间，还是完成 health check 即删除？
3. 新 wrapper image 与 PVC 中已升级 bundle 的版本不一致时，平台是否允许继续
   运行 PVC 版本，还是要求由运维强制重置为 image seed？
4. 升级失败且自动回滚也失败时，平台控制面是否需要提供“重建 sandbox”按钮，
   还是只显示人工恢复说明？

在以上决策确定前，不应把 `show_upgrade` 打开到 production。
