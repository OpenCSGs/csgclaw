# Agent Engine 解耦

英文版：[agent-engine-decoupling.md](agent-engine-decoupling.md)

## 状态

状态：**架构提案；阶段 2 的 Engine 与 Mock Client 基线已完成；阶段 3 内置 IM Adapter 融合开发中**。

Contract 和阶段 1 的进程内 Conversation 实现位于 [`internal/agentengine`](../../internal/agentengine)。
阶段 1 已把匿名 Session API 接入现有 Codex Runtime，并移除匿名 Session 对 IM Entity 的依赖。
阶段 2 完成 Engine、Memory Client、Session 迁移、Codex Runtime Adapter 行为和基于 Mock 的飞书 Adapter 证明。
阶段 2 不引入或切换生产 Channel Adapter。
阶段 3 负责两侧融合与原子切换，阶段 4 再重构 Engine 内部组件。
该 Package 是精确 Go Type 和 Method Signature 的 Source of Truth。
本文档说明期望的 Owner、行为和增量实现计划。

`internal/channel/csgclaw` 已包含阶段 3 的 Runtime-neutral Core，包括 Binding-scoped Worker、
Conversation Key/Input 转换、Attachment Resolver Boundary 和 Transcript Renderer。
这些组件尚未接入 Composition Root；生产内置 IM Path 仍使用 `internal/channelbridge/codexbridge`。
因此当前实现不代表阶段 3 已完成，也不改变现有 Channel 行为。

## 1. 范围

### 1.1 目标

CSGClaw 需要一条 Runtime 中立的执行路径，供匿名 Session、内置 IM、飞书和未来 Direct Channel 使用：

```text
Channel Adapter 或 Session API -> Agent Engine -> Runtime Adapter
```

设计提供两个顶层公共 Resource Interface：

- `Agents()` 管理持久化 Agent Resource 和 Runtime 生命周期。
- `Conversations(agentID)` 为一个已选择的 Agent 执行 Conversation 并管理不可变文件。

接口采用 Kubernetes Client 风格，先选择 Resource Scope，再暴露聚焦的操作。
它不引入 Kubernetes Controller、API Server、Object Metadata Model 或 Reconciliation Framework。

设计必须：

- 让匿名 Session 独立于 IM Room 和 Message。
- 保留内置 IM 的协作行为。
- 把 Runtime 特有协议隐藏在 `ConversationRuntime` 后面。
- 让每个 Runtime Adapter 物化自己的 Credential，并初始化自己的执行环境。
- 支持 Text、File、实时进度、Interaction 和 CSGClaw Structured Output。
- 复用当前 State Owner，不创建 Engine Database。
- 支持按小步、可评审的阶段实现。

### 1.2 非目标

本方案不：

- 替换现有 Agent、IM、Participant、Team、Task 或 Runtime Store。
- 把 `/api/v1/agents/{id}/llm` 改成 Agent Execution API。
- 实现远程 Agent Engine 或 Engine HTTP 协议。
- 实现完整 OpenAI Responses API 或 `previous_response_id` Chain。
- 增加 Files API 或新的飞书文件下载支持。
- 让 Conversation Execution 拥有 Transcript、Attachment、Runtime Credential File 或 Runtime 原生 Conversation Mapping。
- 统一不同 Runtime Adapter 的 Credential File Format 或 Path。
- 增加兼容、Fallback 或双执行路径。
- 在 OpenClaw 暴露合适的直接协议前声称支持 Direct OpenClaw。

## 2. 当前产品约束

### 2.1 现有状态 Owner

架构保留以下当前 Owner 边界：

| 状态 | Owner |
|---|---|
| Agent、Profile、Runtime Record | `internal/agent` |
| Runtime 原生 Conversation Mapping | 具体 Runtime Package，Codex 当前为 `internal/runtime/codex` |
| User、Room、Message、Thread、Attachment | `internal/im` |
| Participant 和 Channel Binding | `internal/participant` |
| Team、Task、Scheduled Task、Notification、Work | 各自现有 Service |
| 模型传输和 Proxy 认证 | `internal/llm` 和 `internal/cliproxy` |

Agent Engine 不能复制这些持久化状态。
它只能持有进程内 Admission、Active Turn 和 Pending Interaction 状态。

### 2.2 现有执行路径

阶段 1 之前，匿名 Session API 会创建 IM Room 和 Message：

```text
POST /api/v1/agents/{agent}/sessions/{session_id}/responses
  -> 解析 Participant 和 IM User
  -> EnsureAgentSessionRoom
  -> 持久化输入 Message
  -> 通过 Codex Channel Bridge 执行
  -> 持久化最终 Message
```

阶段 1 使用 `Conversations(agentID).Run` 替换该路径，同时保留 Request、SSE 和 Error Shape。
它不再创建匿名 Session IM Entity。

内置 IM 和 Host 侧飞书 Codex 执行当前使用 `internal/channelbridge/codexbridge`。
该 Bridge 已负责 Source Subscription、Deduplication、Conversation Key 构造、隐藏 Channel 和 Thread Context、Attachment Manifest、Activity 渲染、Interaction、Stop 和 `/new`。
执行迁移到 Agent Engine 后，这些 Channel 行为继续由 Channel Adapter 负责。

飞书当前接收 Text、Post 和部分 Interactive Content。
Image、File、Audio 和 Media Input 继续不受支持。

Codex 暴露直接 Session、Prompt、Event、Permission 和 User Input API。
OpenClaw 当前通过自己的 Channel 或 Sandbox Gateway 执行，仓库中没有经过验证且等价的直接 `Run`、Streaming Event、Cancel、Reset 和 Resolve 协议。
因此第一个 Runtime Adapter 是 Codex。

## 3. 目标架构

### 3.1 依赖方向

```mermaid
flowchart TB
    Session["Session HTTP Adapter"] --> Engine["Agent Engine"]
    IM["内置 IM Adapter"] --> Engine
    Feishu["飞书 Adapter"] --> Engine

    Engine --> Registry["Runtime Adapter Registry"]
    Registry --> Codex["Codex Runtime Adapter"]
    Registry --> OpenClaw["未来 OpenClaw Runtime Adapter"]

    Session --> Binding["Session Binding Store"]
    IM --> IMStore["IM 和 Attachment Store"]
    Feishu --> FeishuState["飞书 Binding 和远端 Transcript"]
    Codex --> CodexStore["Codex Conversation Store"]
```

