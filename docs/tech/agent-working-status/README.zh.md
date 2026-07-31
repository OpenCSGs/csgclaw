# Agent“工作中”状态实现

## 一句话说明

“工作中”表示某个 Agent 正在处理当前会话里的请求。它不是在线状态，也不是任务系统里的 `busy`，而是一段会自动过期的实时状态。

整个链路可以概括为：

```text
Agent 开始处理请求
        │
        │ 创建并定期续租
        v
worklease.Registry ── participant.work.updated ──> SSE ──> Web“工作中”提示
        ^
        │
        │ 正常结束时主动释放；异常退出时自动过期
        └─────────────────────────────────────────────────────────────
```

## 为什么使用短租约

如果只发送“开始”和“结束”两个事件，Agent 崩溃、容器退出或网络中断时，“结束”事件可能永远到不了，Web 就会一直显示“工作中”。

短租约解决了这个问题：

1. Agent 开始处理请求时创建一个租约，默认有效期为 15 秒。
2. 处理期间每 5 秒使用同一个 `lease_id` 续租。
3. 正常完成后主动释放租约。
4. 如果 Agent 无法继续续租，服务端和 Web 都会在租约到期后自动清除状态。

每次请求使用独立的 `lease_id`。同一个 Agent 可以同时处理多个请求，只要当前房间里还有一个有效租约，Web 就继续显示“工作中”。这样，一个请求结束时不会误清另一个仍在运行的请求。

## 一次请求如何变成“工作中”

1. Runtime 接受请求后生成一个 UUID 作为 `lease_id`。
2. Runtime 调用 `PUT` 创建租约，并提交房间、请求和可选的话题信息。
3. 服务端确认 participant 存在、处于 active 状态，并且仍是该房间的成员。
4. 服务端把租约保存在内存中，并通过 SSE 发布 `participant.work.updated`。
5. Web 按“房间 → participant → 租约”保存状态并显示“工作中”。
6. Runtime 完成请求后调用 `DELETE`；如果没有成功调用，租约也会自动过期。

服务端发布的状态和原因如下：

| 状态 | 原因 | 含义 |
|---|---|---|
| `working` | `started` | 创建了新租约 |
| `working` | `renewed` | 已有租约完成续期 |
| `idle` | `released` | Runtime 主动释放租约 |
| `idle` | `expired` | 租约因未续期而过期 |

## Runtime 接入

### Codex

Codex 的接入位于 `internal/channelbridge/codexbridge/bridge.go`。处理 turn 前创建租约，处理期间每 5 秒续租，并通过 `defer` 覆盖成功、失败和提前返回等结束路径。

### OpenClaw

OpenClaw 的上报逻辑位于 `openclaw-csgclaw-extension`。入站消息通过准入并记录 session 后，一个租约会包住整次 reply dispatch：

- 进入实际回复流程前开始上报。
- 使用独立定时器每 5 秒续租，不依赖 `typingMode`。
- 正常回复、可见的失败回复和 abort 清理全部结束后，再释放租约。
- 首次上报和续租不会阻塞回复；最终释放会等待清理请求结束。
- 上报失败只记录日志，不会让 Agent 回复失败。
- Runtime 通过工作状态阶段区分模型等待、真实 reasoning、工具执行、工具结果处理和正文生成；旧服务端不支持阶段字段时会自动降级到原有状态格式。

当前内置 worker 模板使用 `20260717.27-csgclaw`，已经包含基于 reply dispatch 的实现。

## HTTP 接口

容器内 Runtime 使用服务端 access token 调用以下接口：

```http
PUT    /api/v1/channels/csgclaw/participants/{participant_id}/work-leases/{lease_id}
PATCH  /api/v1/channels/csgclaw/participants/{participant_id}/work-leases/{lease_id}
DELETE /api/v1/channels/csgclaw/participants/{participant_id}/work-leases/{lease_id}
POST   /api/v1/channels/csgclaw/participants/{participant_id}/work:stop
```

`PUT` 同时用于创建和续租。请求体示例：

```json
{
  "room_id": "room-id",
  "thread_root_id": "optional-thread-root-id",
  "request_id": "message-or-request-id",
  "kind": "agent_turn",
  "ttl_seconds": 15
}
```

