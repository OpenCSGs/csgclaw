# CSGClaw Agent Teams 简化架构与 MVP 方案

本文基于当前 `csgclaw` 源码、`/Users/russellluo/Projects/labs/claude-code` 中 Claude Code agent teams 的源码分析，以及早期 `manager-worker-dispatch` skill 的实践经验，重新收敛一版更适合 `csgclaw` 的 Agent Teams 方案。

核心结论：

> `csgclaw` 的 Agent Teams 不应先做一套独立于 IM 的 Team/Work UI。第一版应把现有 IM 的 `room/member/message` 作为人类可见协作界面，把 team/task/approval/presence 作为服务端结构化元数据叠加在 room 之上。

这样可以同时满足三点：

1. 比 Claude Code 完整 agent teams 更简单；
2. 支持比普通 manager-worker-dispatch 更稳定的串行、并行、依赖、审批和人工干预；
3. 能在内建 `csgclaw` IM、Feishu、未来 Matrix 等渠道中复用同一套协作能力，而不是被 Web UI 绑死。

---

## 1. 背景与问题判断

早期多 agent 协作主要依赖 `manager-worker-dispatch` skill：

- 优点：不改 Go 代码，manager 通过 room 消息和 `@worker` 分派任务即可运行；
- 缺点：任务、领取、完成、阻塞、审批都不是一等对象，状态只能靠 prompt、消息历史和 worker 自觉维护，复杂串并行任务不稳定。

Claude Code agent teams 提供了更强的模型：

- team lead；
- teammates；
- task list；
- mailbox；
- approval / shutdown / presence。

但 Claude Code 的实现背景与 `csgclaw` 不同。Claude Code 需要协调多个独立 CLI 进程，因此用本地文件、task 目录、inbox 文件和 lockfile 做轻量共享状态。`csgclaw` 已经有一个常驻本地 server、REST/SSE、agent lifecycle、participant/channel 绑定和 IM room，因此没有必要照搬文件式 mailbox/task lock。

`csgclaw` 应复用 Claude Code 的“协作语义”，不复制它的“进程间文件协调实现”。

---

## 2. 设计目标

### 2.1 必须满足

1. 支持 manager lead + worker teammates 的长期协作。
2. 支持任务的结构化创建、指派、领取、依赖、完成、失败、取消。
3. 支持串行、并行、fan-out/fan-in、人工审批和中途干预。
4. 人类可以在现有 room 中看到关键过程，并通过普通 message 介入。
5. 不要求 Feishu/Matrix 改造 UI，也能使用 agent teams 能力。
6. 第一版实现足够小，能快速做 POC。
7. 架构保留未来扩展成完整 team workspace 的空间。

### 2.2 第一版不追求

1. 完整复刻 Claude Code split-pane / in-process teammate UI。
2. 独立 Team 工作台替代 room 聊天视图。
3. 多 server 实例的一致性。
4. 自动能力匹配、复杂 policy、长期团队记忆。
5. 完整 sideband runtime control protocol。

---

## 3. 关键架构取舍：IM-Native Agent Teams

本方案建议采用 **IM-native Agent Teams**：

```text
Team = 一个带结构化协作状态的 room
Member = room member 对应的 participant
Message = 人类可见协作事件和人工干预入口
Task/Approval/Presence = server 端权威结构化状态
```

也就是说，`team/work` 不是一套替代 IM 的新交互模型，而是叠加在 IM room 上的一层协作元数据。

### 3.1 为什么不能只做独立 Team UI

如果 Agent Teams 的可见性和干预能力只存在于专门 Team UI：

- 内建 Web UI 可以展示；
- Feishu / Matrix 等外部渠道无法呈现完整交互；
- 外部渠道里的用户只能看到普通聊天，无法理解任务状态、审批和进度；
- manager/worker 在外部渠道中的协作能力会退化。

这与 `csgclaw` 当前 channel 设计冲突。现有架构已经把协作对象抽象成：

- channel；
- user；
- room；
- member；
- message；
- participant。

因此，Agent Teams 的第一层产品体验应建立在这些稳定对象上。

### 3.2 为什么仍需要结构化 team/task 状态

只靠 room message 又会回到早期 dispatch skill 的问题：

- 并发领取任务无法保证原子性；
- worker 是否 busy 只能靠猜；
- blocked / depends_on 状态容易丢；
- approval 不能可靠恢复；
- 重试、取消、人工接管缺少权威状态。

所以本方案不是“纯聊天驱动”，而是：

> room/message 负责可见性与干预，server-side task state 负责一致性与编排。

---

## 4. 与当前 CSGClaw 架构的映射

当前代码已经具备 Agent Teams MVP 需要的大部分基础，但需要注意：`internal/im` 只是内建 `csgclaw` channel 的实现，不能把它当成所有 channel 的统一抽象。Agent Teams 应面向 channel-neutral 的 room/member/message 能力编程，再由不同 channel adapter 落到 `internal/im`、Feishu、未来 Matrix 等具体实现。

| 能力 | 当前落点 | 在 Agent Teams 中的作用 |
| --- | --- | --- |
| agent runtime 生命周期 | `internal/agent` | manager/worker 的执行实体 |
| participant 到 agent/user 绑定 | `internal/participant` | team member 的稳定身份 |
| channel-neutral 协作能力 | 建议新增 `team.ChannelAdapter` 或等价接口 | 统一表达 create room / add member / send message / list messages |
| 内建 csgclaw channel | `internal/im` | 内建 channel 的 room/member/message 存储与投影实现 |
| 内建 csgclaw 实时事件 | `internal/im.Bus` | 仅用于内建 Web UI/SSE 的实时更新 |
| 内建 csgclaw participant delivery | `internal/im.ParticipantBridge` | 仅用于内建 channel runtime 接收 room 消息；不是 Feishu/Matrix 的通用 mailbox |
| 外部 channel | `internal/channel/*` + `/api/v1/channels/{channel}` | Feishu/未来 Matrix 的 room/member/message 适配边界 |
| Web conversation UI | `web/app/src/pages/ConversationPage` | 内建 csgclaw channel 的第一版可见性主入口 |

因此，`internal/team` 不应直接写死依赖 `internal/im.Service` 的具体方法作为唯一投影路径。更推荐让 `team.Service` 依赖一个窄接口：

```go
type TeamChannelAdapter interface {
    Channel() string
    EnsureRoom(ctx context.Context, req EnsureRoomRequest) (RoomRef, error)
    AddMembers(ctx context.Context, req AddMembersRequest) error
    SendMessage(ctx context.Context, req SendMessageRequest) (MessageRef, error)
}
```

Phase 0 定义 adapter 时应按最弱 channel 的能力下限设计，而不是按内建 `internal/im` 的便利能力设计。接口语义要保持窄而可移植：

- `EnsureRoom` 只表达“找到或创建可投影的会话目标”，不要泄漏内建 room 的完整生命周期语义；
- `AddMembers` 只保证 best-effort 邀请或同步成员，不要求所有外部 channel 都能强一致确认；
- `SendMessage` 是 MVP 的核心能力，返回可用于排查的 `MessageRef` 即可；
- channel 特有能力通过 optional capability 逐步补充，不提前放进基础接口。