Agent Engine 不 Import IM、Participant、Channel、Team 或具体 Runtime Package。
Composition Root 注册 Runtime Adapter，并把接口连接到现有 Owner。
缺少 Runtime Adapter 时，在创建 Engine Execution State 或 Session Binding 前返回 `runtime_adapter_unavailable`。
它不会启动 Fallback Execution Path。

上面的总览图展示 Dependency Direction 和 State Ownership。
下面两张图把同一组组件展开为控制面和数据面交互，但不会引入第二条 Engine Execution Path。

#### 控制面

控制面修改 Agent 期望状态，并协调 Runtime 生命周期。
阶段 2 中，Engine Agent Facade 把持久化和生命周期操作委托给现有 Agent Service，因此 Engine 不会创建重复的 Agent Store。

```mermaid
sequenceDiagram
    participant Caller as Agent API 或内部调用方
    participant Engine as Agent Engine Agents()
    participant Gate as Agent Lifecycle Gate
    participant Service as Agent Service 和 Store
    participant Runtime as Runtime Adapter

    Caller->>Engine: Create、Update 期望状态、Recreate 或 Delete
    Engine->>Service: 校验完整 Agent 期望状态
    Service->>Gate: 进入生命周期变更
    Gate->>Gate: 关闭执行准入并等待 Active Lease 结束
    alt Drain 完成
        Service->>Runtime: 物化 Credential、执行 InitShell 并变更 Runtime 生命周期
        Runtime-->>Service: 返回观察到的 Runtime 结果
        Service->>Service: 原子提交 Agent 和 Runtime 状态
        Service-->>Engine: 返回更新后的 Agent
        Engine-->>Caller: 返回已隐去 Credential Value 的 Agent
    else 调用方 Context 到期
        Gate-->>Service: Drain 取消
        Service-->>Engine: 返回错误并保持原 Runtime 不变
        Engine-->>Caller: 返回归一化错误
    end
```

所有现有的 Agent Service 直接生命周期调用方都使用同一个 Gate。
这样可以防止 Session 执行、未来 Channel 执行和当前 Agent API 在 Turn 仍持有固定 Execution Lease 时替换或删除 Runtime。

#### 数据面

数据面执行一个归一化 Turn，并返回有序 Event 和唯一 Terminal Result。
Binding、Transcript、Attachment 和 Delivery State 继续由调用它的 Adapter 及其 Store 管理。

```mermaid
sequenceDiagram
    participant Source as Session Client 或 Channel
    participant Adapter as Session 或 Channel Adapter
    participant State as Binding 和 Transcript Owner
    participant Engine as Agent Engine Conversations()
    participant Gate as Agent Lifecycle Gate
    participant Runtime as Runtime Adapter
    participant Native as Runtime-native Conversation

    Source->>Adapter: HTTP Request 或 Inbound Message
    Adapter->>State: 解析 Binding，并按需去重和授权 File
    Adapter->>Engine: Run(TurnID, ConversationKey, Input)
    Engine->>Engine: Fail-fast 准入并注册 Active Turn
    Engine->>Gate: 获取固定 Agent 和 Runtime Lease
    Engine->>Runtime: 执行归一化 Turn
    Runtime->>Native: 创建或恢复 Mapping 并提交 Native Turn
    Native-->>Runtime: Text、Thought、Tool、Interaction 和 Output Event
    Runtime-->>Engine: 返回归一化 Event
    Engine-->>Adapter: 返回有 Sequence 的 EventSink Event
    Adapter->>State: 更新 Adapter 所有的 Transcript 或 Delivery State
    Adapter-->>Source: 返回 SSE 或渲染后的 Channel Delivery
    opt Active Turn 控制
        Adapter->>Engine: Cancel 精确 Turn 或 Resolve Interaction
        Engine->>Runtime: 取消 Native Turn 或回答 Runtime Broker
        Note over Engine,Runtime: Cancel 仅在 Terminal Cleanup 和状态释放后返回
    end
    Native-->>Runtime: Terminal Completion
    Runtime-->>Engine: 在 Runtime 真正结束后返回 Terminal Result
    Engine->>Gate: 释放 Execution Lease 和 Active Turn
    Engine-->>Adapter: 返回唯一 TurnResult
    opt 非活跃 Conversation Reset
        Adapter->>Engine: Reset(ConversationKey)
        Engine->>Runtime: 删除 Runtime-native Mapping
        Runtime-->>Engine: Mapping 已删除
        Engine-->>Adapter: 返回 Reset 结果
    end
```

Engine 在这条流程中管理 Admission、Cancellation、Interaction Routing、Event Ordering 和 Result Normalization。
Runtime Adapter 只管理 Runtime 特有的 Mapping、Protocol Translation、Credential Materialization、`InitShell` 执行和 Runtime-local File Exposure。

### 3.2 公共 Resource Interface

精确声明保留在 `internal/agentengine`。
评审入口为：

| Resource | 操作 | 用途 |
|---|---|---|
| `Agents()` | Create、Get、List、Update、Delete、Recreate | Agent 期望配置和 Runtime 生命周期 |
| `Conversations(agentID)` | Files、Run、Cancel、Reset、Resolve | 限定到一个 Agent 的 Conversation 执行 |
| `Conversations(agentID).Files()` | Create、Get、Delete | 由已选择 Agent 限定的不可变文件 |
| `ConversationRuntime` | Run、Cancel、Reset、Resolve | Engine 后面的 Runtime 特有直接执行 |

公共 Contract 已具备 Transport-ready 形态，但本阶段不定义远程 Engine Protocol：

| Engine Operation | HTTP Mapping |
|---|---|
| `Agents().Create/Get/List/Update/Delete` | Agent Collection 与 Resource 的 POST、GET、PUT、DELETE |
| 带 `spec.desired_state` 的 `Agents().Update` | Start 与 Stop Action POST Endpoint |
| 带 Options 的 `Agents().Recreate` | Recreate 与 Upgrade Action POST Endpoint |
| `Conversations(agentID).Run` | 使用 JSON Request 和 JSON 或 SSE Result Stream 的 Turn POST |
| `Cancel/Reset/Resolve` | 使用 typed JSON Body 的 Conversation Action POST Endpoint |
| `Conversations(agentID).Files().Create` | File Metadata 加 Streaming HTTP Request Body |
| `Conversations(agentID).Files().Get/Delete` | 带 Metadata Header 的 Content GET 和 DELETE |

