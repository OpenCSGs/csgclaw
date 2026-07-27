# CSGClaw Agent Teams Beta 1

Agent Teams Beta 1 把多 Agent 协作从“聊天里口头派活”升级为“聊天里可见、系统里有状态、任务能自动流转”的协作方式。

核心模型：

```text
Team = 可复用成员池
主任务 = 一次执行上下文
执行房间 = 主任务在某个 channel 里的协作房间
子任务 = 继承主任务执行房间的工作项
```

Team 不再绑定固定 channel 或固定 room。Team 只保存 `lead_agent_id` 和 `member_agent_ids`。每次创建主任务时，调用方选择执行通道，系统在该通道创建执行房间，并把 `execution_channel + room_id` 写到主任务上。

旧的 room-bound Team 数据不兼容，需要重新创建。

---

## 1. 一句话理解

以前的多 Agent 协作更像这样：

```text
manager 在群里 @worker 说一句要做什么
worker 看懂了就去做
做完再回群里说结果
manager 自己记住谁做完了、谁还没做、下一步该给谁
```

Agent Teams 希望变成这样：

```text
用户选择 Team 成员池并提出目标
CSGClaw 创建一个主任务
系统按主任务选择的执行通道创建执行房间
manager 拆成多个子任务
系统把子任务派给 worker
worker 领取、执行、回报
系统自动推进后续任务
所有子任务完成后，主任务自动汇总完成
```

核心变化是：任务进度不再只靠聊天记录和 Agent 自觉记忆，而是由 CSGClaw 持续记录和推进。

---

## 2. 主要概念

### 2.1 Team

Team 是一个可复用的小队成员池。一个 Team 里通常包含：

- 一个 manager：保存在 `lead_agent_id`，负责理解目标、拆任务、协调执行。
- 多个 worker：保存在 `member_agent_ids`，负责执行子任务。

Team 不保存 `channel`，也不保存 `room_id`。这使同一个 Team 可以在不同执行通道中复用，例如 Web 默认使用 `csgclaw`，从飞书上下文触发时使用 `feishu`。

### 2.2 主任务

主任务是一次具体执行。它属于某个 Team，并额外保存：

- `execution_channel`：本次任务使用的执行通道，如 `csgclaw` 或 `feishu`。
- `room_id`：本次任务在该通道中的执行房间。

一个主任务只绑定一个执行通道和一个执行房间。子任务继承父任务的执行通道和房间，不再单独创建房间。

### 2.3 执行房间

执行房间用于集中承载一个主任务的过程：

- 系统在这里通知 worker 有任务可做。
- worker 在这里领取任务、报告进度、提交结果。
- 用户可以进入这里看这个任务的具体执行过程。
- 审批和人工干预信息也投影到这里。

如果多个主任务同时运行，它们各自有独立执行房间，避免派发、阻塞和结果混在一起。

### 2.4 Manager 和 Worker

Manager 是 Team 的负责人。它负责判断目标是否需要拆分、哪些任务能并行、哪些任务有依赖、哪个 worker 更适合做哪个子任务。

Worker 是执行者。它收到明确派发后，需要先领取任务，再执行任务，最后回报结果。

“派发”和“领取”是两个状态：

- 已派发：系统已经通知 worker。
- 进行中：worker 已经领取并正式接手。

区分这两个状态后，系统能判断问题到底是“还没轮到 worker”，还是“已经通知但 worker 没响应”。

### 2.5 状态和时间线

每个任务都有结构化状态。房间消息是给人看的过程记录；任务状态是系统推进流程的依据。

常见状态包括：

```text
待开始 -> 已派发 -> 进行中 -> 阻塞 -> 已完成 -> 失败
```

任务事件会带上 `channel` 和 `room_id`，投影器按事件通道选择对应 adapter，把事件写入主任务执行房间。

---

## 3. Channel 选择规则

### 3.1 Web

Web 创建 Team 时只选择成员，不创建房间。

Web 创建主任务时选择执行通道，默认是 `csgclaw`。提交后：

1. `POST /api/v1/teams/{team_id}/tasks/batch` 携带 `execution_channel`。
2. 服务端创建主任务。
3. 服务端按 `execution_channel` 创建执行房间。
4. 服务端把 `execution_channel + room_id` 绑定到主任务。

### 3.2 Manager / 内置 skill

manager 触发主任务时应读取当前 channel context：

