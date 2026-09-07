# Room-first Manager 协作：实现与试用

## 使用方式

新建群聊时默认选择 `type=on_demand` 的「按需发言」房间，并自动选择、锁定 Manager 入群；用户再选择 Worker 并提出目标。用户消息和 Worker 的 @协作消息由程序统一接收并转交 Manager，由 Manager 判断后续安排，并负责派工、验收和汇总。`type=free` 表示「自由发言」，仅通知被 @ 的成员；历史房间没有 `type` 时按 `free` 读取。

1. Manager 根据用户目标或自己的协调目标创建父任务，再拆出一层 Worker 子任务。
2. 每个父任务仅在首次生成计划的 Manager 回复的消息操作区左侧显示一次小号「查看图标 + #任务编号」，与复制、回复同排；标题和状态在任务弹窗中展示。
3. 点击后打开简洁状态清单：顶部父任务名称与整体状态，下面子任务名称、负责人和状态。结果和报告在聊天交付。房间任务入口直接分组显示全部清单。
4. Manager 显式派发满足条件的子任务。不同 Worker 的独立工作可异步进行；前置任务尚未验收时，服务端拒绝提前派工。
5. Worker 先认领再执行，提交结果时同时生成 @Manager 通知。`completed` 对 Worker 表示「提交结果」，保存状态为 `pending_review`。
6. 任一参与者或 Manager 发现已有交付与当前要求不符时，由 Manager 判断影响范围，在原父任务下调整工作安排：补充子任务、重新派发或改派，并为新增工作保存依赖关系。任务数量、负责人和后续接收方式按实际需要确定。
7. Manager 发布 `succeeded / issues / failed / stopped` 结论和交付总结。用户始终可从原消息打开对应任务；房间标题区也有完整任务列表。

自由发言房间、私聊和 Team 功能保留独立流程。全局任务看板中的 Room 任务链接转到 `/rooms/<room_id>?task=<task_id>`，可以精确指向父任务或子任务。

## 数据结构与分层

父任务、子任务共用 `Task`，通过可选的 `parent_id` 表达父子关系；核心存储支持递归层级，校验缺失父级、环和跨归属关联。

Room 协作采用两层结构：Manager 创建和维护顶层父任务及计划，Worker 执行一层子任务。

| 模块 | 职责 |
| --- | --- |
| Room / IM | 房间成员、角色、消息、原始发送者和 @对象 |
| `internal/taskcore` | 通用 Task、递归父子关系、ID、状态、追加事件和原子持久化 |
| `internal/roomtask` | Room 计划校验、派工、验收、容量等待、父任务队列、停止与恢复 |
| API / `cli/task` | 统一 Task 命令、任务归属解析、角色和运行范围校验、任务内沟通与操作 |
| Channel / Engine | Manager 按房间会话、Worker 按任务会话；传递任务与执行批次 |
| Web | Manager 消息下的小入口、房间任务弹窗、列表、准确链接与状态同步 |

- Room 任务使用 `assignment_type=room`、`assignment_id=room_id` 和独立 `room_id`；父子属于同一个房间。父任务负责人是 Manager，子任务负责人只能是当前房间 Worker。
- `parent_id` 表达目标拆解；`depends_on` 表达执行前置条件。Room 计划整体校验成员、引用和依赖环后保存。
- `attempt` 标识一次明确派工；认领、结果和验收都必须带当前批次。改派或重新派发会增加批次，服务端校验批次后处理结果和停止确认。
- `review`、`reviewed_by` 保留当前验收意见，追加事件保留历史负责人、本次结果和验收记录。
- Room 使用独立服务和 API DTO；Room、单人任务和 Team 分别消费通用任务核心。
- 每棵任务树的当前状态扁平写入 `tasks/<root-id>/tasks.json`；父任务与后代是 `tasks` 数组中的同级元素，只通过 `parent_id` 关联。
- 审计事件单独追加到 `tasks/<root-id>/events.jsonl`。`tasks.json` 中的 `event_seq` 是提交标记，启动和下次写入会忽略或清理未提交的日志尾部；任务状态与 IM 聊天记录不混存。
- 全局的 `tasks/sequence.json` 只保存数字任务编号的最新值，不保存任务索引；读取和更新某棵任务树不会竞争这个共享文件。