`context.Context`、`io.Reader`、`io.ReadCloser` 和 `EventSink` 属于 Go Client 与 Server Adapter 边界。
跨过该边界的 Request、Resource、Event、Result 和 Error Value 都具有稳定 JSON 表示，并且不包含 Runtime Path 或进程内 Capability。

`AgentInterface` 是 Agent Resource 的 Collection-scoped API，调用方不能依赖当前 `internal/agent.Service`。
生产环境中的 HTTP、CLI、Participant、Team、Task 和投递恢复管理路径，通过共享的 Engine 实例执行 Agent 期望状态查询、变更和 Runtime 生命周期操作。
Profile、Skill、MCP Server、Instructions、Memory 配置和生命周期意图都是 Agent Resource Field，不再对应独立的 Endpoint-shaped Interface Method。
生效指令、可读 Memory、脱敏 Model 数据、Capability、Runtime Readiness 和 Sandbox Identity 作为 Get 返回的观测 Status Projection。
Create 保留显式 ID 与 Template Selection，Update 使用 Field Mask 和 Resource Version 表示部分变更，Recreate Options 区分普通重建与镜像升级。
Channel Binding、Participant 记录、Workspace 浏览、日志流、Runtime 注册和 LLM 执行继续由现有 Owner 负责。
阶段 2 的 Engine Facade 可以通过私有 Backend 包装当前 Agent Service，以优先复用已经验证的 Agent 持久化和 Runtime 生命周期代码。
该包装只是 Engine 内部实现，不进入公共 Contract，也不阻止阶段 4 把宽泛的 Service 拆成明确的 Storage、Lifecycle 和 Runtime 组件。
Conversation Execution 不保存重复的 Agent Record，并协调 Active Turn 和生命周期变更。
阶段 1 的 Conversation 实现只能通过 Composition Root 注入的私有 Adapter 访问当前 Agent Service。
阶段 2 扩展这个边界，让完整 Engine 在不先重写现有 Agent 组件的前提下实现 `Agents()`、`Conversations(agentID)` 及其嵌套 `Files()` Resource。
阶段 4 再在保持外部行为的前提下替换临时 Facade，并收敛 Agent State 与 Runtime 生命周期的内部 Owner。
如果内部重构确实需要调整公共 Interface，该调整继续通过共同 Review 完成，并同步所有实现、Mock Client 和 Contract Test。

`AgentSpec` 包含完整期望状态：Name、Description、Instructions、Role、Runtime、Model、Skills、MCP Server、Memory 配置和生命周期意图。
`AgentInterface.Update` 在 Agent 生命周期 Gate 内比较当前 Spec 与完整期望 Spec，保留未提供的只写 Secret，并且只协调发生变化的子系统。
HTTP Patch 以及聚焦的 Skill、MCP、Profile、Instructions 和 Memory Route，把部分意图翻译成 `AgentUpdateRequest.FieldMask`，再执行同一个 Update Planner，不维护独立生命周期路径。
Start 和 Stop 设置 `spec.desired_state`；应用 `running` 时，即使期望值没有变化，也会协调观测到的 Runtime Drift。
Upgrade 使用设置 `UpgradeImage` 的 `Recreate`，而不是独立 Interface Method。
完整 Update 中缺少某个 Skill 或 MCP Server 表示删除它；未提供 Runtime Credential 表示保留，显式空 Credential Map 表示清除。
`RuntimeSpec.Credentials` 是 Workspace 相对文件路径到完整 Secret File Content 的 Map。
`RuntimeSpec.InitShell` 是以 Runtime Workspace 为工作目录执行的幂等 Shell Program。
Create 和 Update 把这两个 Field 作为完整 Runtime 期望状态的一部分进行替换。
Go Name 遵循 Kubernetes Go API Field Convention；序列化形式使用 `credentials` 和 `initShell`。
`Credentials` 在 Create 和 Update 中是 Write-only；Create、Update、Get 和 List 返回的所有 `Agent` 都省略其 Value。

阶段 2 的 Codex Runtime Adapter 验证每个相对路径，以严格权限原子写入 Credential File，并删除完整 Update 中省略的旧 File。
它仅在 Credential File 可用后运行 `InitShell`；File 或 Shell 失败会让 Agent Operation 失败，并恢复此前受管理的 Credential File。
`InitShell` 使用选定的 Workspace 作为 `cwd`，并接收与 Codex Process 相同的 `HOME`、Agent 专属 `CODEX_HOME`、Model Environment 和 Reserved Variable Filter；Shell Export 在脚本退出后不会继续生效。
Credential Value 不能进入 Log、Status Message、Event、Transcript、Shell Argument 或 `InitShell` 本身。
Host Feishu Adapter 负责 Delivery 时，Codex Adapter 不接收 Feishu Credential；这两个 Field 不改变 Channel Ownership。

`AgentStatus` 包含观察到的生命周期状态和当前 Runtime ID。
更新 Agent 时，把完整期望 Specification 作为一个 Resource Update 替换。

`ConversationInterface` 不暴露 CRUD Method，因为 Engine 不持久化 Conversation Resource。
阶段 1 已启用 `Run`，并使用 `context.Context` 处理当前 Request Cancellation。
阶段 2 在同一个 Contract 中启用 `Cancel`、`Reset`、`Resolve` 及其相关 Request Field，使 Engine 和 Adapter 可以独立实现并通过 Mock Client 对齐行为。

### 3.3 Conversation 语义

本节描述完整的目标 Contract。
阶段 1 只使用 `TurnID`、`ConversationKey`、Text `InputPart`、Text 和 Tool Event，以及终态 Result。
阶段 2 一次补齐 Continuation Policy、按 Conversation 快速失败的串行化、File、Interaction、Structured Output 和显式生命周期 Method，不再按调用方拆分 Engine 能力。