- 当前在 CSGClaw 内建 IM 中：传 `--execution-channel csgclaw`。
- 当前在 Feishu 中：传 `--execution-channel feishu`。

不要把 channel 写到 Team 上，也不要复用某个旧 Team 房间。

### 3.3 CLI

创建 Team：

```bash
csgclaw-cli team create \
  --title "Beta 1 Team" \
  --lead-agent-id u-manager \
  --member-agent-ids u-worker-a,u-worker-b
```

创建主任务：

```bash
csgclaw-cli team task create-batch \
  --team-id team_xxx \
  --execution-channel csgclaw \
  --created-by manager \
  --task 'A|Prepare rollout|worker-a|8'
```

`--execution-channel` 默认为 `csgclaw`。从 Feishu 上下文触发时应显式传 `feishu`。

---

## 4. API 模型

### 4.1 Team

Team response:

```json
{
  "id": "team_123",
  "title": "Beta 1 Team",
  "lead_agent_id": "u-manager",
  "member_agent_ids": ["u-worker-a", "u-worker-b"],
  "status": "active"
}
```

Team 不包含 `channel`、`room_id` 或 room members。

Create Team request:

```json
{
  "title": "Beta 1 Team",
  "lead_agent_id": "u-manager",
  "member_agent_ids": ["u-worker-a", "u-worker-b"]
}
```

### 4.2 Task

主任务 response 包含执行上下文：

```json
{
  "id": "task_123",
  "team_id": "team_123",
  "execution_channel": "csgclaw",
  "room_id": "room_456",
  "title": "Prepare Beta 1 review",
  "status": "pending"
}
```

子任务继承父任务的 `execution_channel` 和 `room_id`。

Batch create request:

```json
{
  "execution_channel": "csgclaw",
  "created_by": "manager",
  "tasks": [
    {
      "id_ref": "A",
      "title": "Draft rollout note",
      "assign_to": "worker-a",
      "priority": 8
    }
  ]
}
```

`execution_channel` 为空时默认 `csgclaw`。

---

## 5. 执行房间创建

创建主任务时，服务端立即创建执行房间。

房间成员来自：

1. Team lead agent。
2. Team member agents。
3. 子任务显式 assignee。

创建 Feishu 执行房间时，Team 成员必须已经有对应 Feishu participant 绑定；缺失绑定时请求会返回明确错误。内建 `csgclaw` adapter 会继续使用本地 participant/user 规则。

子任务创建、规划、派发、认领、完成、审批都使用父任务执行房间。

---

## 6. Web 页面行为

### 6.1 Team 页面

Team 页面展示成员池：

- Team 名称和状态。
- Lead agent。
- Member agents。
- Team 下的主任务。

Team 页面不展示固定房间，也不通过添加 room member 修改 Team。后续如果需要编辑成员，应走 Team 成员 API，而不是 room member API。

### 6.2 Tasks 页面

Tasks 页面第一层是主任务列表和主任务看板。

主任务卡片应优先展示：

- 执行通道。
- 执行房间。
- 子任务进度。
- 阻塞/失败数量。
- 依赖摘要。

进入主任务后展示子任务看板和依赖关系。事件时间线放在任务详情中。

### 6.3 执行房间

执行房间不是替代任务看板，而是补充任务看板：

- 看板适合扫状态。
- 详情适合看结构化时间线。
- 执行房间适合看 worker、系统和人类之间的具体交流。

---

## 7. 当前 Beta 限制

1. 不兼容旧 Team room-bound 数据。
2. 单个主任务只绑定一个执行通道和一个执行房间。
3. Team 可跨 channel 复用，但执行时要求成员已有对应 channel participant 绑定。
4. 执行房间归档、清理、重命名、重新绑定等能力还需要补。
5. 审批 UI、worker 超时、重试和恢复还需要继续增强。

---

## 8. 验收标准

这版功能至少应该满足：

1. 创建 Team 不新增房间。
2. Web 创建主任务默认创建 CSGClaw 执行房间。
3. CLI/manager 能通过 `execution_channel` 指定 `csgclaw` 或 `feishu`。
4. Feishu 上下文创建主任务时创建 Feishu 群。
5. 子任务继承父任务的执行通道和执行房间。
6. 任务派发、领取、完成、阻塞、审批事件投影到主任务执行房间。
7. `FindTeamByRoom(channel, room_id)` 能通过主任务执行房间反查 Team 和主任务。
8. 用户能在看板、详情和执行房间里看懂当前进度。