## 消息转交与 Manager 协调

| 输入 | 路由与行为 |
| --- | --- |
| 用户直接发送、@Worker、@多人 | Manager 接收一次，保留原始意图与 @对象 |
| Worker @Manager、@Worker、@多人 | Manager 接收一次，判断后向目标 Worker 转告或派工 |
| Manager 正式派工 | 已保存的派工事件直接唤醒目标 Worker，携带明确任务和批次 |
| Manager 任务内转告 | 唤醒该任务当前执行者 |
| Worker 提交结果、失败或阻塞 | 状态与反馈事件同时保存，Manager 再调用模型判断 |
| Worker 工具日志、思考、未 @的普通回复 | 作为活动记录或聊天内容展示 |
| Manager 最终汇总 | 保存并展示交付总结 |

普通消息、任务内消息、API 和 CLI 共用 IM 路由规则。服务端保存原始发送者、正文和全部 @对象，Manager 决定告知谁、告知什么。

正式派工和反馈通过持久化事件核验身份，投递使用稳定事件 ID 去重。

Manager 调用模型生成结构化计划并判断交付质量，服务端负责条件、身份和状态校验。

## 每轮上下文与 CLI 绑定

宿主 Codex 的 IM 执行器在房间轮次出队后，通过服务端 ContextProvider 自动读取当前成员和任务事实，作为私有输入交给模型。Manager 沿用原房间会话，只补充当前／排队任务的状态、依赖、负责人、批次与必要结果摘要；Worker 只得到自己的任务和前置交付物。原始来源与任务 ID 独立传递。

首次空房间可直接 submit/plan。需要完整输入或交付内容时使用 `task get --task <id>`。命令先通过全局唯一任务 ID 解析 `assignment_type` 和 `assignment_id`，再转到对应 Room、Team 或 Agent 接口；模板与运行时规则使用程序提供的当前房间上下文。

`make build` 构建 `bin/csgclaw` 及 `bin/csgclaw-cli`。运行时使用 Profile 中程序注入的 companion CLI 路径生成经过 shell 安全引用的命令。`CSGCLAW_CLI` 作为内部兼容绑定，由程序维护。

任务计划使用 `task plan --task <id> --plan-file <path.json>`，也支持 `--plan-json`。认领／验收指令通过私有执行上下文传递，`task_feedback` 在聊天中显示成员身份和简短状态提示。详情在任务消息变化和打开入口时请求最新状态，并保留可见房间的兜底轮询；弹窗不提供多余的手动刷新按钮，任务接口使用 `Cache-Control: no-store`。

## 会话、队列与资源

- Manager 会话键按 Binding + Room 建立，子任务 ID 作为上下文传递。同一房间轮次顺序处理，不同房间独立处理。
- Worker 会话按 Binding + Room + Task 建立，同任务的后续沟通继续该任务上下文。执行批次随消息、工具与结果传递。
- 房间可以保存多个父任务；来源消息/Manager 稳定来源 ID 重试返回原任务。最早未结束的父任务占用房间执行位置，其余排队。
- 父任务受阻、等待验收或停止处理中，继续占用队列位置。父任务结束并交付总结后，系统通知 Manager 检查并安排下一项。
- 首版同一 Worker 的 Room 任务执行容量为 1。被占用时保存 `waiting_on_task_id` 和等待原因；释放后通知等待房间的 Manager，由它重新检查和派工。
- 不同 Worker 可并行执行无依赖工作。容量约束作用于 Room 任务。共享目录、文件和端口的使用由 Manager 在计划中通过依赖安排。

任务状态、事件和消息持续保存。模型通过长会话摘要与按需读取获取上下文，任务关联以明确的任务 ID 为准。

## 停止、成员与恢复