`ConversationKey` 是调用方拥有的不透明 Identity。
Engine 只校验其非空且长度有界。
它不会解析 Key 中的 Room、Thread、Channel、Binding 或 Session 字段。

`TurnID` 是调用方为一次 `Run` Request 生成的不透明 Identity。
Channel Adapter 在完成 Ingress Validation 后生成该值，并让同一个 Source Event 的重试保持稳定；Session HTTP Adapter 则为其 Request 生成一个 Response ID。
Engine 只校验其非空且长度有界，并原样传递给 Runtime Adapter。
它与 `ConversationKey` 保持不同；Channel Adapter 可以把限定 Scope 的 Source Event ID 规范化为 `TurnID`，因为该 Event 在 Delivery Retry 间标识同一个逻辑 Turn。
在一个 Engine 进程内，`(agentID, ConversationKey, TurnID)` 是幂等 Identity。
进行中的重试加入原 Turn，已完成且已分派的重试回放有界的进程内 Progress Event 并返回缓存 Result，不会再次提交 Runtime Turn。
使用相同 Identity 但不同的规范化 Input 或 Policy 会返回 `invalid_request`。

每个 Adapter 负责构造无碰撞 Key：

| 调用方 | Key 来源 |
|---|---|
| 内置 IM | Agent Participant、Room 和可选 Thread Root |
| 飞书 | App Binding、Chat 和可选 Thread Root |
| Session API | Session Binding Store 保存的随机内部 Key |

Engine 同时只允许 `(agentID, ConversationKey)` 存在一个 Turn 或原子 Control Operation。
不同 Conversation Key 可以并发执行。
`AdmissionPolicy` 为不同的重叠 Turn 选择 `reject_if_busy`、`wait` 或 `supersede`。
`reject_if_busy` 立即返回 `conversation_busy`，并继续作为匿名 Session API 的默认策略。
`wait` 等待当前 Turn 或 Control Operation 完成后再竞争 Admission。
`supersede` 关闭 Admission、取消当前 Turn、等待 Runtime 真实清理完成，并且不留新 Run 插入的窗口直接准入替代 Turn。
因此 Cancel 使用按 Agent 限定的 `ConversationKey` 和 `TurnID` 精确标识一个运行中的 Turn。
Resolve 额外携带 `InteractionID` 来标识一个 Pending Interaction。

Turn 生命周期结束后，`TurnID` 只在有界的进程内幂等窗口中保留。
它不是 Conversation Key、Runtime 原生 Conversation Mapping、Transcript Identity 或持久化 Engine Resource，因此进程重启后仍依赖 Source 侧去重。
`Reset` 仍按 `ConversationKey` 限定，`Resolve` 仍按 `ConversationKey` 和 `InteractionID` 限定。

`ContinuationPolicy` 明确 Runtime Mapping 行为：

- `create_or_resume` 创建缺失的原生 Mapping，或恢复已有 Mapping。
- `require_existing` 在 Mapping 缺失时返回 `conversation_not_resumable`。

`InteractionPolicy` 选择调用方如何处理 Blocking Runtime Interaction：

- `resolve` 允许调用方通过 `Resolve` 回答。
- `reject` 使用 `interaction_unsupported` 结束 Turn。
- `skip_user_input` 提交 Runtime 的空答案形式，并安全拒绝 Permission。

内置 IM 使用 `resolve`。
匿名 Session API 使用 `reject`。
飞书保留当前 `skip_user_input` 行为。

### 3.4 Input、Event 和 Result

`TurnRequest.Input` 是一个有序 `InputPart` List。
Text Part 包含 `Text`。
File Part 包含一个仅携带 FileID 的 Agent-scoped `InputFile` 引用。
不存在并行 File List，也不存在 Engine File Preparation 步骤。
增量实现不会把这个 Contract 缩窄为 String。
阶段 1 的 Session HTTP Adapter 创建一个 Text Part，私有 Codex Runtime Adapter 则在调用当前 Runtime API 前按顺序合并 Text Part。
File Part 保留公共 Type，但在后续阶段实现 File Execution 前，会在 Runtime Dispatch 前返回 `file_unavailable`。

Event Sink 接收一个逻辑 Turn 的有序进度：

- Text Delta。
- Thought Delta。
- Activity Update。
- Interaction Request。
- 已验证的非文件 Output Item。

Sink 不是 Event Bus、Transcript Store 或 Channel Renderer。
每个 Event 都携带 `TurnID` 和单调递增的 `Sequence`。
Adapter 可以使用 `(TurnID, Sequence)` 对重试后回放的 Envelope 去重。

`Run` 只返回一个 `TurnResult`，没有第二套裸 Runtime Error。
成功 Result 通过 `Files` 携带 JSON-safe 不可变文件 Metadata；失败或取消 Result 不包含可投递文件。
`Dispatched=false` 表示原生 Turn 尚未提交。
这包括 Engine 拒绝 Admission，以及创建、解析或持久化必要的 Runtime 原生 Conversation Mapping 失败。
`Dispatched=true` 表示 Continuation Policy 已成功满足，必要的 Mapping 已经持久化或解析，并且原生 Turn 已提交。
提交后，成功、失败、取消和超时都保持 `Dispatched=true`。

稳定失败类别包括无效请求、Agent 不可用、Runtime Adapter 不可用、Conversation Busy、Runtime Mapping 缺失、File 不可用、不支持 Interaction、取消和 Runtime 失败。

## 4. Owner

每个事实只有一个 Owner：

| 组件 | 负责 | 不负责 |
|---|---|---|
| Agent Resource 实现 | Agent 持久化、包含 Runtime Credential 和 `InitShell` 的期望配置、Runtime 生命周期、Workspace 和 Runtime Provision | Turn Input、Transcript、Runtime 原生 Conversation Mapping、Channel Event Worker 生命周期 |
| Agent Engine | Admission、每 Conversation 串行化、Dispatch、Active Turn、Pending Interaction、Event 顺序、规范化 Result、进程内不可变 Artifact Store | 持久化 Agent 或 Conversation State、外部文件传输、Channel 行为 |
| Runtime Adapter | Runtime Credential 序列化、`InitShell` 执行、原生 Conversation Mapping、直接 Runtime 协议、Runtime Event 转换、Input File 暴露、Output File 授权 | Channel Subscription、Transcript、Agent 持久化 |
| Channel Adapter | Ingress、Identity、Binding 和 Channel Event Worker 生命周期、Host 侧 Channel Credential、Deduplication、Hidden Context、Input File 授权、已授权 Output File 投递、Transcript、Rendering、Ack | Runtime 原生 Mapping、Engine Admission |
| Session HTTP Adapter | HTTP Validation、Session Binding、SSE 和 Error Mapping | IM Room、Message、Participant、Transcript |