内建 `csgclaw` adapter 可以包一层 `internal/im.Service`、`internal/im.Bus` 和 `internal/im.ParticipantBridge`；Feishu adapter 则通过 `internal/channel/feishu` 发送消息；未来 Matrix adapter 走 Matrix 的 room/member/message API。`ListMessages` 不作为 MVP 必需能力；只有在做外部 channel 的消息 cursor 恢复、人工恢复或历史同步时，才作为 optional capability 加回 adapter，避免把 task 状态恢复依赖 room history。

推荐的概念映射：

| Claude Code 概念 | CSGClaw 简化映射                                                             |
| ---------------- | ---------------------------------------------------------------------------- |
| Team lead        | role=manager 的 participant，且是 team room member                           |
| Teammate         | role=worker 的 participant，且是 team room member                            |
| Team config      | room 上的 team metadata，server 持久化                                       |
| Task list        | room-scoped structured tasks                                                 |
| Mailbox          | MVP 先用 task assignment + room-visible messages；后续再做 sideband envelope |
| Approval         | room-scoped approval object + room-visible approval message                  |
| Presence         | member runtime state + heartbeat summary                                     |
| Team UI          | 先复用 ConversationPage；后续增加结构化侧栏                                  |

---

## 5. 推荐总体架构

新增一个轻量 domain：

```text
internal/team/
  model.go        # TeamMeta / TeamTask / TeamApproval / TeamEvent / Presence
  service.go      # 任务编排、状态变更、审批、room 投影
  store.go        # JSON snapshot + event jsonl
  projector.go    # 将结构化事件写成 room-visible message/event
```

第一版不要新增独立 `internal/teambridge`。原因是 MVP 的关键风险不是 runtime sideband，而是结构化任务和 IM 可见性是否能跑通。

`internal/team.Service` 依赖现有服务：

```text
team.Service
  ├─ participant lookup: participantID -> agentID/userID/channel/role
  ├─ channel adapter registry: 按 channel 选择 csgclaw / feishu / matrix adapter
  ├─ channel adapter: 创建或复用 room、添加 members、写入 visible messages
  ├─ agent: 查询 runtime 状态，必要时 start/stop
  └─ store: 持久化 team/task/approval/presence
```

### 5.1 权威状态边界

权威状态分两层：

1. Channel 权威状态：
   - room；
   - member；
   - message。
2. Team 权威状态：
   - room 是否启用 team；
   - task；
   - approval；
   - presence；
   - orchestration event。

不要把 task 状态只写进 message 文本；message 是投影，不是 source of truth。

对内建 `csgclaw` channel 来说，Channel 权威状态由 `internal/im` 管理。对 Feishu/Matrix 来说，Channel 权威状态在外部平台，`csgclaw` 本地只保存 team 元数据、participant/channel 映射、投影事件和必要的 mirror/cursor。

### 5.2 Channel 复用原则

所有外部渠道都应只需要支持基础能力：

- 创建/绑定 room；
- 添加 member；
- 发送 message；
- 接收用户 message 或 participant event。

Agent Teams 的高级语义由 `internal/team` 解释和维护，再投影成普通消息。因此 Feishu/Matrix 不需要自定义复杂 UI 也能使用。

不同 channel 的能力可以分层支持：

| 能力 | csgclaw 内建 IM | Feishu / Matrix |
| --- | --- | --- |
| room/member/message | 本地完整读写 | 通过 channel adapter 调用外部 API |
| room history 审计 | 本地可完整读取 | 受外部平台权限和历史可见性限制 |
| structured event 展示 | Web UI 可增强渲染 | 默认投影为普通文本，后续可加卡片 |
| participant delivery | 可复用 `internal/im.ParticipantBridge` | 需要各自 webhook/event adapter |
| sideband control | 后续可本地实现 | 后续按渠道能力单独适配 |

---

## 6. 数据模型

### 6.1 TeamMeta

第一版不需要复杂 Team 对象，建议把 team 视为 room 的增强元数据：

