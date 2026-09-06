# 飞书 lark-cli RuntimeExtension

本文描述 2026-09-04 完成的托管 RuntimeExtension 实现。
公共架构见 [Agent Engine](../agent-engine-decoupling.zh.md)，消息执行见[托管飞书 Channel](agent-engine-channel-integration.zh.md)。
lark-cli 配置不再由 API 直接写 Runtime 文件或重启 Codex。

## 边界

```text
连接或手动 Participant 更新
  -> Agent mutation lease
  -> feishubind 校验 AppID 独占并保存 Participant
  -> RuntimeExtensions(agentID).Apply
       -> feishu-participant Source
       -> Codex lark-cli Driver
       -> 托管 generation / environment / instructions
       -> 必要时 reload 已运行的 Codex
  -> Channel Binding 对账
```

本文的 Codex 托管飞书消息只通过 `Conversations` 执行。
Extension 不提供对话执行入口。
缺少 lark-cli 不阻断飞书连接。
CSGClaw 不下载、安装或升级 lark-cli。
OpenClaw/PicoClaw 继续使用既有原生飞书 Channel，不创建 lark-cli Extension。
其连接、机器人切换和断开通过 Engine 的 `Agents().Recreate` 重新渲染并加载原生 gateway 配置，保留原有 Runtime 支持。

## 固定资源

```text
name           = feishu-lark-cli
kind           = lark-cli
source.provider = feishu-participant
source.ref     = <participant-id>
failure_policy = optional
```

公共 Spec 只有业务事实引用。
不能提交文件、路径、密钥或命令作为 HTTP Extension payload。
相同资源的重复 Apply 推进 generation，但可以复用同 revision 的有效投影。

## 所有权

| 所有者 | 负责内容 |
| --- | --- |
| Participant Store | AppID、App Secret、Agent 归属 |
| feishubind | 跨 Agent 的 AppID 独占检查与凭据写入互斥 |
| Feishu ExtensionSource | 确认 Participant 归属，计算 revision，生成 scoped source token 与语义 payload |
| Agent Engine | generation、resource version、状态、同 Agent 串行化、投影事务、reload 和恢复 |
| Codex lark-cli Driver | executable probe、source/config 布局、bind 校验、环境、instructions fragment、loaded/drift 观察 |
| Channel Binding Manager | 飞书 Worker 生命周期，与 Runtime 生命周期独立 |
| API / UI | 固定业务 action、结构化 warning 和 Engine 状态展示 |

Source 实现在 `internal/channel/feishu/larkcli/source.go`。
Driver 实现在 `internal/runtime/codex/lark_cli_extension.go`。
事务和隔离目录由 `internal/agentengine/runtime_adapter.go` 与 `internal/runtime/extensionstate` 实现。

## 托管布局与事务

```text
CODEX_HOME/
  AGENTS.md
  runtime-extensions/
    feishu-lark-cli/
      active.json
      generation-<id>/
        config/
          lark-channel/config.json
        source/
          config.json
```

Source config 和 CLI 配置只写到本 Extension 的私有 generation。
目录权限为 0700，托管文件使用 0600。
Bind 在 staging 中执行，路径在激活前后保持一致。
成功后原子切换 active manifest，再由 Runtime 合并 instructions。
回滚只覆盖 CSGClaw 托管目录和 Runtime contribution，不保证撤销命令对外部系统的操作。

Driver 首先查找 `lark-cli` 并执行有超时的版本 probe。
缺失、权限错误或无法启动返回 `unavailable / executable_unavailable`。
Bind 使用 `lark-cli config bind --source lark-channel --identity bot-only --force --lang zh`。
绑定后检查生成配置中的 AppID。
命令输出不作为公共错误细节返回，避免将配置或凭据带入日志。

## Runtime contribution

Driver 提供：

- `LARKSUITE_CLI_CONFIG_DIR`。
- `LARK_CHANNEL`。
- `LARK_CHANNEL_HOME`。
- `LARK_CHANNEL_PROFILE`。
- `LARK_CHANNEL_CONFIG`。
- lark-cli managed instructions fragment。