阶段 1 的最小 Session 实现没有生命周期协调。
阶段 2 的完整 Engine 让 Agent Resource 实现和 Conversation Execution 共享一个 Agent Lifecycle Gate，确保生命周期变更不会替换活动 Turn 正在使用的资源。
Gate 保持为实现细节，不进入公共 Interface。

## 5. 主要流程

### 5.1 匿名 Session

入口保持不变：

```http
POST /api/v1/agents/{agent}/sessions/{session_id}/responses
```

目标流程为：

```text
Session HTTP Adapter
  -> 加载或创建 Session Binding
  -> 生成 TurnID
  -> Conversations(agentID).Run
  -> Runtime Adapter
  -> 把 Engine Event 映射为现有 SSE
```

阶段 1 Session Binding Store 以 `(agentID, externalSessionID)` 作为唯一 Key，只包含这些 ID 和一个随机不透明 Conversation Key。
它不保存 Prompt、Output、File、Runtime Handle、Interaction 或 Recovery State。
进程重启后，Binding 复用相同 Conversation Key，Codex Adapter 调用现有的幂等 `EnsureSession` 行为。
后续严格续接设计可以在其他调用方真正需要时增加显式 Mapping State。

Route 保留当前 Request Input、`stream`、Body Limit、Timeout、SSE、Error Envelope、`409 session_busy` 和空 `room_id` Response Metadata。
它不创建 Room、User、Participant、IM Message、Participant Work 或 Hidden Channel Context。

### 5.2 内置 IM

```text
IM 持久化用户 Message
  -> Channel Adapter 执行 Routing 和 Deduplication
  -> 构造 ConversationKey、生成 TurnID，并排列 Input
  -> Conversations(agentID).Run
  -> Runtime Adapter
  -> Channel Adapter 渲染 Activity 和最终 Message
```

Channel Adapter 保留 Mention、Thread Context、Skill、Participant Work、Stop、`/new`、Superseding、Replay、Reaction 和 Transcript 行为。
它可以按当前方式，在调用 Engine 前把 Hidden Channel Context 或新 Thread Context 合并进规范化 Text Input。
Engine 不单独建模该 Context。

`/new` 使用同一个 `ConversationKey` 调用 `Reset`。
Engine 在同一个 Conversation Gate 内关闭 Admission、取消并 Drain 活动 Turn、调用 Runtime Adapter 删除原生 Mapping，然后才重新开放 Admission。

### 5.3 Runtime Adapter

Create、影响 Runtime 的 Update 或 Recreate 期间，Agent Resource 实现根据 `RuntimeSpec.Adapter` 选择已注册的 Adapter。
Adapter 物化 `Credentials`、运行 `InitShell`，并且仅在两个步骤都成功后启动 Runtime。
Credential Layout 和初始化机制是该 Adapter 的内部实现。

获得 Admission 后，Engine 为 Agent 已就绪的 Runtime 选择注册的 Adapter。
所选 Adapter：

1. 根据 `ContinuationPolicy` 解析或创建并持久化原生 Conversation Mapping。
2. 执行有序 Input。
3. 把原生 Progress 转成 Engine Event。
4. 在公开 Text 发出前解码合格的 CSGClaw Structured Output。
5. 返回一个终态 Result。

Codex Adapter 复用现有 `conversation_sessions` Mapping 和 `EnsureSession` 行为。
Reset 为同一个 `ConversationKey` 替换该 Mapping。

只有 OpenClaw 提供稳定的直接提交、终态、Event Delivery、Cancel、Reset 和 Interaction 行为后，才增加 OpenClaw Adapter。
它不能通过 IM 或飞书 Event 模拟直接执行。

## 6. 关键边界

### 6.1 持久化

Agent Engine 没有持久化 Conversation Store。
它只保留有界的进程内已分派 Turn Progress Event 和 Result Cache，用于重试幂等。
Agent Resource 实现拥有期望的 Runtime Credential，每个 Runtime Adapter 拥有自己物化出的 Credential File。
Runtime Adapter 拥有原生 Conversation Mapping。
Channel Adapter 拥有 Transcript 和 Source Delivery State。
Session Binding Store 只拥有外部 Session ID 和 Conversation Key 之间的关联。

Engine 进程重启会中断等待中和运行中的 Turn，并清空进程内幂等 Cache。
它不会删除 Runtime 原生 Mapping。
设计不承诺跨重启 Replay、Exactly-once Execution 或 In-flight Side Effect 恢复。

### 6.2 文件

内置 IM 继续拥有其持久化 Attachment Metadata、Blob、Download Token 和 GC。
调用 `Run` 前，调用方通过 `Conversations(agentID).Files().Create` 上传内容，并获得带 opaque FileID 的不可变 Metadata。
`InputFile` 只携带该 FileID，因此调用方和 Engine 不需要共享文件系统或 `SourcePath`。
Engine 在选定 Agent Scope 内解析 ID，Runtime Adapter 再把不可变快照复制到执行环境。
解析会在 Admission 前取得 Lease，因此无效的 Supersede Request 不会取消 Active Turn，并发 Delete 也不能使已准入的 Input 失效。

只有新上传或明确再次引用时才把 File 加入 Input。
不能仅为继续 Runtime 原生 Conversation 而重发之前的 File Byte。