```go
type TeamMeta struct {
    ID          string    `json:"id"`       // 建议等于 roomID 或 team-{roomID}
    RoomID      string    `json:"room_id"`
    Channel     string    `json:"channel"`  // csgclaw, feishu, matrix
    Title       string    `json:"title"`
    LeadParticipantID string `json:"lead_participant_id"`
    Status      string    `json:"status"`   // active, paused, archived
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

设计理由：

- `RoomID` 是所有渠道都能理解的协作容器；
- `TeamMeta` 只是标记“这个 room 开启了 team orchestration”；
- 避免在 MVP 中引入与 room 平行的 team membership。

### 6.2 TeamMember

第一版可以不单独持久化完整 member 列表，而是从 room members + participant store 派生。

需要缓存的只是运行态：

```go
type MemberPresence struct {
    TeamID          string    `json:"team_id"`
    ParticipantID   string    `json:"participant_id"`
    UserID          string    `json:"user_id"`
    AgentID         string    `json:"agent_id"`
    Role            string    `json:"role"`   // manager, worker
    State           string    `json:"state"`  // idle, busy, blocked, waiting_approval, offline
    CurrentTaskID   string    `json:"current_task_id,omitempty"`
    Summary         string    `json:"summary,omitempty"`
    LastHeartbeatAt time.Time `json:"last_heartbeat_at,omitempty"`
    UpdatedAt       time.Time `json:"updated_at"`
}
```

`POST /api/v1/teams/{team_id}/presence` 是按 `team_id + participant_id` 的 upsert，不是 append。普通 heartbeat 可以只更新内存中的 `LastHeartbeatAt`；状态变化、checkpoint 和 server 退出时再落盘。

### 6.3 TeamTask

MVP 的关键对象：

```go
type TeamTask struct {
    ID          string     `json:"id"`
    TeamID      string     `json:"team_id"`
    RoomID      string     `json:"room_id"`
    ParentID    string     `json:"parent_id,omitempty"` // 为空表示主任务；非空表示子任务
    Title       string     `json:"title"`
    Body        string     `json:"body"`
    Status      string     `json:"status"` // pending, assigned, in_progress, blocked, completed, failed, cancelled
    CreatedBy   string     `json:"created_by"`
    AssignedTo  string     `json:"assigned_to,omitempty"`
    ClaimedBy   string     `json:"claimed_by,omitempty"`
    DependsOn   []string   `json:"depends_on,omitempty"`
    Priority    int        `json:"priority,omitempty"`
    DeadlineAt  *time.Time `json:"deadline_at,omitempty"`
    TimeoutAt   *time.Time `json:"timeout_at,omitempty"`
    Result      string     `json:"result,omitempty"`
    Error       string     `json:"error,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    UpdatedAt   time.Time  `json:"updated_at"`
    CompletedAt *time.Time `json:"completed_at,omitempty"`
}
```

状态含义：

- `pending`：可领取；
- `assigned`：已指定 worker，但未开始；
- `in_progress`：worker 已 claim 并执行；
- `blocked`：等待依赖、人工输入或审批；
- `completed`：完成；
- `failed`：失败，可人工重试或重新分派；
- `cancelled`：人为取消。

`DeadlineAt` 和 `TimeoutAt` 在 MVP 中先作为数据字段和 UI/投影提示，不要求自动调度。后续的 stale detection 和 auto-reassign 可以基于这些字段实现。

### 6.4 TeamApproval

```go
type TeamApproval struct {
    ID          string     `json:"id"`
    TeamID      string     `json:"team_id"`
    RoomID      string     `json:"room_id"`
    TaskID      string     `json:"task_id,omitempty"`
    RequestedBy string     `json:"requested_by"`
    ApproverID  string     `json:"approver_id,omitempty"`
    Kind        string     `json:"kind"`   // plan, command, write, release, escalation
    Summary     string     `json:"summary"`
    Payload     string     `json:"payload,omitempty"`
    Status      string     `json:"status"` // pending, approved, rejected, cancelled
    Resolution  string     `json:"resolution,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}
```

### 6.5 TeamEvent

用于审计、恢复和 UI timeline：

```go
type TeamEvent struct {
    Seq       int64     `json:"seq"`
    TeamID    string    `json:"team_id"`
    RoomID    string    `json:"room_id"`
    Type      string    `json:"type"`
    ActorID   string    `json:"actor_id,omitempty"`
    TaskID    string    `json:"task_id,omitempty"`
    TargetID  string    `json:"target_id,omitempty"`
    Summary   string    `json:"summary,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}
```

`TeamEvent` 是审计事实，不应依赖 room message 投影是否成功。完整外部 channel 支持前，可以再引入单独的投影结果记录：

```go
type TeamProjection struct {
    EventSeq  int64     `json:"event_seq"`
    TeamID    string    `json:"team_id"`
    Channel   string    `json:"channel"`
    RoomID    string    `json:"room_id"`
    MessageID string    `json:"message_id,omitempty"`
    Status    string    `json:"status"` // pending, sent, failed
    Error     string    `json:"error,omitempty"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

这样即使 Feishu 限流或外部投影失败，审计日志仍然完整；projector 可以按 `TeamProjection.Status` 重试。第一版内建 POC 不要求落盘维护 `TeamProjection`，只要求投影失败不影响主状态，并能从日志和 `projection failed` event 排查。

---

## 7. 任务编排能力

### 7.1 简化但有效的编排模型

MVP 不需要上来实现完整 DAG scheduler，只需要实现以下原语：

1. `CreateTask`
2. `CreateTasks`
3. `AssignTask`
4. `ClaimTask`
5. `ClaimNext`
6. `UpdateTaskStatus`
7. `CompleteTask`
8. `FailTask`
9. `CancelTask`
10. `RequestApproval`
11. `ResolveApproval`

这些原语已经足够表达复杂任务：

- 串行：任务 B `depends_on=[A]`；
- 并行：多个无依赖任务同时 `pending`；
- fan-out：manager 创建多个同级任务；
- fan-in：汇总任务依赖多个 worker 任务；
- 人工干预：用户在 room 中 @manager 或通过 approval resolve 介入；
- 失败恢复：`failed` 任务重新 assign 或 clone。

`CreateTasks` 是批量创建 task 的 MVP 原语，用于 manager 一次提交完整任务计划。它不是通用事务接口，只覆盖同一个 team 内的一批新 task：

- 请求内允许使用临时 `id_ref` 表达依赖；
- MVP 只允许 `depends_on_refs` 引用同一个 batch 内的 `id_ref`，不支持跨 batch 或混用已有 task id；
- 服务端在同一个 `team.Service` 锁内生成真实 task id 并解析 `depends_on_refs`；
- 任一 task 校验失败时整批失败，不写入部分 task；
- 成功后写一组 task created events，并由 projector 合并成一条 room-visible plan summary；
- 单个 task 创建仍保留 `CreateTask`，用于简单任务和人工补充任务。

### 7.2 ClaimNext 原子性

`ClaimNext(teamID, botID)` 是 MVP 中最重要的稳定性改进。

规则：

1. 只能领取当前 room/team 的任务；
2. `depends_on` 中存在未完成任务时不能领取；
3. worker 当前已有 `in_progress` 任务时默认不能再领取；
4. `assigned_to` 为空或等于当前 `botID` 的任务才可被当前 worker 领取；
5. assigned 给其他 worker 的任务不能被抢；
6. 领取动作在 `team.Service` 锁内原子完成；
7. 领取成功后写 team event，并投影一条 room-visible message。

候选任务排序必须可预测，MVP 使用：

1. `priority` 降序；
2. `created_at` 升序；
3. `id` 升序。

`ClaimTask(teamID, taskID, botID)` 也遵循同样的所有权规则：worker 不能 claim 已指派给其他 worker 的任务；manager 如需覆盖指派，必须先显式 `AssignTask` 或使用后续扩展的 force 操作。`claim` 成功后：

- `status` 从 `pending` 或 `assigned` 变为 `in_progress`；
- `claimed_by` 写入当前 worker；
- presence 更新为 `busy`，`current_task_id` 指向该 task。

这比早期 dispatch skill 稳定，因为 worker 不再通过阅读聊天记录判断“这个任务是不是我的”。

### 7.3 Task 状态机

MVP 必须把 task 状态转换定义为服务端规则，而不是只靠 manager/worker prompt 约定。

| 当前状态 | 事件 / 操作 | 新状态 | 允许 actor | 说明 |
| --- | --- | --- | --- | --- |
| `pending` | `AssignTask` | `assigned` | manager | 写入 `assigned_to`，不写 `claimed_by` |
| `pending` | `ClaimTask` / `ClaimNext` | `in_progress` | worker | 仅限未指派或指派给自己的任务 |
| `pending` | `CancelTask` | `cancelled` | manager / human | 取消后不可再 claim |
| `assigned` | `ClaimTask` / `ClaimNext` | `in_progress` | assigned worker | 其他 worker 不能抢占 |
| `assigned` | `AssignTask` | `assigned` | manager | 可重新指派，覆盖 `assigned_to` |
| `assigned` | `CancelTask` | `cancelled` | manager / human | 如果 worker 尚未 claim，不需要 runtime 中断，只投影取消事件 |
| `in_progress` | `UpdateTaskStatus(blocked)` | `blocked` | worker / manager | 等待依赖、人工输入或 approval |
| `in_progress` | `CompleteTask` | `completed` | claimed worker / manager | 写入 `result` 和 `completed_at` |
| `in_progress` | `FailTask` | `failed` | claimed worker / manager | 写入 `error` |
| `in_progress` | `CancelTask` | `cancelled` | manager / human | 需要投影取消事件；runtime 中断是后续 sideband 能力 |
| `blocked` | approval approved | `in_progress` | system | 仅当该 approval 关联 task 且 task 仍 blocked |
| `blocked` | approval rejected | `blocked` | system | 记录 rejection reason，等待 worker 修改方案或 manager 处理 |
| `blocked` | `AssignTask` | `assigned` | manager | 用于人工重派或解除阻塞 |
| `blocked` | `CancelTask` | `cancelled` | manager / human | 终止任务 |
| `failed` | `AssignTask` | `assigned` | manager | 清理 `claimed_by`，可重试 |
| `failed` | clone task | `pending` | manager | 推荐保留失败任务审计，创建新 task 重试 |
| `completed` | 无 | `completed` | system | 终态，不允许修改为运行态 |
| `cancelled` | 无 | `cancelled` | system | 终态，不允许修改为运行态 |

`blocked` 的解除必须显式发生：approval approved、manager reassign、manager cancel，或 worker/manager 提交新的 status update。不要通过普通 message 隐式解除阻塞。

approval 与 task 的关系：

- approval `approved`：关联 task 从 `blocked` 回到 `in_progress`；
- approval `rejected`：关联 task 保持 `blocked`，`resolution` 写入拒绝原因；
- approval `cancelled`：关联 task 保持当前状态，由 manager 后续处理。

依赖未完成导致不可领取时，不需要把 task 写成 `blocked`；它可以保持 `pending` 或 `assigned`。只有 worker 已经开始执行后遇到外部等待，才进入 `blocked`。

---

## 8. IM 可见性与人工干预

### 8.1 可见性原则

所有对人有意义的 team 事件都应投影到 room：

- team enabled；
- task created / tasks batch created；
- task assigned；
- task claimed；
- progress update；
- task blocked；
- approval requested；
- approval resolved；
- task completed；
- task failed；
- task cancelled；
- member idle/busy/offline 的重要变化。

内部细节不投影：

- cursor；
- ack；
- heartbeat 高频续租；
- store snapshot；
- retry bookkeeping。

### 8.2 Room message 形态

MVP 可以先使用普通 Markdown 文本，不要求所有 channel 支持交互卡片。

示例：

```text
[team] Task created: T-102 Review release notes
owner: @manager
status: pending
depends_on: T-101
```

```text
[team] Manager created 3 tasks:
- T-101 Collect feedback -> @alice
- T-102 Analyze priority (depends: T-101)
- T-103 Generate report (depends: T-102)
```

```text
[team] @alice claimed T-102
status: in_progress
```

```text
[approval] @alice requests approval for T-102
kind: command
summary: Run go test ./...
Reply in this room with: approve T-102 or reject T-102 <reason>
```

对内建 Web UI，可以额外识别结构化 `Message.Kind=event` / `EventPayload.Key` 做更好的展示；对 Feishu/Matrix，普通文本也可读。

### 8.3 人工干预入口

人工干预优先基于普通 message，而不是专用 UI：

- `approve T-102`：批准审批；
- `reject T-102 reason...`：拒绝审批；
- `cancel T-102`：取消任务；
- `reassign T-102 @bob`：重新分派；
- `pause team` / `resume team`：暂停或恢复调度；
- `status`：请求 manager 汇总当前任务状态。

第一版不做完整自然语言解析，但应实现固定格式文本命令解析，保证 Feishu/Matrix 用户不依赖 CLI/API 也能完成最小干预。MVP 至少支持：

```text
approve <task_id>
reject <task_id> <reason>
cancel <task_id>
reassign <task_id> <participant_id>
```

这些命令只在 team room 内生效，并且必须经过权限检查。无法解析或权限不足时，应投影一条可读的失败说明。更复杂的自然语言命令、按钮和卡片可以作为后续增强。

### 8.4 是否需要单独 Team UI

需要，但不是 MVP 的前置条件。

推荐顺序：

1. MVP：ConversationPage 中展示普通 room messages + 基础 structured event；
2. POC 增强：在现有 room 右侧增加 Team sidebar，显示 tasks / approvals / members；
3. 未来：独立 Team workspace，用于过滤、审计、批量操作和高级调度。

这样即使用户在 Feishu/Matrix 中，也不会失去 Agent Teams 的基本能力。

---

## 9. API 设计

MVP API 保持小而稳定。

```text
GET    /api/v1/teams
POST   /api/v1/teams
GET    /api/v1/teams/{team_id}
PATCH  /api/v1/teams/{team_id}
POST   /api/v1/teams/{team_id}/pause
POST   /api/v1/teams/{team_id}/resume

GET    /api/v1/teams/{team_id}/tasks
POST   /api/v1/teams/{team_id}/tasks
POST   /api/v1/teams/{team_id}/tasks/batch
GET    /api/v1/teams/{team_id}/tasks/{task_id}
PATCH  /api/v1/teams/{team_id}/tasks/{task_id}
POST   /api/v1/teams/{team_id}/tasks/{task_id}/assign
POST   /api/v1/teams/{team_id}/tasks/{task_id}/claim
POST   /api/v1/teams/tasks/claim-next
POST   /api/v1/teams/{team_id}/tasks/claim-next

GET    /api/v1/teams/{team_id}/approvals
POST   /api/v1/teams/{team_id}/approvals
POST   /api/v1/teams/{team_id}/approvals/{approval_id}/resolve

GET    /api/v1/teams/{team_id}/presence
POST   /api/v1/teams/{team_id}/presence
GET    /api/v1/teams/{team_id}/events
```

`POST /api/v1/teams/{team_id}/pause` 和 `resume` 是 `PATCH /teams/{team_id}` 的显式语义化快捷入口，分别把 `TeamMeta.Status` 置为 `paused` 和 `active`，便于 CLI 和固定文本命令共用。

`POST /api/v1/teams/tasks/claim-next` 用于未指定 team 的领取。服务端按 `participant_id` 查找 participant 所属 active teams，筛选可领取任务并按统一优先级排序；如果无法得出唯一结果或策略不支持跨 team claim，应返回要求指定 `team_id` 的错误。

`POST /api/v1/teams` 建议支持两种模式：

1. 基于已有 room 启用 team：

```json
{
  "channel": "csgclaw",
  "room_id": "room-123",
  "lead_participant_id": "manager"
}
```

2. 创建新 room 并启用 team：

```json
{
  "channel": "csgclaw",
  "title": "release team",
  "lead_participant_id": "manager",
  "member_participant_ids": ["alice", "bob"]
}
```

`POST /api/v1/teams/{team_id}/tasks/batch` 用于一次创建同一个 team 内的多个 task，并在请求内解析依赖：

```json
{
  "tasks": [
    {
      "id_ref": "collect",
      "title": "Collect feedback",
      "assign_to": "alice"
    },
    {
      "id_ref": "analyze",
      "title": "Analyze priority",
      "depends_on_refs": ["collect"]
    },
    {
      "title": "Generate report",
      "depends_on_refs": ["analyze"]
    }
  ]
}
```

规则：

- `id_ref` 只在本次请求内有效，不能持久化；
- `depends_on_refs` 只能引用同一请求内的 `id_ref`；
- 如果任何 task 校验失败或依赖引用不存在，整批请求失败；
- 服务端返回真实 task id 与 `id_ref` 的映射；
- 投影默认合并为一条 batch summary，避免 room 被多条 task-created 消息刷屏。

`POST /api/v1/teams/{team_id}/presence` 的语义是 upsert：

- key 为 `team_id + participant_id`；
- worker heartbeat 可以重复提交同一状态；
- 高频 heartbeat 不要求每次落盘；
- 服务端可以返回当前 presence 和建议的下一次 heartbeat 间隔。

---

## 10. CLI / Skill Contract

`csgclaw-cli` 是 manager/worker runtime 最现实的结构化入口。

### 10.1 环境与输出约定

CLI 复用现有 participant 身份环境变量：

```bash
PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID=alice
```

执行任务期间，`team_id` 从 `claim` / `claim-next` / `task list` 的响应或显式 `--team` 参数获取。

CLI 默认根据 stdout 自动选择输出：

- stdout 是 terminal：输出 table 或简洁文本，方便人读；
- stdout 被 pipe 或脚本消费：输出 JSON，方便 agent 和脚本解析；
- 需要显式覆盖时使用 `--output json|table`。

`--participant-id` 参数默认可由 runtime 从 `PICOCLAW_CHANNELS_CSGCLAW_PARTICIPANT_ID` 读取，调试或 manager 代操作时也应显式传入 participant ID。这里的 participant ID 是 team 内的稳定身份，例如 `alice` / `p-w-0604`，不是 `u-alice` / `u-p-w-0604` 这类 channel user 或 agent ID。

### 10.2 命令集分层

完整命令集可以按下面方向演进，但 Phase 2 只实现 POC 必需子集，避免把第一版复杂度耗在 CLI 打磨上：

```text
csgclaw-cli team list
csgclaw-cli team create --channel <ch> [--room-id <room>] [--title <title>] --lead-participant-id <participant> [--member-participant-ids <ids>]
csgclaw-cli team pause --team <id>
csgclaw-cli team resume --team <id>

csgclaw-cli team task list --team <id> [--status <status>] [--assigned-to <participant>]
csgclaw-cli team task create --team <id> --title <title> [--body <text>] [--assign-to <participant>] [--depends-on <ids>] [--priority <n>]
csgclaw-cli team task create-batch --team <team> --created-by <participant> --file <tasks.json>
csgclaw-cli team task assign --team <id> --task <id> --participant-id <participant> --actor-id <participant>
csgclaw-cli team task claim --team <id> --task <id> --participant-id <participant>
csgclaw-cli team task claim-next [--team <id>] --participant-id <participant>
csgclaw-cli team task update --team <id> --task <id> --actor-id <participant> --status completed|failed|blocked [--result <text>] [--error <text>] [--reason <text>]
csgclaw-cli team task cancel --team <id> --task <id>

csgclaw-cli team approval list --team <id> [--status pending]
csgclaw-cli team approval create --team <id> --task <id> --requested-by <participant> [--approver-id <participant>] --kind <kind> --summary <text>
csgclaw-cli team approval resolve --team <id> --approval <id> --status approved|rejected [--reason <text>]

csgclaw-cli team presence update --team <id> --participant-id <participant> --state idle|busy|blocked [--summary <text>]
```

Phase 2 必需子集：

- `team create` 或 `team enable-room`；
- `task create-batch`；
- `task claim-next`；
- `task update`；
- `approval create`；
- `approval resolve`。

Phase 2 可选调试命令：

- `task list`；
- `approval list`。

其余命令，包括 pause/resume、assign、cancel、presence update、完整 table/json 输出一致性和复杂筛选，放到 Phase 3b 之后按 POC 反馈补齐。

`task update` 是 worker 汇报执行状态的统一入口：

- `--status completed` 必须带 `--result`；
- `--status failed` 必须带 `--error`；
- `--status blocked` 必须带 `--reason`。

`task cancel` 单独保留，因为它是 manager 或 human 的强制终止动作，不应和 worker 的执行状态汇报混在一起。

`task claim-next` 的 `--team` 可以省略。省略时，服务端可以基于 participant membership 跨 team 搜索可领取任务；如果 participant 属于多个 active team 且无法得出唯一或最高优先级任务，应返回需要显式 `--team` 的错误，而不是猜测。

### 10.3 Manager 行为约束

manager prompt / skill 应明确：

1. 复杂任务优先创建 `TeamTask`，不要只发普通 room 消息；
2. 一次规划出多个 task 时优先使用 batch create，用 `id_ref` 表达请求内依赖；
3. 并行任务拆成多个无依赖 task；
4. 串行任务使用 `depends_on`；
5. 任务创建后需要调整负责人时使用 `task assign`；
6. 需要停止调度时使用 `team pause`，恢复时使用 `team resume`；
7. 汇总时读取 `task list` 和 `approval list --status pending`，而不是只读聊天历史；
8. 指派 worker 后在 room 中只发摘要，权威状态以 Team API 为准。

### 10.4 Worker 行为约束

worker prompt / skill 应明确：

1. 收到 team task 后先 `claim` 或 `claim-next`；
2. `claim-next` 响应中的 `team_id` 和 `task_id` 是后续 update / approval / presence 的上下文；
3. 遇到需要人或 manager 判断时创建 approval；
4. 执行中更新 presence 和 task summary；
5. 完成、失败或阻塞时调用 `task update`，而不是只在 room 中回复“完成了”；
6. 如果 Team API 不可用，才回退到普通 room dispatch。

---

## 11. 持久化

MVP 使用文件存储，保持与当前项目风格一致，但不要把所有 team、task、approval、presence 都塞进一个大 `state.json`。推荐按 team 拆目录，并按对象类型拆 snapshot 文件：

```text
~/.csgclaw/teams/
  index.json
  {team_id}/
    team.json
    tasks.json
    approvals.json
    presence.json
    events.jsonl
```

`index.json` 保存全局索引：

- team id；
- channel；
- room id；
- title；
- lead participant id；
- status；
- team directory path 或可推导路径。

每个 `{team_id}/` 目录保存该 team 的当前快照：

- `team.json`：`TeamMeta`；
- `tasks.json`：当前 task 列表和状态；
- `approvals.json`：当前 approval 列表和状态；
- `presence.json`：成员运行态快照。

`events.jsonl` 保存该 team 的 append-only 审计事件：

- team enabled；
- task created / assigned / claimed / updated；
- approval requested / resolved；
- presence changed；
- projection failed。

第一版 POC 不维护完整 `projections.json` 和投影重试状态机。Phase 1c 只要求把关键 event 写成 room message；发送失败时记录日志并追加 `projection failed` event，不能影响 task/approval 的主状态。完整的 `pending` / `sent` / `failed` / retry / repair 状态机放到外部 channel 投影前再实现，建议在 Phase 4 前作为独立子阶段补齐。

这样拆分的好处：

1. 查看方便：排查任务只看 `tasks.json`，排查审批只看 `approvals.json`；
2. 写入范围小：更新某个 team 不需要重写所有 team 的全局状态；
3. 调试清晰：当前状态在 snapshot 文件，历史变化在 `events.jsonl`；
4. 迁移容易：未来迁移 sqlite/postgres 时，表边界更自然；
5. 跨渠道映射清楚：`index.json` 可以快速从 `channel + room_id` 找到 team。

`presence.json` 需要特殊处理。presence 是高频、偏运行态的数据，不建议每次 heartbeat 都落盘。MVP 可以采用：

- 状态从 `idle` 到 `busy`、`blocked`、`offline` 等重要变化时写盘；
- 普通 heartbeat 只更新内存；
- server 退出或定时 checkpoint 时再刷新 `presence.json`。

stale 检测必须承认异常退出时 heartbeat 可能没有落盘。MVP 需要设置保守边界：

- stale TTL 必须显著大于 presence checkpoint 间隔；
- graceful shutdown 时强制刷新 `presence.json`；
- kill -9 或机器断电后只能依赖最近一次 checkpoint，因此恢复逻辑只把任务标记为 `blocked` 并要求人工 reassign，不做自动重派。

并发控制：

- MVP 使用 `team.Service` 内部 `sync.Mutex`；
- 每次关键状态变更先 append `events.jsonl` 并 flush，再写对应 snapshot 文件；
- snapshot 写入必须使用临时文件 + flush/fsync + atomic rename，不能直接覆盖目标 JSON；
- `events.jsonl` 采用 append-only 写入，写入后 flush；单条事件必须是一行完整 JSON；
- 后续如需更强一致性，再迁移 sqlite WAL。

启动恢复：

1. 读取 `index.json` 和每个 team 的 snapshot；
2. 校验 `events.jsonl` 最后一行是否完整，不完整时截断到最后一个完整 JSON 行；
3. 如 snapshot 落后于完整 event，可按 event 补齐 snapshot；不得出现 snapshot 已提交但 event 缺失的写入顺序；
4. 对 `in_progress` task 检查 `claimed_by` 的 `LastHeartbeatAt`；
5. 如果 worker 已超过 stale TTL，则把 task 标记为 `blocked`，写入 `error` 或 `summary` 说明需要人工 reassign；
6. 对 `pending`、`assigned`、`blocked`、`failed`、`completed`、`cancelled` 状态按 snapshot 恢复，不从 room message 反推 task 状态。

MVP 不做自动 reassign。自动检测 stale 后只标记和投影，避免错误地把仍在执行的 worker 任务分派给其他人。

---

## 12. MVP 实施方案

### Phase 0：文档和协议冻结

目标：

1. 确认 `Team = room + structured metadata`；
2. 确认 MVP 不新增独立 Team UI；
3. 确认 room-visible projection 是跨渠道可用性的硬要求；
4. 冻结 API schema 草案、CLI 命令列表和 `TeamChannelAdapter` 接口签名。

产出物：

- `TeamMeta`、`TeamTask`、`TeamApproval`、`MemberPresence`、`TeamEvent` 的字段草案，以及后续 `TeamProjection` 的候选字段；
- task 状态机和 approval 联动规则；
- `/api/v1/teams` 的最小 API schema 或等价接口草案；
- `csgclaw-cli team ...` 的最小命令列表，包括单 task 创建和 batch task 创建；
- `TeamChannelAdapter` 的 MVP 方法集合。

Phase 0 的重点是减少 Phase 1 返工，不要求实现代码，也不追求把 Phase 4/5 的扩展一次性设计完整。接口和 schema 达到可验证 POC 的“够用”程度后就进入 Phase 1a，后续通过真实实现反馈再修订。

### Phase 1a：纯内存 Team Domain + 状态机

实现：

1. 新增 `internal/team`；
2. 实现 `model.go` 和纯内存 `service.go`；
3. 把第 7.3 节的 task 状态机转换表落成代码；
4. 实现 `CreateTask`、`CreateTasks`、`AssignTask`、`ClaimTask`、`ClaimNext`、`UpdateTaskStatus`、`CompleteTask`、`FailTask`、`CancelTask`；
5. 实现 batch create 的 `id_ref` 解析、依赖校验和全有或全无写入；
6. 实现 ClaimNext 的所有权过滤、依赖过滤和稳定排序；
7. 实现 approval create / resolve 对 task 状态的最小联动；
8. 添加不依赖 IO、API、CLI、IM 的 Go 单元测试。

验收标准：

- task 状态机是否合法；
- 非法状态转换会被拒绝；
- 并发 worker 不能 claim 同一个 task；
- assigned 给其他 worker 的任务不会被 claim-next 抢走；
- depends_on 未完成的 task 不会被 claim；
- batch create 能原子创建带依赖的任务计划，失败时不留下部分 task；
- `depends_on_refs` 只接受同一 batch 内的 `id_ref`，跨 batch 依赖在 MVP 中被明确拒绝；
- `go test ./internal/team/... -race` 通过。

这一阶段不写文件、不接 API、不接 CLI、不投影 room message。它只验证 Team Domain 的行为边界。

### Phase 1b：文件存储 + 启动恢复

实现：

1. 实现 `store.go`；
2. 实现按 team 目录拆分的 `team.json`、`tasks.json`、`approvals.json`、`presence.json`、`events.jsonl`；
3. snapshot 写入使用临时文件 + flush/fsync + atomic rename；
4. `events.jsonl` 使用 append-only 写入，并能截断最后一条不完整 JSON 行；
5. 实现启动恢复逻辑；
6. 实现 stale `in_progress` task 标记为 `blocked`；
7. 实现 presence upsert 和落盘策略。

验收标准：

- 重启后 tasks/approvals/events 状态一致；
- 半写 snapshot 不会破坏上一个完整状态；
- 半写 `events.jsonl` 能恢复到最后一个完整事件；
- 状态变更遵循先 event 后 snapshot 的写入顺序；
- stale worker 的 `in_progress` task 会变为 `blocked`，但不会自动 reassign。

### Phase 1c：内建 Channel Adapter + Projector

实现：

1. 定义 `TeamChannelAdapter` 接口；
2. 实现基于 `internal/im` 的 csgclaw 内建 adapter；
3. 实现 `projector.go`，把 `TeamEvent` 投影为 room-visible message；
4. 投影失败只记录日志并追加 `projection failed` event，不维护完整 retry 状态机；
5. 投影失败不影响 `TeamEvent` 写入；
6. 添加内建 channel 投影测试。

验收标准：

- task created / tasks batch created / claimed / completed 等事件能出现在 room 中；
- 投影失败不破坏 task 状态；
- 不实现 `projections.json`、投影重试队列或人工 repair 命令；
- 不实现 Feishu。

### Phase 2：CLI + ConversationPage 基础展示

前置条件：Phase 1c 完成，内建 projector 已工作。

实现：

1. 注册 `/api/v1/teams` 的最小 API；
2. 新增 `csgclaw-cli team ...`；
3. 支持基于已有 csgclaw room 启用 team；
4. CLI 只实现 POC 必需命令：`team create` 或 `team enable-room`、`task create-batch`、`task claim-next`、`task update`、`approval create`、`approval resolve`；
5. `task list` 和 `approval list` 可作为调试命令加入，但不要求 table/json 双模式完整打磨；
6. 其他命令如 pause/resume、assign、cancel、presence update、复杂筛选和批量操作延后到 Phase 3b 之后补齐；
7. ConversationPage 只做基础展示：普通 room messages + 最小 structured event 识别；
8. 不做 sidebar，不做批量操作，不做复杂筛选。

验收标准：

- CLI `claim-next` 成功后，room 中能看到投影消息；
- CLI `create-batch` 成功后，room 中只出现一条 batch summary；
- manager 可以通过最小 CLI/API 汇总进度和跟进待处理审批；
- pending approval 可以通过 CLI/API resolve；
- ConversationPage 能读懂关键 team event，但仍以 room message 为主。

### Phase 3a：Skill 初稿 + 固定文本命令

实现：

1. 新增或升级 `workspace/skills/agent-teams/SKILL.md`；
2. manager 模板只覆盖 create-batch / list / summarize / approval resolve；
3. worker 模板只覆盖 claim-next / update / complete；
4. 固定文本命令解析可以在 Phase 2 末或 Phase 3a 初完成，Phase 3a 至少支持 `approve`、`reject`，`cancel`、`reassign` 可按 POC 需要补；
5. 保留 `manager-worker-dispatch` 作为 fallback；
6. 只验证最小 happy path：一个 manager、一个 worker、一个串行 task；
7. 不要求一次跑通完整并行、depends_on 和 approval 场景。

验收标准：

- manager 能创建并指派单个 task；
- manager 能用 batch create 提交一个小型串行计划；
- manager 能通过 task list / approval list 汇总进度和待处理项；
- worker 能 claim-next、更新并完成单个 task；
- human 能通过固定文本命令处理 approval 或取消任务；
- skill contract 与 API/CLI 命令保持一致。

### Phase 3b：完整 POC 验收

实现：

1. 按第 13 节的验收标准跑完整场景；
2. 覆盖串行、并行、fan-in、approval 和重启恢复；
3. 记录 POC 中 skill prompt、API、CLI 的不一致点；
4. 只修正影响 POC 正确性的缺口，不在这一阶段扩展 UI 或 Feishu。

验收标准：

- 第 13 节 checklist 全部通过；
- room history 中能看懂关键过程；
- 不打开专门 Team UI 也能完成最小人工干预。

这一阶段形成可运行 POC。

### Phase 4：Feishu Projection

前置条件：Phase 3b 完成，内建 POC 已验证。

实现：

1. 实现 Feishu 的 `TeamChannelAdapter`；
2. Feishu room 中能看到 task/approval/progress；
3. 暂不要求 Feishu 支持卡片或按钮；
4. Feishu 文本消息支持固定格式 approve/reject/cancel/reassign；
5. 投影失败写入 `TeamProjection` 并可重试。

这一阶段验证“基于 IM 基本属性复用”的关键假设。

### Phase 5：Global Tasks 入口

Phase 1/2/3 的 POC 已提前验证一定效果，因此 Phase 5 先调整为：**优先支持左侧全局 `Tasks` 入口，用于跨 room 查看所有 team tasks**。这一阶段的重点是补一个“任务总览工作台”，而不是继续把结构化信息绑在单个 room 内。

`Tasks` 作为全局入口，比 `Teams` 更适合当前目标。因为这一阶段的主问题是“统一查看和跟进任务”，而不是先进入某个 team 再看内部状态。`team` 和 `room` 在这一版中作为任务的归属信息展示，而不是一级导航对象。

#### 5.1 目标

1. 在左侧主导航增加全局 `Tasks` 入口；
2. 用户进入后，可以跨 room 查看所有 team 的任务，而不是只能在单个 conversation 内查看；
3. 页面优先服务 manager / operator / reviewer 的任务跟进场景，快速定位 pending、running、blocked、waiting approval 的任务；
4. 保持现有 room message 主流程不变，ConversationPage 仍然承担具体协作与消息可见性；
5. 第一版只做“查看、筛选、跳转”三类核心能力，不把它扩展成完整 Team workspace。

#### 5.2 推荐范围

1. 左侧新增一级入口 `Tasks`；
2. `Tasks` 页面展示跨 team、跨 room 的统一任务列表；
3. 列表默认按 `parent_id` 聚合：优先展示主任务，再按需展开子任务；
4. 主任务和子任务在视觉上必须可区分；
5. 当前活跃 task、blocked task、pending approval 相关 task 在视觉上应可区分；
6. 点击单个 task 后，右侧 detail 区域或独立 detail 面板展示描述、depends_on、所属 team、所属 room、最新状态摘要和最近关键事件；
7. detail 中应提供“跳转到对应 conversation”的明确入口，方便回到原 room 继续协作；
8. 第一版支持少量高价值筛选，优先考虑：status、assignee、team；
9. 若列表为空，应能区分“系统暂无 task”和“当前筛选条件下无结果”。

#### 5.3 实现切分

1. 导航与路由：
   在 workspace 左侧导航中增加 `Tasks` 入口，并为其提供独立 route；不要把它塞进 ConversationPage 的局部状态里。
2. 数据接入：
   补一个面向全局任务页的最小读取链路。优先做“全局 task list + 单 task detail”；如现有接口只支持按 team 查询，则补一层聚合 API 或 server-side list endpoint。
3. 列表模型：
   前端任务列表项需要稳定包含 task 自身字段以及 `team`、`room` 归属信息；同时需要 `parent_id` 来区分“主任务/子任务”，避免全局页直接平铺细粒度执行项。
4. 筛选策略：
   第一版只做最必要筛选，不做复杂查询构建器。默认排序直接复用服务端规则；若后端已有 priority/status 排序，则前端不要重复发明一套。
5. 详情展示：
   detail 只读优先，先保证状态、负责人、依赖、归属和关键时间字段可见；复杂编辑动作延后。
6. 跳转联动：
   从 task detail 跳回对应 conversation 时，应能直接定位到目标 room；是否自动滚到相关 task 投影消息可后续再增强。
7. 刷新策略：
   第一版允许“页面进入时拉取一次 + 用户手动刷新”；如果后续 team realtime 事件稳定，再加增量刷新。
8. 空态与异常：
   需要覆盖三种情况：系统暂无任务、筛选后无结果、任务加载失败。

#### 5.4 本阶段先不做

1. 不做以 `Teams` 为一级入口的独立 team workspace；
2. 不做 approvals/presence 独立全局页面；
3. 不做 task 的直接 approve/reject、reassign、cancel 等操作；
4. 不做批量操作、拖拽排序、复杂多维筛选；
5. 不要求外部 channel（如 Feishu）同步拥有相同的结构化全局任务页；
6. ConversationPage 内的 team sidebar 延后，作为后续补充而不是本阶段主目标。

#### 5.5 验收标准

1. 左侧主导航中能看到全局 `Tasks` 入口；
2. 进入 `Tasks` 页面后，能跨 room 列出当前系统中的 team tasks；
3. 用户在列表中能直接看出任务属于哪个 team、哪个 room，并区分主任务和子任务；
4. 点击 task 后，能看到该任务的核心结构化信息，而不是只能依赖 room 消息文本；
5. 从 task detail 可以跳回对应 conversation；
6. 普通 conversation 页面在未做 sidebar 的情况下，现有聊天体验不受影响；
7. task 数据加载失败时，页面有明确错误提示，且不影响其他 workspace 功能使用。

#### 5.6 后续自然演进

完成这一版后，再根据使用反馈逐步追加：

1. `Tasks` 页面增加 pending approval / blocked 的快捷视图；
2. 增加 group by team / group by room 视图；
3. 在 task detail 中加入 quick actions，如 approve/reject、reassign、cancel；
4. 再补 ConversationPage 内的 team sidebar，作为单 room 上下文增强；
5. 与 team realtime event 联动刷新。

#### 5.7 推荐实施拆分

为了避免把“全局列表、详情、筛选、联动、实时刷新”捆成一个大任务，Phase 5 建议拆成 3 个连续子阶段。前一个子阶段完成并验证后，再进入下一个。

##### Phase 5a：Global Tasks MVP

目标：先把全局入口和最小可用列表做出来，验证“跨 room 看任务”这条主路径是否成立。

实现：

1. 左侧主导航增加 `Tasks` 入口；
2. 增加独立 route 和基础页面骨架；
3. 提供全局 task list 的最小读取链路；
4. 列表默认展示主任务，并支持展开查看子任务；主/子项都至少显示：title、status、assignee、team、room、priority、updated_at；
5. 补齐 loading、empty、error 三类基础状态；
6. 支持点击任务进入 detail，但 detail 可以先是占位或极简版；
7. 不做复杂筛选、分组、实时刷新。

验收标准：

1. 用户可以从左侧进入 `Tasks` 页面；
2. 页面能跨 room 列出当前系统中的 team tasks；
3. 用户能在列表中直接识别 task 的 team 和 room 归属；
4. 页面在无数据或加载失败时有明确反馈；
5. 普通 conversation 体验不受影响。

##### Phase 5b：Task Detail + Conversation Jump

目标：在能看到全局列表之后，让用户能看懂单个 task，并快速回到对应协作现场。

实现：

1. 补齐 task detail 的结构化展示；
2. detail 至少显示：description、depends_on、status、assignee、team、room、priority、updated_at；
3. 展示最近关键事件或状态摘要，避免用户只能依赖 room 消息文本回溯；
4. 在 detail 中提供跳转到对应 conversation 的入口；
5. 保证从全局任务页回到 room 的路径明确、稳定。

验收标准：

1. 点击 task 后，用户能看到该任务的核心结构化信息；
2. detail 中能明确看出任务所属 team 和 room；
3. 用户可以从 detail 跳回对应 conversation；
4. 不需要打开 room history，也能理解任务当前大致状态。

##### Phase 5c：筛选、视图与刷新增强

目标：在 MVP 路径稳定后，再补提升效率的增强能力，不阻塞前两阶段交付。

实现：

1. 增加少量高价值筛选，优先考虑：status、assignee、team；
2. 增加 pending approval / blocked 的快捷视图；
3. 评估并加入 group by team / group by room；
4. 视情况接入 team realtime event，支持增量刷新；
5. 如 POC 反馈确有需要，再评估 quick actions。

验收标准：

1. 用户可以快速聚焦 blocked、pending approval 或指定 assignee 的任务；
2. 筛选后的列表和 detail 仍保持稳定可用；
3. 刷新策略清晰，不因实时更新破坏页面可理解性；
4. 增强能力不影响 Phase 5a/5b 已有主路径。

### Phase 6：Sideband Control 与高级调度

当 MVP 证明有效后再做：

1. synthetic control event；
2. `internal/teambridge`；
3. worker runtime push，而不是依赖 room message 或 polling；
4. timeout auto-reassign；
5. capability matcher；
6. dedicated Team workspace。

---

## 13. 最小 POC 验收标准

一个可信 POC 至少应完成以下场景：

1. 在已有 room 中启用 team。
2. manager 创建三个任务：A、B、C，其中 C 依赖 A 和 B。
3. 两个 worker 分别 claim A/B 并并行执行。
4. worker A 发起 approval，人工 approve 后继续。
5. A/B 完成后，C 才能被 claim。
6. C 完成后，manager 在 room 中输出汇总。
7. 刷新 server 后，tasks/approvals/events 能恢复。
8. worker stale 后，`in_progress` task 会被标记为 `blocked` 并提示人工 reassign。
9. room history 中能看懂关键过程；不打开专门 Team UI 也能介入。

---

## 14. 与完整 Agent Teams 的演进关系

MVP 不是一次性弱化版，而是未来完整能力的稳定地基：

| MVP                       | 未来增强                                        |
| ------------------------- | ----------------------------------------------- |
| room-scoped team metadata | 多 room / 多 workspace team                     |
| task create/claim/update  | DAG scheduler / auto planner                    |
| room-visible approval     | UI button / channel card / policy approval      |
| presence heartbeat        | richer stale policy / auto reassign             |
| room message projection   | sideband runtime control event                  |
| ConversationPage sidebar  | dedicated Team workspace                        |
| JSON store                | sqlite / postgres                               |

关键是：未来可以增加 Team UI，但不改变“IM 是第一层可见协作界面”的原则。

---

## 15. 推荐代码落点

新增：

- `internal/team/model.go`
- `internal/team/service.go`
- `internal/team/store.go`
- `internal/team/projector.go`
- `internal/team/command_parser.go`
- `internal/api/team.go`
- `cli/team/*`
- `internal/templates/embed/*/workspace/skills/agent-teams/SKILL.md`

集成：

- `internal/api/router.go`：注册 `/api/v1/teams`；
- `internal/server`：初始化 `team.Service`；
- `internal/im/service.go`：必要时增加 team event message helper，但不把 task 状态塞进 IM；
- `web/app/src/pages/ConversationPage`：Phase 2 增加基础投影展示，Phase 5 增加 team sidebar；
- `internal/channel/feishu`：Phase 4 增加 team message projection 和固定文本命令入口。

暂缓：

- `internal/teambridge`；
- 独立 `web/app/src/pages/TeamPage`；
- runtime sideband protocol；
- 自动调度器。

---

## 16. 最终建议

当前最合适的方案不是完整照搬 Claude Code agent teams，也不是继续只靠 `manager-worker-dispatch` skill，而是采用中间路线：

> 以 IM room/member/message 作为统一协作界面，以 server-side task/approval/presence 作为结构化控制面。

这条路线的收益是：

1. 实现复杂度明显低于完整 Claude Code agent teams；
2. 比纯 skill dispatch 稳定，能支持串行、并行、依赖和审批；
3. 可见性和人工干预天然存在于 room 中；
4. Feishu/Matrix 只要支持基础 room/message，就能复用 agent teams；
5. 后续可以平滑演进到 sideband control、Team sidebar、dedicated workspace 和高级调度。

因此，MVP 应优先实现：

1. `internal/team` 轻量 domain；
2. room-scoped team metadata；
3. task 状态机、claim-next、approval 和 presence；
4. 原子文件存储、events 审计和启动恢复；
5. room-visible projection 和固定文本干预入口；
6. `csgclaw-cli team`；
7. manager/worker `agent-teams` skill。

这会形成一个足够简单、可验证、跨渠道可复用的 Agent Teams POC。
