# Agent Engine 与 RuntimeExtension

English: [agent-engine-decoupling.md](agent-engine-decoupling.md)

## 已实现的架构

最终重构已将原 Service-backed facade 拆成资源控制器、Repository、Lifecycle Coordinator 和 Runtime Adapter Registry。
Phase 1（Session）、Phase 2（公共契约与 MemoryClient）、Phase 3（共享生产 Engine）、Phase 4（内部所有权拆分）和 RuntimeExtension 已实现。
本文描述实际接口，不再增加过渡方案。
平台拓扑与 Runtime 能力矩阵见[整体架构](architecture.zh.md)。

```text
API / Channel / Session
  -> agentengine.Interface
       Agents()
       Conversations(agentID)
       RuntimeExtensions(agentID)
  -> 共享 lifecycle.Coordinator
  -> 已注册 Runtime Adapter
```

公共类型定义在 `internal/agentengine/contract`，由 `internal/agentengine` 导出。
这是为避免 Engine 与具体 Adapter 的循环依赖而进行的契约拆包，不是兼容 Facade。
Engine 不导入具体 Runtime、IM、Participant 或 Channel。

## 所有者

| 所有者 | 职责 |
| --- | --- |
| `agents.Repository` | 在现有本地存储中保存 Agent 期望状态、Runtime 观察状态和 Extension 记录 |
| `agents.Controller` | Agent 资源、field mask、基础 Provision 和生命周期编排 |
| `Engine` Conversation coordinator | Turn admission、幂等重放、事件排序、Cancel、Reset、Resolve 和不可变文件句柄 |
| `interactionstate.Coordinator` | 原生与 detached interaction 的 pending/终态、校验、过期和唯一回复 |
| `lifecycle.Coordinator` | 同 Agent execution lease 与独占 mutation lease |
| `registry.Registry` | Runtime 注册、选择与有序关闭 |
| `runtimeExtensionManager` | Extension 资源、Source 注册/解析、generation 和恢复 |
| Runtime Adapter | 布局、Provision、进程生命周期、probe、原生会话和 Extension Driver |
| `WorkspaceService` / `ModelConfiguration` | Workspace/日志/模板支持，以及模型配置窄接口 |

Controller 自己实现资源操作，不再委托给已删除的 `internal/agent.Service`。
旧 Agent facade、Service adapter、Codex bridge 和 Channel 生命周期回调已移除。
HTTP 构造函数显式接收 Engine。
Runtime Registry 在装配后封闭，未注册能力返回错误。

## Agent 资源

`Agents()` 暴露 Create、Get、List、Update、Delete、Recreate。
Start/Stop 使用 `desired_state` field mask。
Profile/model、Skills、MCP、instructions 和 memory 的 HTTP 操作都翻译成 field-mask 更新。
省略 field mask 表示完整替换期望 Spec，只有 ID 的重建请求必须使用显式 mask 保留配置。
省略的 write-only credentials 和模型密钥继续保留，返回值不暴露凭据内容。

`RuntimeSpec.Credentials` 与 `InitShell` 仅属于完整基础 Provision。
增量 Extension reload 不重新执行它们。
Workspace 浏览、日志、模板发布和模型配置由专门的支持接口提供。
普通 Agent Get 读取已保存资源，只有 `ProbeRuntime` 显式要求实时探测。

## Conversations

`Conversations(agentID)` 暴露 Run、Cancel、Reset、Resolve、GetInteraction 和 Files。
ConversationKey 由调用者构造，对 Engine 不透明。
原生会话映射属于 Adapter，不是 IM transcript 或 Session Binding Store。

Run 校验请求，按 Agent/key 进行唯一活跃 Turn admission，取得 execution lease，再选择已注册的 ConversationProvider。
缺少实现时返回 `runtime_adapter_unavailable`，没有 fallback。
拒绝、等待、supersede 是不同的 admission 策略。
重复 Turn ID 必须对应相同请求，并重放相同结果而不是再次执行。
Engine 在进程内最多保留 1024 个已分发完成 Turn。