对于输出，每个 Runtime Adapter 消费 typed 原生文件引用，并在跨过 Engine 边界前完成解析。
Codex app-server Adapter 只在 Engine 创建的 Thread 上暴露接收 workspace 相对路径的 typed dynamic tool `csgclaw_publish_file`；Legacy Bridge Thread 不发布无法投递的能力。
当前 app-server Protocol 不能在 cold `thread/resume` 时附加 Dynamic Tool，因此恢复出的 Engine Mapping 会续接同一个原生 Thread 以保留 Conversation Context，但在协议支持重新附加前不会发布 File Capability。
直接使用 Responses 的 Adapter 应消费原生生成文件引用和工具输出。
Runtime Adapter 通过受 workspace 根目录约束的文件访问打开解析后的本地路径，拒绝越界路径和最终为 symlink 的非普通文件，并注册包含权威名称、MIME type、大小与 SHA-256 元数据的唯一不可变快照。
Engine 分配与 SHA-256 无关的随机 opaque ID，并在成功的 `TurnResult.Files` 中返回 Metadata，但不暴露宿主路径。
Channel Adapter 调用 `Conversations(agentID).Files().Get(fileID)` 获取权威 Metadata 和独立快照流，Turn Replay 会保留相同 ID 和内容。
`Get` 会原地校验不可变快照并返回 Leased Descriptor；Delete 会立即撤销新访问，并在最后一个 Active Lease 关闭后删除实际字节。
它只能上传成功 `TurnResult` 中的文件。
Channel Adapter 不得下载 `resource_link`；普通 HTTP(S) 资源链接始终只是 Markdown 链接。

### 6.3 Structured Output 和 Interaction

唯一共享 Decoder 只拥有 `resource_link` 和 Detached `request_user_input` 的 `::csgclaw-output::` Grammar。
Runtime File Output 使用 typed 原生 Runtime Event，不依赖解析 Assistant Text 或 Command Stdout。
原始控制行不能进入公开 Text 或 Channel Renderer。

Blocking Runtime Permission 或 User Input 保持同一个 Turn 打开，并使用 `Resolve`。
Detached `request_user_input` 完成当前 Turn，并在用户回答后创建后续 Turn。
Detached Input 不调用 `Resolve`。
Engine 在调用 Runtime Adapter 前原子 Claim Pending Interaction，因此重复 Action 只有第一个能够进入 Runtime Resolution。
Runtime Resolution 失败时，只要同一个 Turn 仍处于 Active，就把 Claim 恢复为 Pending；成功时则消费该 Interaction。
Channel Adapter 授权操作用户，Engine 只把 `ResponderID` 当作不透明的审计 Identity，绝不使用它做授权。

Secret Interaction Answer 不能进入 Log 或 Transcript。
Detached Secret Answer 也不能插入模型续接。

### 6.4 并发和生命周期

Engine 在获取 Agent Execution Lease 前应用 Request 的 `AdmissionPolicy`。
等待和 Supersede 保持为进程内状态，Runtime 原生 Conversation Mapping 仍按 Conversation Identity 建立索引。
Channel Adapter 可以为 Subscription、Deduplication 和 Ack 保留 Source Ingress Buffer。

Sink 失败时，Engine 在可能时请求 Runtime Cancel，并等待 Runtime 真实终态后才释放 Admission。
Runtime 不支持 Cancel 时，Engine 继续监督到终态。

Agent Lifecycle Gate 是每 Agent 的进程内并发控制原语，不是 Service 或公共 Interface。
它记录 Admission 是否开放以及哪些 Execution Lease 正在运行。
阶段 2 扩展现有的 `internal/agent.agentLifecycleGate`，不引入第二个 Coordinator。
如果扩展后的职责以后需要更合适的内部名称，可以重命名，但不改变该公共 Contract。

Run Admission 和生命周期变更通过同一个 Gate 串行化。
Run 只有在原子地确认 Agent Ready，并使用选定的内部 Runtime Handle 登记 Active Turn 后才能分派。

Stop、影响 Runtime 的 Update、Recreate 和 Delete 首先把 Agent 标记为不可用，并关闭新 Admission。
新的 Run 返回 `TurnFailed`、`Dispatched=false` 和 `agent_unavailable`。
运行中的 Turn 在 Runtime State 变更前可以执行到终态。

配置的 Drain Timeout 限制等待时间。
超时后，生命周期操作失败，不替换或删除当前 Runtime，并且只在该 Runtime 仍然 Ready 时重新开放 Admission。
Agent 持久化和 Runtime Adapter 调用在 Gate 临界区之外执行，期间 Admission 保持关闭。
生命周期调用失败时，只有确认原 Runtime 仍然 Ready 才重新开放 Admission；否则 Agent 保持不可用，并在 Status 中记录观察到的失败。

Stop 保留 Runtime Conversation Store，Start 只在 Runtime Ready 后重新开放 Admission。
Recreate 和 Delete 在替代 Runtime Ready 或删除完成前，删除 Runtime 所有的 Conversation Mapping。
Mapping 丢失时，严格调用方收到 `conversation_not_resumable`。

### 6.5 Channel Event Worker 生命周期

Channel 层是 Channel Event Worker 生命周期的唯一 Owner。
Composition Root 只启动每个 Channel Adapter 一次，Adapter 按稳定的 Binding Identity 协调已启用的 Binding。
Binding 的创建、更新和删除通过幂等操作启动、重新配置和停止唯一的 Worker。
Worker 监听传入的 Channel Event，面向 Agent ID 并调用 `Conversations(agentID)`，不绑定 Runtime ID 或原生 Session ID。

Agent Resource 实现、Agent Engine 和 Runtime Adapter 既不控制 Channel Event Worker，也不访问 IM Message 持久化。
每个 Channel 迁移时，从 Agent Resource Path 删除当前 `LifecycleObserver` 和 `BindingActivator -> codexBridgeMgr` 控制链。
Binding 变更直接调用所属 Channel 层。

Agent Stop、影响 Runtime 的 Update、Recreate 和 Runtime Restart 不改变 Binding、Worker 或已保存的 Transcript。
Agent 不可用期间，Worker 继续正常 Ingress 和 Ack，并按 Channel 行为处理 `agent_unavailable`。
Agent 删除由 Application 和 Binding 边界协调：删除或停用关联 Binding，Channel Adapter 停止对应 Worker，已保存的 Transcript 继续由 Channel 拥有。
`AgentInterface.Delete` 本身保持 Channel-neutral。

## 7. 增量实现

