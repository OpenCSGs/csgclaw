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

自由发言房间、私聊和其他任务功能保留独立流程。全局任务看板中的 Room 任务链接转到 `/rooms/<room_id>?task=<task_id>`，可以精确指向父任务或子任务。

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

需要其他 Worker 参与的请求必须通过房间任务规划和正式派发。普通房间消息与 @ 遵循房间路由规则，不代表已经派工；按需房间里的普通 @ 不会启动 Worker。已派发任务内的沟通使用 `room_task_message` 或 `task message`。

正式派工和反馈通过持久化事件核验身份，投递使用稳定事件 ID 去重。

Manager 调用模型生成结构化计划并判断交付质量，服务端负责条件、身份和状态校验。

## 每轮上下文与 CLI 绑定

Codex 在 `thread/start` 和 `thread/resume` 的 `developerInstructions` 中接收 CSGClaw 身份与会话策略。DSH 用运行时插件把同一份策略加入 `systemPrompt`。Manager 和 Worker 分别接收自己的按需规则，普通私聊、自由发言房间与自主协作房间按各自的规则参与。旧版 Team 任务的认领和回报也由运行时提供，只对服务器派发给当前 Worker 的 Team 任务生效。内置模板已移除 `agent-teams` 技能。已加载的 Codex thread 后续直接使用 `turn/start`，不会重复发送 `developerInstructions`；创建、恢复时重新提供完整策略，与 Raft 的原生 thread 注入方式一致。

Manager 策略规定：当前轮存在可信按需激活块时，只要用户要求实际产出、修改、检查、判断或交付结果，就必须先创建或续接房间任务、规划 Worker 子任务并派发；判断与具体领域、工作大小、表面复杂度或 Manager 自身可用工具无关。除澄清范围或评估 Worker 返回结果所需的最少操作外，Manager 不得先自行执行目标工作。独立事项拆成多个子任务并尽量并行。Worker 策略只允许执行本轮明确分配的子任务，并要求先认领、后提交对应批次。

房间轮次出队后，服务端读取当前成员、任务和来源消息。首个独立输入块以 `<csgclaw-runtime-context` 开头，携带 `room_type`、`role` 和 `policy_id`，供本轮选择常驻策略。块内的消息、团队指南、成员描述和任务数据按参考资料使用。系统每轮发送当前事件和回复目标，按变化发送房间范围和任务快照，并定期补发完整事实。团队的“协作规则”在建房时复制为房间的 `collaboration_guide`，作为 JSON 转义的参考数据注入，而非提升为系统权限。首次、规则变更（包括清空）、会话重置、失败后重试及收到压缩事件后的下一轮补发；其余轮次省略未变化的指南，沿用已有每 12 轮完整事实刷新兜底。这是上下文刷新频率，不是协作轮数上限。服务重启后投影缓存为空，恢复首轮发送完整上下文。当前轮缺少房间上下文时按普通请求直接处理。

私聊中 Manager 直接回答和执行。自由发言房间内，Manager 和其他成员按当前消息及各自职责参与。按需房间内，Manager 可以直接处理问候、必要的澄清或状态说明，实际工作走任务规划和派工。

Manager 的私聊、每个群聊分别使用不同的 ConversationKey；按需房间内 Manager 继续共用该房间的一条会话，Worker 则按 Room + Task 隔离执行会话。上下文投影状态也按 ConversationKey 保存摘要，不保存完整提示词副本；不同房间、不同 Worker 任务互不复用。房间成员或任务状态变化时只更新对应摘要，普通本轮来源独立发送。

首次空房间可直接 submit/plan。创建父任务时任务尚不存在，因此 `task submit` 需要当前 `room_id`；创建成功后，任务已经记录 `assignment_type` 和 `assignment_id`。需要完整输入或交付内容时使用 `task get --task <id>`，后续任务命令也只传全局唯一的 task ID，由 CLI 解析对应 Room、Team 或 Agent 归属。

`make build` 构建 `bin/csgclaw` 及 `bin/csgclaw-cli`。运行时 `AGENTS.md` 中的按需策略使用 Profile 注入的 companion CLI 路径生成经过 shell 安全引用的命令。

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
csgclaw-cli task submit --room <room_id> --source-message <source_id> --title "<目标>" --body "<要求与交付标准>"

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
csgclaw-cli task claim --task <child_id> --attempt <attempt>
csgclaw-cli task update --task <child_id> --attempt <attempt> --status completed --result "<交付物与完成情况>"

# Manager 验收后，按当前计划安排后续工作。
csgclaw-cli task review --task <child_id> --attempt <attempt> --accept --result "<验收意见>"
csgclaw-cli task dispatch --task <next_child_id>

# 任一参与者可反馈已有交付与当前要求的差异，由 Manager 协调。
csgclaw-cli task message --task <child_id> --message-id <feedback_id> --body "<涉及的任务、要求差异、证据及当前影响>"

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