Engine 检查环境变量冲突，只有值相同才允许共享。
instructions 按 Extension name 排序，由 Runtime renderer 统一合并。
Driver 不自行覆盖整个 AGENTS.md。
用户 instructions 和其他 Extension 保持独立。

Runtime 已停止时只保存投影。
Runtime 运行中且尚未加载有效投影时只 reload 一次。
增量 reload 不重新执行基础 Credentials 或 InitShell。
同 revision 且有效配置已加载的重试不重复 bind/restart。

Codex 启动时使用所有 active projection 构建进程环境，并记录 projection digest。
`runtime_loaded` 必须匹配 live process 的 digest，不能从文件存在推断。
Recreate 保留托管 root，并在启动前重新解析 Source、Apply 和检查必需 Extension。

## Source token 与机器人切换

Source config 使用 exec provider 调用 CSGClaw 的 AppInfo helper 获取当前凭据。
App Secret 只由 Participant Store 持久化，不写入 Engine Extension resource 或 Source payload。
Source token 绑定 purpose、Agent、Participant 和 credential revision。
Credential revision 随 Participant ID、AppID 或 App Secret 改变。
Source revision 还覆盖实际 source endpoint/helper/token 配置。

AppInfo endpoint 只返回 token 对应的 Participant，并再次检查当前归属和凭据 revision。
全局 API token 不能替代这个 source token。
即使服务启用 no-auth，AppInfo 也不能匿名获取 App Secret。
没有持久 API signing key 的 no-auth 服务使用进程内随机 signing key，并在启动对账中重新生成投影。

切换机器人时，在同一个 Agent mutation lease 内保存新 Participant 凭据并 Apply 同名资源。
旧 source token 随事实变更立即失效。
新 revision 配置失败时，旧环境和 instructions 投影被停用。
只有相同 revision 的失败重试才允许保留上一 active generation。

## 连接、断开与重试

连接流程先保存 Participant，再 Apply Extension，再激活 Binding。
Extension unavailable/error 不阻止连接，响应提供可操作的 warning。
Channel Worker 不因为 Codex reload、Stop 或 Recreate 而被关闭。

断开先删除 Participant，使 source token 无法再取得 App Secret，再 Delete Extension。
清理失败返回 `partial / feishu_cleanup_pending`，并保留 Engine 的 `delete_failed` 状态。
UI 刷新为已断开，并显示“重试清理工具”。
清理重试会检查当前是否已经连接新的 Bot，避免旧清理请求删除新绑定。
若 Runtime 无法安全撤销投影，停止 Runtime 并保留待清理记录。

## HTTP 与 UI

| 接口 | 语义 |
| --- | --- |
| `POST /api/v1/agents/{id}/lark-cli:init` | 仅通过 Engine Apply 固定 Extension |
| `POST /api/v1/agents/{id}/lark-cli:cleanup` | 仅在无已连接 Bot 时通过 Engine Delete 重试清理 |
| `GET /api/v1/agents/{id}/feishu/app-info` | 受 revision-scoped token 保护的 exec-provider 事实读取 |
| Agent 返回值中的 `lark_cli` | 从 Engine 状态派生，不读取 Runtime 文件 |

UI 保留安装指引和手动重试。
已配置但未加载显示等待加载，不误报已经生效。
重载失败显示 warning，配置不被误报为丢失。
API 不返回宿主目录、source 文件内容或 resolved payload。

## 验证

测试覆盖自动配置、缺少 executable、手动重试、机器人切换、AppID 冲突、token 失效、停止状态保持和单次重载。
投影测试覆盖多 Extension 隔离、环境冲突、instructions 排序、staging 回滚、source 变化失效和删除恢复。
HTTP 回归使用真实 Engine 与隔离事实源，验证 no-auth 仍需 source token，以及部分断开的清理重试。
进程内测试不代替真实租户权限验证；飞书远端权限或 tenant policy 仍可能导致实际 CLI 调用失败。