- Worker 只有认领当前有效派工后才能提交结果。Manager 验收通过后子任务成为 `completed`，才可满足其他任务的依赖。验收不通过则受阻，由 Manager 重新派发原任务或改派当前成员。
- 通过 `task stop --task <parent>` 可停止父任务。父任务先进入 `stopping`，撤销未认领任务；只向该父任务当前执行批次的具体租约发送停止请求。
- 停止请求等待执行结束确认，期间保留正在执行状态和等待原因。全部执行结束后由 Manager 汇总停止结论，已保存结果保留。
- Manager 保持为房间协调者；移除 Worker 前，先停止或核实其当前执行。待执行负责人移出房间后任务受阻，历史负责人和产物保留，Manager 可显式改派。
- 服务启动时，在途 Room 任务进入 `recovery_required` 检查。Manager 核实旧执行已结束并检查现有产物后，用 `recover` 记录说明，再决定是否派发新批次。恢复确认要求原执行租约已结束。
- 结果或总结投递失败时，任务事实已保存。`retry-delivery` 按稳定 ID 补发原事件；总结补发成功后也会恢复下一父任务的协调通知。
- Worker 失败后，由 Manager 判断后续工作安排。用户停止则进入停止确认流程。部分交付的 `issues` 状态显示为“已结束（有遗留）”。

## CLI 操作

配套 `csgclaw-cli`、服务和运行指令需要一起更新。下面的占位符需替换为实际任务信息：

```bash
# 根据用户目标创建父任务。
csgclaw-cli task submit --room <room_id> --source-message <source_id> --actor-id <requester_id> --title "<目标>" --body "<要求与交付标准>"

# 将工作拆分和依赖保存为 plan.json。tasks 按实际需要填写。
cat > plan.json <<'JSON'
{
  "summary": "<工作安排>",
  "tasks": [
    {"id_ref": "first", "title": "<第一项工作>", "body": "<输入、交付物与验收要求>", "assigned_to": "<worker_a_id>"},
    {"id_ref": "next", "title": "<后续工作>", "body": "<输入、交付物与验收要求>", "assigned_to": "<worker_b_id>", "depends_on_refs": ["first"]}
  ]
}
JSON
csgclaw-cli task plan --task <parent_id> --plan-file plan.json

# Manager 派发当前可执行任务；Worker 使用派工返回的批次认领并提交。
csgclaw-cli task dispatch --task <child_id>
csgclaw-cli task claim --task <child_id> --actor-id <worker_id> --attempt <attempt>
csgclaw-cli task update --task <child_id> --actor-id <worker_id> --attempt <attempt> --status completed --result "<交付物与完成情况>"

# Manager 验收后，按当前计划安排后续工作。
csgclaw-cli task review --task <child_id> --attempt <attempt> --accept --result "<验收意见>"
csgclaw-cli task dispatch --task <next_child_id>

# 任一参与者可反馈已有交付与当前要求的差异，由 Manager 协调。
csgclaw-cli task message --task <child_id> --actor-id <worker_id> --target <manager_id> --message-id <feedback_id> --body "<涉及的任务、要求差异、证据及当前影响>"

# Manager 需要补充工作时，在原父任务下追加所需子任务。
# 新任务的依赖可引用已有子任务 ID，或同批新增任务的 id_ref。
cat > additional-plan.json <<'JSON'
{
  "summary": "<调整原因与补充安排>",
  "tasks": [
    {"id_ref": "additional", "title": "<补充工作>", "body": "<处理要求、输入与交付标准>", "assigned_to": "<selected_worker_id>", "depends_on_refs": ["<predecessor_task_id>"]}
  ]
}
JSON
csgclaw-cli task plan --task <parent_id> --append --request-id <adjustment_id> --plan-file additional-plan.json

# Manager 按依赖逐项派发和验收。全部必要工作验收、目标达成后结项。
csgclaw-cli task report --task <parent_id> --outcome succeeded --result "<最终交付与结论>"
csgclaw-cli task retry-delivery --room <room_id>
```