Cancel 定位一个 Turn。
Reset 关闭该 Conversation 的新 admission，取消并等待活跃执行，再重置原生映射。
两者都不删除 IM transcript。
只有成功 Turn 的 Runtime 输出文件会成为不可变、Agent-scoped Engine FileID，失败或取消的文件不投递。

## 交互解析

生产交互只有一条路径：

```text
HTTP / Channel action
  -> Channel 可信路由
  -> Conversations().GetInteraction / Resolve
  -> interactionstate.Coordinator
  -> 已选 Runtime Adapter
  -> 原生 permission / user-input request
```

Engine 原子认领 pending interaction 并校验回复。
transcript 回调在原生请求释放前完成。
回调失败保留 pending 状态，重复回复不能重复写入 transcript。
显式失效也会取消进行中的回复回调，阻止其随后完成 detached 答案。
Turn 正常完成与取消分开处理，因为原生回复可能先解除执行阻塞、随后才返回。
公共快照对 secret 答案脱敏。

成功命令的结构化输出可以在 Turn 结束后激活 detached question。
它属于 Engine，而不属于 Codex 原生请求 broker。
回复、过期、取消、重置、新 Turn 和生命周期事件更新原 UI 投影。
Channel 在答案投影落地后，通过正常 ingress 提交仍有效的 detached continuation，不直接调用 Runtime。
旧 Codex detached、Channel Bind 和 transcript 接口已删除。

## 共享生命周期

Run 持有 execution lease，直到 Runtime 清理结束。
Agent Update/Delete/Recreate 与 Extension Apply/Delete 取得 mutation lease。
Mutation 关闭新的 execution admission、drain 活跃 lease，并串行修改该 Agent。
等待和 drain 支持 context 取消，默认 drain 上限为两分钟。
Mutation context 可重入，业务事实更新和嵌套 Extension Apply 使用同一个 lease。
不同 Agent 互不阻塞。

Channel Binding Manager 独立拥有 Worker 生命周期。
Runtime Stop/Recreate/reload 不删除 Binding、transcript，也不停止 Channel Worker。
业务凭据变化显式触发 Channel Binding 对账。
OpenClaw/PicoClaw 保留原生 gateway Channel 和命令协议。
其绑定配置变化通过既有 Runtime Adapter 执行 Engine 管理的 Recreate；托管 Codex Worker 则继续独立于 Runtime 生命周期。

## RuntimeExtension 契约

```go
RuntimeExtensions(agentID).Apply(ctx, RuntimeExtensionApplyRequest)
RuntimeExtensions(agentID).Get(ctx, name)
RuntimeExtensions(agentID).List(ctx)
RuntimeExtensions(agentID).Delete(ctx, name)
```

```go
type RuntimeExtensionSpec struct {
    Name          string
    Kind          string
    Source        RuntimeExtensionSourceRef
    FailurePolicy RuntimeExtensionFailurePolicy
}
type RuntimeExtensionSourceRef struct {
    Provider string
    Ref      string
}
```

Name 在 Agent 内唯一。
Spec 引用业务事实，不包含解析后的 secret、宿主路径、文件内容或命令。
Apply 独立更新子资源，不放进 AgentSpec。
更换 Runtime 时保留 Extension 期望资源，新 Adapter 不支持配置或启动失败时也不会删除资源。
可选 resource version 拒绝旧写，每次接受的 Apply 推进 generation。
相同 source revision 可以复用有效投影，不重复 bind，也不重启已加载配置的进程。

状态包含 `configured / unavailable / error`、generation、observed_generation、source_revision、reason、message、checked_at、applied_at、runtime_loaded。
Unavailable 表示缺少 executable 或可用性 probe 失败，不作为内部错误。
未注册 Driver 返回 `extension_unsupported`。
当前 Adapter 没有 Extension 投影时，仍允许删除期望资源。
删除意图不阻止 ready，清理 reload 的成功也不依赖其他 Extension 是否 ready。
Get/List 只读已保存状态，不执行命令，也不隐式 Apply。

## Source 与 Driver

