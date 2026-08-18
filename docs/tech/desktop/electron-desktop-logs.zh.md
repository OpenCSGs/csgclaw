# CSGClaw Electron Desktop 日志查看与导出

本文用于排查 CSGClaw Desktop 首次启动闪退、窗口消失、sidecar 启动失败以及 Renderer/GPU 进程异常。

桌面端主要生成以下诊断文件：

| 文件 | 内容 |
| --- | --- |
| `main.log` | Electron 主进程启动、退出、单实例、Squirrel、Renderer/GPU 和异常事件 |
| `main.previous.log` | `main.log` 达到 2 MiB 后轮转保留的上一份日志 |
| `backend.log` | Go sidecar 的 stdout、stderr 和 HTTP 访问日志 |
| `Crashpad` | Electron 原生崩溃产生的本地 dump；不会自动上传 |

`main.log` 使用一行一个 JSON 对象的 JSON Lines 格式。每次启动都会生成新的 `runId`，排查时应查看同一个 `runId` 下的完整事件序列。

## macOS

默认日志目录：

```text
~/Library/Logs/CSGClaw/
├── main.log
├── main.previous.log
└── backend.log
```

Crashpad 默认目录：

```text
~/Library/Application Support/CSGClaw/Crashpad/
```

查看主进程最新 300 行：

```bash
tail -n 300 "$HOME/Library/Logs/CSGClaw/main.log"
```

持续查看主进程事件：

```bash
tail -f "$HOME/Library/Logs/CSGClaw/main.log"
```

查看 Go sidecar 最新 300 行：

```bash
tail -n 300 "$HOME/Library/Logs/CSGClaw/backend.log"
```

列出本地 Crashpad 文件：

```bash
find "$HOME/Library/Application Support/CSGClaw/Crashpad" -type f -print
```

把日志目录打包到桌面：

```bash
ditto -c -k --keepParent \
  "$HOME/Library/Logs/CSGClaw" \
  "$HOME/Desktop/csgclaw-logs.zip"
```

如果 Crashpad 中存在 `.dmp` 文件，可单独打包：

```bash
ditto -c -k --keepParent \
  "$HOME/Library/Application Support/CSGClaw/Crashpad" \
  "$HOME/Desktop/csgclaw-crashpad.zip"
```

## Windows

Windows Website/Squirrel 安装包的日志位于 Electron `userData` 目录下，通常可以从 `%APPDATA%\CSGClaw` 找到。下面的 PowerShell 命令递归查找文件，不依赖日志位于该目录的哪一层。

在仓库根目录运行诊断收集脚本：

```powershell
.\scripts\collect-desktop-diagnostics.cmd
```

脚本会把最新的 `main.log`、`main.previous.log`、`backend.log`、Windows 渠道安装协调器日志和 ready 标记、安装目录下的 `Squirrel-*.log`、Crashpad dump、当前进程状态以及最近 30 分钟的 Windows Application 事件打包到桌面：

```text
csgclaw-diagnostics-YYYYMMDD-HHMMSS.zip
```

需要修改 Windows 事件回溯时间或 userData 位置时，可以直接调用 PowerShell 脚本：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\collect-desktop-diagnostics.ps1 `
  -EventLookbackMinutes 60 `
  -UserDataDirectory "$env:APPDATA\CSGClaw"
```

以下命令用于不生成诊断包时手工查看日志。

查找并查看最新 `main.log` 的最后 300 行：

```powershell
$root = Join-Path $env:APPDATA 'CSGClaw'
$mainLog = Get-ChildItem $root -Recurse -File -Filter 'main.log' -ErrorAction SilentlyContinue |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 1
if (-not $mainLog) { throw '未找到 CSGClaw main.log，请确认已运行包含桌面诊断日志的新版本' }
Get-Content -LiteralPath $mainLog.FullName -Tail 300
```

持续查看主进程事件：

```powershell
Get-Content -LiteralPath $mainLog.FullName -Wait
```

Windows 渠道切换由无控制台的原生 helper 协调。`channel-installer.log` 中正常的阶段顺序应为：

```text
coordinator-started
parent-handle-opened
coordinator-ready
parent-exited
installer-started
installer-exited code="0"
relaunch-started
relaunch-existing-window-detected
relaunch-window-activation foreground="true"
coordinator-finished code="0"
```

安装器未主动启动新应用时，helper 会记录 `relaunch-requested`，等待窗口出现后再记录 `relaunch-window-activation`。如果 Windows 拒绝 helper 抢占前台，`foreground="false" flashed="true"` 表示 helper 已尝试恢复和置前窗口，并通过持续闪烁任务栏提醒用户。

如果日志停在 `coordinator-ready`，检查 `process-status.txt` 中 helper 是否在 60 秒后按超时退出；如果停在 `installer-started`，结合 `Squirrel-*.log` 判断安装器阶段；如果出现 `relaunch-window-not-detected`，说明启动请求已发出，但 15 秒内没有找到目标应用窗口。

查找并查看最新 `backend.log` 的最后 300 行：

```powershell
$backendLog = Get-ChildItem $root -Recurse -File -Filter 'backend.log' -ErrorAction SilentlyContinue |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 1
if (-not $backendLog) { throw '未找到 CSGClaw backend.log' }
Get-Content -LiteralPath $backendLog.FullName -Tail 300
```

Crashpad 通常位于 `%APPDATA%\CSGClaw\Crashpad`。列出 dump：

```powershell
Get-ChildItem (Join-Path $root 'Crashpad') -Recurse -File -ErrorAction SilentlyContinue
```

## 首次启动闪退的采集步骤

1. 确认旧版 CSGClaw 已完全退出。
2. 安装或打开待验证的新包，只启动一次。
3. 如果窗口闪退，先不要立即第二次启动。
4. 在仓库根目录运行 `.\scripts\collect-desktop-diagnostics.cmd` 并保存生成的 ZIP。
5. 第二次启动后仍可导出日志，但分析时必须按不同 `runId` 区分两次启动。

常见事件含义：

| 事件 | 含义 |
| --- | --- |
| `window-opened` | sidecar 和桌面窗口已经成功打开 |
| `squirrel-startup-exit` | Windows Squirrel 安装或更新握手触发的退出 |
| `single-instance-lock-denied` | 已有 CSGClaw 实例持有单实例锁 |
| `startup-failed` / `uncaught-exception` | Electron 主进程 JavaScript 异常，查看 `errorStack` |
| `render-process-gone` / `child-process-gone` | Renderer、GPU 或 Utility 子进程退出，查看 `reason` 和 `exitCode` |
| `before-quit` / `quit` / `process-exit` | 应用进入正常退出路径 |

如果日志停在 `window-opened` 后，没有任何退出事件，并且 Crashpad 产生了 `.dmp`，优先按 Electron 原生崩溃处理。如果日志仍在写、进程仍然存在，则窗口消失不等于应用闪退，还需检查托盘、单实例和窗口隐藏逻辑。

日志会对常见 token、Authorization 和密码字段进行脱敏，但对外发送 `backend.log` 或 dump 前仍应检查是否包含本机路径、账号或其他敏感信息。