按需运行时从服务端私有上下文取得当前房间、当前成员和已有任务；Manager 不需要在每轮开始时列举房间或成员。只有创建父任务以及 `context`、`retry-delivery` 这类房间级操作需要 `--room`，已有任务的归属由 task ID 解析。服务端继续校验 Manager、Worker assignee、执行批次和任务状态，但不再根据同一 Agent 的全部活跃房间限制全局 API。这是避免误操作的工作流约束；持有服务全局凭据的宿主进程仍有管理员权限。

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


## 可选自主协作与上下文分层

创建团队任务时可选择“自主协作”，对应 `POST /api/v1/rooms` 的 `type: "autonomous"`；CLI 支持 `csgclaw room create --type autonomous`。默认仍为 `on_demand`，空类型和已有 `free` 房间继续按原规则投递，不自动迁移或回放历史。

自主房间没有按需房间的 Manager/Worker 调度角色。人和 Agent 的公共消息都能通知其他成员，发送者不通知自己，工具活动、停止记录和空结果不触发成员。成员根据职责、指南与公共历史协商认领工作；认领是协作约定，不是按需任务的原子 claim 或文件锁。需要强制依赖、验收和独占派工时继续使用按需模式。

程序分别维护长期身份与当前场景：

- Codex 的 `developerInstructions` 与 DSH 的 `systemPrompt` 接收程序生成的 Agent ID、名称和会话策略。
- 每轮输入携带场景和当前事件。发送者、结构化提及和回复目标来自服务端记录，消息线程里的回复沿用原线程。
- `self` 包含当前名称、职责，以及有记录时的创建者和最近命名者。新建、改名从 API 的已认证调用者或本地当前用户取得操作者，保存在 Agent 记录的 `created_by`、`named_by`；旧 Agent 不猜测回填，未改名的重复保存不覆盖来源。
- 上下文投影沿用会话隔离、变化摘要、周期性全量刷新与重置语义。自主模式只附当前成员、主对话或话题的未读数量和序号，不再推送正文或最近 12 条消息摘录。正文由 Agent 主动读取。成员的私有执行记录、个人记忆和其他房间不会混入未读正文。

自主成员先通过 `messages_read` 读取当前范围的未读消息，完整收到一页后用 `messages_ack` 确认 `ack_token`；也可使用 `csgclaw message read --unread --room-id <room> --participant-id <self>`，CLI 在整页 JSON 写入 stdout 成功后确认。`has_more` 表示还有下一页，`--limit` 控制每页数量（默认 50，最多 100），`--peek` 只预览，不推进游标。未确认页面会在五分钟内保留原来的截止序号和 token，重试不会跳过该页；过期后可以重新读取。确认表示收到，不表示工作已完成。Agent 认证只能确认自己的游标，本地管理调用必须明确指定成员。

主对话与各话题独立记录读取进度。话题读取用 `thread_root_id` / `--thread-root-id`，同时返回根消息上下文；读取话题不会消费主对话的未读。`messages_list` 用于不消费游标的历史预览。自主协作中的主动发言、读取未读和确认都绑定当前房间及线程。Codex 原生工具依据当前回合的会话绑定校验范围，并补全省略的线程 ID。CLI 和没有回合标识的工具依据唯一的活跃会话定位，CLI 按返回的实际范围确认。Agent 有多个活跃会话时，请用绑定到回合的工具处理未读或主动发言。重试发送复用 `client_message_id`。

成员根据职责和未读内容决定是否接话。成功完成的最终答复会作为普通消息发回对应房间或原线程，其他成员可以读取并接着讨论。同一轮已在同一会话发送的相同答复会去重。工具与中间执行过程记录在活动面板。没有内容需要发布时，成员可以用 `<csgclaw-silent/>` 结束这一轮。需要用户处理的结构化问题和审批继续可见。

服务端在出队、调用 Runtime 前持久化本次提供的未读水位。同一成员同一范围忙碌时，待处理通知合并成一个；出队后检查最新未读，因此模型运行期间已读完的待处理通知不会再启动模型。同一批未读即使未确认或成员沉默，也不会反复唤醒。新公开消息触发通知，不需要空闲轮询。成员仍可并行，此实现没有引入整房间单成员执行锁或运行中强制 steer。

移除原来的整房间共享 16 回合限制。`autonomous_state` 持久化消息序号、各成员/话题的 `read_through`、`notified_through`、待确认页和暂停状态；旧 `receipts` 仅用于迁移时防止已准入消息重放，不再计数。消息序号保存在会话 JSONL 中，不依赖数组下标。停止某个自主成员仍暂停房间后续准入；已运行成员可完成当前轮次，发送新的人类消息恢复。前端只显示自主协作或已暂停，不再显示轮次耗尽。上述字段由服务端管理，创建和更新房间请求不能设置。

新自主房间沿用房间级共享工作目录，各成员保持自己的模型、工具与权限。按需任务接口仍只接受按需房间，自主模式不绕过原有成员和工具权限检查。
