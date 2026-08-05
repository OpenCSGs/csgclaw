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

查找并查看最新 `backend.log` 的最后 300 行：

```powershell
$backendLog = Get-ChildItem $root -Recurse -File -Filter 'backend.log' -ErrorAction SilentlyContinue |
  Sort-Object LastWriteTime -Descending |
  Select-Object -First 1
if (-not $backendLog) { throw '未找到 CSGClaw backend.log' }
Get-Content -LiteralPath $backendLog.FullName -Tail 300
```

把主进程日志复制到桌面，便于发送：

```powershell
Copy-Item -LiteralPath $mainLog.FullName `
  -Destination (Join-Path $env:USERPROFILE 'Desktop\csgclaw-main.log') `
  -Force
```

也可以用下面的一条命令完成查找和导出：

```powershell
$log = Get-ChildItem "$env:APPDATA\CSGClaw" -Recurse -File -Filter 'main.log' -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1; if (-not $log) { throw '未找到 CSGClaw main.log' }; Copy-Item $log.FullName "$env:USERPROFILE\Desktop\csgclaw-main.log" -Force; Write-Host '日志已导出到桌面: csgclaw-main.log'
```

Crashpad 通常位于 `%APPDATA%\CSGClaw\Crashpad`。列出 dump：

```powershell
Get-ChildItem (Join-Path $root 'Crashpad') -Recurse -File -ErrorAction SilentlyContinue
```

## 首次启动闪退的采集步骤

1. 确认旧版 CSGClaw 已完全退出。
2. 安装或打开待验证的新包，只启动一次。
3. 如果窗口闪退，先不要立即第二次启动。
4. 导出 `main.log`；如果 Crashpad 中存在 `.dmp`，同时导出对应 dump。
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