补充操作（查询只在缺少具体信息时使用）：

```bash
csgclaw-cli task get --task <child_id>
# 省略 --accept 表示验收未通过；再次 dispatch 可用 --target 指定当前房间其他 Worker。
csgclaw-cli task review --task <child_id> --attempt <n> --result "缺少运行说明，请补充"
csgclaw-cli task dispatch --task <child_id> --target <worker_id>
csgclaw-cli task stop --task <parent_id>
csgclaw-cli task recover --task <child_id> --attempt <n> --result "已核实旧执行退出，检查并保留了已有产物"
```

运行时 CLI 按活跃房间及角色授权查询和操作：Manager 管理父任务和派工，Worker 更新自己的子任务。普通消息也校验调用者与房间。此处是工作流约束，持有服务全局凭据的宿主进程仍有管理员权限。

## 验证与试用

自动测试覆盖 Task 递归持久化、单层 Room 接口、Manager 自建父任务、Worker 权限、提交与验收分离、依赖约束、显式派工、不同 Worker 执行、房间父任务队列、容量释放通知、改派与旧批次、精确停止、恢复检查、投递重试、统一 @转交与去重，以及新弹窗和历史链接。Engine/Binding 测试覆盖会话隔离、异步执行与同会话有序处理。

自动测试验证任务状态、路由和执行控制，交付质量由 Manager 在真实模型工作流中验收。

按项目现有流程重新构建服务、Web 和 CLI，安全结束已有执行后再重启服务及相关运行时。建议新建测试房间验证「提出需求 → 任务入口 → Worker 交付 → Manager 验收 → 后续 Worker 处理 → Manager 汇总」，再验证 Worker @Worker、第二父任务排队和停止恢复。

## 执行中的任务调整与结项

任务执行中的反馈可能来自任一 Worker、交付物的接收者、用户或 Manager。后续参与者使用前置交付物时，可以指出其与当前要求的差异；Manager 也可以在验收和汇总时主动发现需要补充的工作。

Manager 结合原目标、已完成工作、当前反馈和依赖关系，决定具体调整方式：

- 当前子任务的交付需要完善时，记录验收意见，再重新派发该任务或改派合适的房间成员。
- 已验收工作需要补充，或后续处理出现新的必要工作时，在原父任务下追加所需子任务，明确输入、交付要求、负责人及依赖。
- 后续接收、核对或继续处理的工作，根据实际需要一并纳入计划。其承担者由能力和上下文决定，任务数量由工作范围决定。
- 调整后的工作仍按依赖和 Manager 验收推进。Manager 派发当前可执行任务后等待反馈，再判断后续安排。

已有子任务的交付记录与验收历史保留。Manager 分别判断一项工作是否已交付、交付物是否满足后续要求，以及父任务目标是否达成；补充工作持续归属于该父任务。

`POST /rooms/{room}/tasks/{parent}/plan` 接收 `append: true` 和稳定的 `request_id`，其余使用 `summary` 与 `tasks`。`depends_on_refs` 可以引用本批新任务的 `id_ref` 或同一父任务已有子任务的 ID。服务端验证当前成员、两层关系和依赖；新增任务作为一个原子批次保存，由 Manager 显式派工。事件保存请求标识和内容摘要，相同请求重试按已保存的批次处理，不同内容复用标识报错。

追加工作后，父任务从待汇总恢复执行中。Manager 可审阅已提交的结果，包括附有完整交付内容的 failed 报告，并记录验收判断。succeeded／issues 结项要求所有子任务已验收；succeeded 同时要求整体目标达成，issues 表示已结束的部分交付，顶部显示“已结束（有遗留）”。failed／stopped 用于明确终止。

后续补充计划投递 `task_plan_updated` 消息；聊天入口绑定最早的 `task_planned` 消息。模型结果、交接和汇总统一使用 `metadata.task_id` 与可选的 `metadata.task_attempt`，任务弹窗同步显示父子任务编号、最新清单和状态。
