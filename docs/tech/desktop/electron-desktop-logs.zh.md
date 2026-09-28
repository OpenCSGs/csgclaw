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

也可以把 `collect-desktop-diagnostics.cmd` 和 `collect-desktop-diagnostics.ps1` 放在同一个目录发给出现问题的用户，在该目录的终端运行 `collect-desktop-diagnostics.cmd`，无需克隆源码仓库。

脚本将所有信息放进桌面上的**一个 ZIP**，不会为智能体或崩溃文件另外生成压缩包：

```text
csgclaw-diagnostics.zip
```

每次成功运行更新同名 ZIP，先完成压缩再替换旧文件。旧版脚本留下的带时间戳 ZIP 不会自动删除；本次只需发送 `csgclaw-diagnostics.zip`。

主要收集内容：

| 包内文件 | 内容 |
| --- | --- |
| 桌面与安装日志 | 最新 `main.log`、`main.previous.log`、`backend.log`、渠道安装日志与 ready 标记、Squirrel 日志；每个文本文件默认保留最后 10000 行并进行常见凭据脱敏 |
| `agent-data/agents.json` | 本地 API 的智能体快照，包括运行状态、配置完成状态和启动状态；API 不可用时记录原因 |
| `agent-data/persisted-agents.json` | 本地 `state.json` 中的智能体部分，经脱敏后保留配置与期望状态；不复制整个状态文件 |
| `agent-data/<id>/` | Codex 和 DSH 的运行元数据、目录清单及 stderr 尾部，默认各 500 行；应用未运行时也从本地文件收集 |
| `process-status.txt`、`agent-data/process-details.txt` | 相关进程的资源占用、文件版本、PID、父 PID、启动时间和脱敏命令行；包含 DSH 的 CMD/Node 子进程 |
| `agent-data/network-connections.txt` | 相关进程的 TCP 连接 |
| `windows-system-info.txt`、`diagnostics-info.txt` | Windows 版本、构建号、内存、系统启动时间、PowerShell 版本、位数、时区与采集时间 |
| `windows-events.txt` | 最近 180 分钟内与 CSGClaw、DSH、Node、Codex 相关的 Windows Application 事件 |
| `windows-power-events.txt`、`windows-defender-events.txt` | 同一时间范围内的系统休眠唤醒事件和与运行时相关的 Defender 事件；无权限时记录原因 |
| `crash-*` | Crashpad dump 和元数据，仍放在同一个 ZIP 中；二进制 dump 无法脱敏 |
| `collection-errors.txt`、`collected-paths.txt`、`README.txt` | 不可用的数据项、来源路径和包内说明 |

某个文件读不到、桌面日志目录不存在或应用已经退出时，脚本仍尽量生成包含其余信息的 ZIP。排查“智能体自动离线”时，建议出现问题后立即运行，先保留现场再重启应用。

需要修改 Windows 事件回溯时间或 userData 位置时，可以直接调用 PowerShell 脚本：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass `
  -File .\scripts\collect-desktop-diagnostics.ps1 `
  -EventLookbackMinutes 60 `
  -UserDataDirectory "$env:APPDATA\CSGClaw"
```

也可以通过 CMD 入口传参，例如 `.\scripts\collect-desktop-diagnostics.cmd -AgentLogLines 2000 -DesktopLogLines 20000`。

### 重新安装 Windows 受管 DSH

CSGClaw 默认安装 `@deepseek-ai/dsh@0.1.5-rc.3`。更新 CSGClaw 不会自动升级已有 DSH；若要清理旧安装后重新安装，先退出 CSGClaw 及其 `serve` 服务，再在 PowerShell 中执行：

```powershell
$dshPaths = @(
  "$env:USERPROFILE\.local\bin\dsh.cmd",
  "$env:USERPROFILE\.local\share\deepseek-harness"
)
foreach ($dshPath in $dshPaths) {
  if (Test-Path -LiteralPath $dshPath) {
    Remove-Item -LiteralPath $dshPath -Recurse -Force -ErrorAction Stop
  }
}
```

以上路径为默认受管安装位置。智能体配置和会话保存在 `.csgclaw` 中，清理命令不会删除这些数据。

使用已经包含版本升级改动的 CSGClaw 可执行文件重新启动，在“电脑 → Agent Runtime”中安装 DSH。源码用户需要先重新编译 CSGClaw；仅拉取代码而继续运行旧的可执行文件，仍会安装旧版本。

安装器会向 registry 重新校验包元数据，避免旧的 npm 版本列表缓存导致 `ETARGET`（找不到目标版本），无需提前清空全局 npm 缓存。安装后确认版本：

```powershell
& "$env:USERPROFILE\.local\bin\dsh.cmd" --version
```

应显示 `0.1.5-rc.3`。随后启动 DSH 智能体并确认能完成一次对话。

### 手工查看日志

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