阶段只描述交付顺序，不把 Agent Engine Contract 或实现能力人为切碎。
从阶段 2 开始，Agent Engine Interface 是 Engine 与 Adapter 之间唯一的中介线。

### 阶段 1：匿名 Session Path（已完成）

- 建立独立的 `internal/agentengine` Contract 和进程内 Conversation 实现。
- 通过私有 Adapter 复用 Codex 的 `EnsureSession`、`Prompt` 和 Scoped Runtime Event。
- 让 Streaming 和 Non-streaming 匿名 Session Request 通过 `Conversations(agentID).Run` 执行。
- 增加 Agent-scoped Session Binding Store。
- 删除匿名 Session 对 IM Room、Message 和 Participant 的持久化依赖，同时保留现有 HTTP、SSE、Timeout 和 Error Shape。
- 对不受支持的 Runtime Adapter 明确失败，不启动 Fallback Path。
- 保持 Agent CRUD、内置 IM、飞书、Team、Task、Scheduled Task、Notification 和 Work 行为不变。

### 阶段 2：Agent Engine 与 Mock 基线 - 已完成

- 实现完整 `agentengine.Interface`，并提供实现同一 Contract 的并发安全、有状态 `enginetest.MemoryClient`。
- Interface 不是不可修改的冻结协议；实现中发现遗漏或错误时，可以通过共同 Review 调整，并在同一次变更中同步真实 Engine、Mock Client、Contract Test 和受影响的 Adapter 调用代码。
- 实现 `Agents()`、`Run`、`Cancel`、原子 `Reset`、Claimed `Resolve`、显式 Admission Policy、Turn 重试幂等、TurnID Event Envelope、File Input、已授权 Runtime File Output、Interaction、Structured Output、`Dispatched` 和稳定 Error。
- Agent Engine Facade 优先包装现有 Agent Service、Codex Session、Broker、Structured Output 和 Runtime Provision 代码，不把内部重构作为完成 Engine 的前置条件。
- Agent Engine 通过共享 Lifecycle Gate 协调 Agent Mutation、Admission、Active Turn、Drain 和固定 Runtime Handle。
- 把匿名 Session API 迁移到 `agentengine.Interface`，同时保留其 HTTP Contract 和零 IM Entity 创建。
- 通过 `MemoryClient` 证明 Test-only 飞书 Ingress 和 Output Delivery Harness，包括飞书图片上传、文件上传和消息 OpenAPI 调用，并让飞书 Credential 只存在于当前 Channel Binding Owner。
- 生产 Channel Adapter 的实现、融合和原子切换保留在阶段 3。
- 将 Codex `RuntimeSpec.Credentials` 物化为 Workspace 相对文件，并在 File 可用后运行 `RuntimeSpec.InitShell`。

### 阶段 3：Agent Engine 与 Adapter 融合

- Composition Root 把真实 Agent Engine 注入已通过 Mock Client 验证的 Adapter。
- 运行 Engine 与 Adapter 的联合 Contract、并发、生命周期和端到端行为验证。
- 验证通过后，把目标 Channel 的执行路径原子切换为 `Channel Adapter -> Agent Engine -> Runtime Adapter`。
- 保留现有 Room、Thread、Mention、File、Interaction、Work、Stop、`/new`、Transcript、Rendering、Reaction 和 Ack 行为。
- 切换时删除对应的旧执行入口、重复队列、重复取消状态和 Agent 生命周期到 `codexBridgeMgr` 的控制链。
- 不运行双执行、Shadow Prompt 或 Fallback；任一必需能力缺失都阻止切换。

### 阶段 4：重构 Agent Engine 内部组件

- 在外部行为保持不变的前提下重构 Engine 内部组件；公共 Interface 默认保持稳定，确有必要时仍可通过共同 Review 演进。
- 把阶段 2 的 Agent Service Facade 收敛为职责明确的 Agent Resource Backend、Conversation Coordinator、Lifecycle Gate、Runtime Adapter Registry 和 Runtime Adapter。
- 抽取可复用的 Storage、Runtime Provision、Credential、InitShell、File Exposure、Interaction 和 Structured Output 能力，删除重复状态与反向控制依赖。
- 保持管理调用方迁移后由 `Agents()` 提供统一的 Engine 入口，再在不改变调用方行为的前提下替换其私有 Agent Service Facade。
- 使用阶段 2 的 Contract Test 和阶段 3 的端到端测试证明重构不改变 CSGClaw 现有行为；如果 Interface 发生经 Review 的调整，则同步更新 Adapter 和 Mock Client。

每次合入都必须保持 CSGClaw 现有行为可用。
阶段 2 的两侧可以独立开发和验证，阶段 3 只负责融合与原子切换，阶段 4 只改变 Engine 内部结构。

## 8. 验收标准

### 8.1 阶段 1（已完成）

- Streaming 和 Non-streaming Session Request 都使用 `Conversations(agentID).Run`。
- 匿名 Session Execution 不创建 IM Entity，并保留现有 HTTP、JSON、SSE、Timeout 和 Error Shape。
- Session Binding 以 `(agentID, externalSessionID)` 作为唯一 Key，持久化一个不透明 Conversation Key，并在重启后复用它。
- 同一 Agent 和 Session 的重叠请求返回 `409 session_busy`；不同 Session，以及不同 Agent 下相同外部 Session ID 可以并发执行。
- Codex Text Delta 和 Tool Activity 保留现有 SSE Shape 和 Secret Redaction。
- Session Adapter 发送一个 Text `InputPart`，私有 Codex Runtime Adapter 保留有序的 Multi-part Text Input。
- 不受支持的 Runtime Adapter 在创建 Binding 前明确失败，不启动 Fallback。
- Request Cancellation 通过 `context.Context` 到达 Codex Runtime。
- 取消后的新请求会等待 Runtime Cleanup 完成再开始；仍在运行的重叠请求继续立即返回 `409 session_busy`。
- Agent CRUD、内置 IM、飞书、Team、Task、Scheduled Task、Notification 和 Work 行为保持不变。

### 8.2 目标架构