集成层注册 Source，将 `(agentID, ref)` 解析为 source revision 和带版本的 opaque payload。
Resolved payload 只在本次对账内存中存在，Engine 不保存、不记录日志。
Source 不拥有 Runtime 布局或重启策略。

Runtime 的 ExtensionDriverProvider 按 kind 选择 Driver。
Driver 校验、probe，并准备可回滚的托管投影。
PreparedExtension 提供 Projection、Activate、Rollback、Cleanup。
Runtime 的 ExtensionHost 读取投影、渲染 Engine 排序后的 instructions，并在 executable 不可用时仍支持删除。

每个投影使用私有 `<agent>/<extension-name>/generation-*` root。
初始化在 staging 内运行，使用最终不变的路径。
原子 active manifest 选择当前 generation。
Driver 不覆盖用户文件或其他 Extension。
环境变量存在冲突时失败，只有值完全相同才允许共享，也会检查 Agent 显式配置的 model environment。
instructions fragment 按 Extension name 排序后由 Runtime renderer 合并，保留 managed block 外的用户内容。

## Apply、恢复与删除

1. 在 Agent mutation lease 内校验并保存新 desired generation。
2. 解析当前 Source 事实，选择已注册 Driver。
3. Probe、校验、staging。
4. 校验完整的环境和 instructions contribution。
5. 激活并渲染，失败时在安全范围内回滚托管投影。
6. 只有已经运行且尚未加载有效投影的 Runtime 才 reload。
7. 更新 observed generation、状态和时间戳，并清理过期 staging。

Apply/Delete 不启动已停止的 Runtime。
Codex 的 loaded 状态比较 live process 启动时记录的 projection digest，不通过检查文件存在来推断。
已配置但尚未加载的工具会显示等待加载或 reload warning。

同 source revision 的失败重试可以保留上一 active generation。
Source revision 一旦变化，失败就停用旧投影，不能继续激活旧凭据。
Source 无法解析时也会停用已有投影。
回滚覆盖托管文件和 Runtime contribution，不承诺撤销初始化命令的外部系统副作用。

Recreate 在 Runtime 启动前按 name 顺序恢复期望 Extension。
Optional 失败只降低 Extension 状态，不阻止正常启动。
Block_runtime Extension 必须已配置且已加载，Agent 才 ready。
Delete 先记录删除意图，移除 Runtime 投影后才删除 desired resource。
清理失败保留 `delete_failed`，可手动重试或在启动恢复时继续删除，不会重新 Apply 被删除的工具。
若无法安全移除运行中 Runtime 的投影，则停止 Runtime 并保留清理记录。

## 飞书集成

固定绑定为 `feishu-lark-cli / lark-cli / feishu-participant / <participant-id>`，策略为 optional。
注册连接和手动 Participant 写入都将凭据更新与 Apply 串行化。
AppID 独占仍由 `feishubind` 负责。
缺少 lark-cli 不阻止连接飞书，UI 提供安装和重试指引。
断开时先删除 Participant 使 source token 失效，再删除 Extension。
部分清理失败提供独立重试入口，并拒绝清理新连接的 Bot。

Source token 绑定 Agent、Participant、purpose 和 credential revision，no-auth 模式也要求有效 token。
App Secret 只保存在 Participant Store。
Codex Driver 拥有 bind、私有配置、保留环境变量和 instructions。
CSGClaw 不安装或升级 lark-cli。
不存在接收任意 payload/命令的通用 HTTP Apply API。

## 验证与限制

真实 Engine 和 MemoryClient 运行同一套资源与 detached interaction 契约。
针对性测试覆盖投影隔离、冲突、回滚、source 失效、停止状态保持、单次 reload、delete retry 和 HTTP token 撤销。
`architecture_test.go` 防止 Engine 导入具体 Runtime/Channel，以及 API/Channel/CLI 调用原生 broker。

Runtime 能力边界以整体架构中的矩阵为准。
Pending interaction、Turn 重放状态和 FileID 都是进程内资源。
系统不提供与飞书的跨进程事务，也不保证撤销命令对外部系统的副作用。