`ttl_seconds` 可省略，默认是 15 秒；服务端会把显式值限制在 5～60 秒。对同一个 `lease_id` 续租时，participant、房间和请求等元数据必须保持一致。

`PATCH` 上报单调递增的状态序号和可选阶段。阶段只来自 Runtime 可验证的事件：

| 阶段 | 页面文案 | 触发事件 |
|---|---|---|
| `preparing_reply` | 正在准备回复 | 租约创建或新一轮模型输出开始，但还没有 reasoning/正文 |
| `thinking` | 正在思考 | 收到非空 reasoning stream；空回调继续保持“正在准备回复” |
| `running_tool` | 正在执行/调用工具 | 工具或 work item 开始 |
| `generating_reply` | 正在生成回复 | 收到非空 final partial reply |

阶段通过 `work_stage_v1` capability 协商；`thinking` 正文仍通过 `thinking_status_v1` 上报。显式 `thinking` 阶段必须同时携带非空正文，最后一段正文会保留到工具执行或最终回复开始。工具结束后 Runtime 只切换回无细分阶段的 `thinking` phase，Web 在下一段 reasoning 或最终回复到达前继续展示最近的具体工具调用。服务端保留原来的 `working/thinking` phase，供旧 Runtime 和旧 Web 兼容使用。

停止能力通过 `turn_stop_v1` capability 协商。`POST work:stop` 只先持久化 `stop_requested`，再由 `TurnControlDispatcher` 调用 Runtime 的可选 `TurnController`：Codex bridge 取消对应 Prompt context，并由 app-server 执行 `turn/interrupt`；OpenClaw adapter 当前通过 participant SSE 投递。投递失败和超时分别上报 `stop_failed`、`stop_timed_out`，不会伪装成已经停止。

Runtime 结束租约时，`DELETE` 可提交 `{"outcome":"released"}`、`{"outcome":"stopped"}` 或 `{"outcome":"stop_timed_out"}`。只有 Runtime 在处理停止请求后显式确认 `stopped`，服务端才发布 `stopped`；旧 Runtime 省略请求体时按普通 `released` 处理。

Runtime 显式确认 `stopped` 后，服务端会用对应 Agent 的用户身份在原房间（有话题时在原话题）写入一条 “Conversation interrupted” 记录。仅投递停止请求、投递失败或停止超时都不会写入该记录。同一 `lease_id` 使用固定消息 ID，重复完成请求不会产生重复记录；该内部控制记录只进入会话历史和 Web 实时流，不会再次分发给任何 Agent。

## 服务端如何避免脏状态

核心实现在 `internal/worklease/`：

- Registry 只在内存中保存活动租约；只有 Runtime 显式确认 `stopped` 时，统一完成路径才会额外写入上述会话记录。
- janitor 每秒清理一次过期租约。
- 租约释放或过期后保留 70 秒 tombstone；迟到的续租会收到 `410 Gone`，不能重新激活已经结束的请求。
- 每个服务进程都有独立的 `registry_epoch`。服务重启后，Web 收到新 epoch 会丢弃旧进程的状态。
- 每个租约都有递增的 `revision`，Web 用它忽略重复或乱序事件。

租约接口和 SSE 转发分别接入 `internal/api/participant_work.go`、`internal/api/handler.go` 与 `internal/server/http.go`。

## Web 如何展示

`useParticipantWorkStatus` 维护以下结构：

```text
room_id -> participant_id -> lease_id -> expires_at
```

Web 收到 SSE 后新增、续期或关闭对应租约，并根据 `expires_at` 设置本地定时器。即使 `released` 或 `expired` 事件丢失，页面也不会永久显示“工作中”。

Web 还会在 participant 被更新或删除、房间被删除、成员离开房间时清理相关状态。主要实现位于：

- `web/app/src/hooks/workspace/useParticipantWorkStatus.ts`
- `web/app/src/hooks/workspace/participantWorkState.ts`
- `web/app/src/hooks/workspace/useWorkspaceRealtime.ts`

## 明确边界

- “工作中”不等同于 participant presence。
- “工作中”不复用 team task 的 `busy`。
- 租约和 SSE 都是实时状态，不作为 IM 消息持久化。
- 普通回复或单个工具结束，不能清除其他并发请求的租约。
- Runtime 异常退出时，以租约自动过期作为最终兜底。