- `internal/agentengine` 不 Import IM、Participant、Channel、Team 或具体 Runtime Package。
- `Interface` 暴露 `Agents()` 和按 Agent 限定的 `Conversations(agentID)`；文件是通过 `Conversations(agentID).Files()` 选择的嵌套 Conversation Resource。
- Conversation Request 不重复携带 Agent ID。
- Conversation Key 保持不透明并由调用方拥有。
- 每次 Run 都携带调用方生成的不透明 Turn ID，Cancel 使用 Conversation Key 和 Turn ID 定位一个 Turn。
- Engine 不持久化 Agent、Conversation、Transcript、File 或 Delivery State；Artifact Store 仅存在于进程内，并由显式删除或 Turn Cache 淘汰限制生命周期。
- Agent Resource 实现、Agent Engine 和 Runtime Adapter 不依赖 Channel Event Worker，也不访问 IM Message 持久化。
- Channel Event Worker 按稳定的 Binding Identity 建立索引，不使用 Runtime ID 或原生 Session ID。
- Runtime 原生 Conversation Mapping 只有一个 Owner。
- 阶段 2 完成后，外部调用方只依赖 `AgentInterface`，`Conversations()` 只通过 Engine 内部 Facade 访问 Agent 可用性和 Runtime 选择。
- 阶段 4 完成后，`AgentInterface` 实现是唯一的 Agent 持久化和 Runtime 生命周期 Owner，并且 Engine 内部不再依赖宽泛的 `internal/agent.Service`。
- Runtime Credential File Layout 和初始化由各 Runtime Adapter 负责。
- Runtime File Output 只能通过成功的 `TurnResult.Files` 跨过 Engine，其中 Metadata 指向包含 opaque ID、MIME type、大小和 SHA-256 且不含宿主路径的不可变快照。
- Input File Part 只包含 Agent-scoped FileID；公共 Request 和 Result 不包含本地 Source Path 或 Reader 能力。
- 公共 Resource 与 Event Value 使用稳定 JSON Field Name，`Files.Create` 和 `Files.Get` 分别映射 HTTP Request 与 Response Body。
- 缺少 Runtime Adapter 时明确失败，不启动 Fallback Path。
- Go Contract 和两种语言文档保持同步。

### 8.3 目标行为

- 匿名 Session 不创建 IM Entity，并保留公共 API Contract。
- 不同 Conversation 可以并发，一个 Conversation 内保持串行。
- 内置 IM 保留 Room、Thread、Mention、File、Activity、Stop、Work、Interaction 和 `/new` 行为。
- 飞书保留已有 Text 行为，并通过 Agent-scoped 快照投递成功 Turn 的 Engine Output File。受支持图片使用飞书 Image Message；较大图片和其他产物在飞书上传限制内使用 File Message。
- Binding 创建、更新和删除通过幂等操作协调唯一的 Channel Event Worker。
- Agent Stop、Recreate 和 Runtime Restart 既不重启 Channel Event Worker，也不删除 Binding 或 Transcript。
- Agent API 删除会删除或停用关联 Binding、停止对应 Event Worker，并保留已保存的 Transcript。
- Codex Conversation 在 Stop 后再次 Start 时可以继续。
- 生命周期变更关闭 Admission、Drain 运行中的 Turn，并且不会替换活动 Turn 正在使用的 Runtime。
- Lifecycle Drain Timeout 保持当前 Runtime 不变，并返回失败的生命周期操作。
- Session Binding 按 `(agentID, externalSessionID)` 唯一，Mapping 失败后保持 `initializing`，进程重启后使用相同的 Conversation Key 重试。
- Codex Credential File 以严格权限原子替换；`InitShell` 执行失败会让 Agent Operation 失败，并恢复此前受管理的 Credential。
- Create、Update、Get 和 List Result 省略 Runtime Credential Value。
- Recreate 和 Delete 如实报告严格续接 Mapping 缺失。
- CSGClaw Structured Output 不泄漏原始控制行。
- 普通 `resource_link` Output 不得被下载或提升为可上传文件。
- Secret Answer 不进入 Log 或 Transcript。

### 8.4 目标验证

- Contract Test 覆盖 Run、Cancel、原子 Reset、Claimed Resolve、Event Envelope、终态 Result 和稳定 Error。
- 测试覆盖 Turn 重试幂等、不同 Conversation 并发、Reject、Wait、Supersede、Sink Failure 和 Cancel 行为。
- 测试覆盖无 MCP、本地 MCP、远程 MCP、Text Input、File Input、已授权 Runtime File Output 和 Workspace 越界拒绝。
- 飞书 OpenAPI 端到端 Harness 验证图片上传、文件上传、消息投递、失败 Turn 不投递以及 `resource_link` 不下载规则。
- 匿名测试验证 IM Entity 数量不变，并且 Session Binding Scope 按 Agent 隔离。
- Channel 测试验证 Deduplication、Replay、Superseding、Rendering、Binding 驱动的 Event Worker 生命周期和幂等协调。
- Lifecycle 测试验证 Agent Stop、Recreate 和 Runtime Restart 不启动或停止 Channel Event Worker。
- Agent 删除测试验证 Binding 清理、Event Worker 停止和 Transcript 保留。
- Lifecycle 测试验证 Admission 关闭、Active Turn Drain、Drain Timeout、Lifecycle Failure 和 Runtime Pinning。
- 阶段 2 使用同一套 Contract Test 验证 Mock Client 和真实 Engine，并验证 Adapter 可以只通过 Mock Client 独立完成行为测试。
- 阶段 3 联合测试验证真实 Engine 与 Adapter 的契约一致、原子切换没有双执行或 Fallback，并保留所有现有 Channel 行为。
- 阶段 4 测试验证 Agent API 使用 `Agents()`、临时 Service Facade 已删除；如公共 Contract 经 Review 调整，Mock Client、Adapter 和 Contract Test 必须同步通过。
- Runtime 测试验证分派前完成 Mapping 创建和持久化、严格续接、Reset、Stop 和 Start、Recreate 和 Delete 语义。
- Runtime Adapter 测试验证 Credential Path Containment、原子替换、删除、权限、`InitShell` 失败回滚和 Secret Redaction。
- Agent Contract 测试验证所有返回的 Agent Value 都省略 Runtime Credential。
- 现有 Agent、Session API、内置 IM、飞书、Team、Task、Scheduled Task、Notification 和 Work 回归测试通过。
