# Agentic 前端接入会话提问

Genius 创建知识库助手时，可能需要用户选择资料来源或补充说明。Agentic 的 `CsgBotChat` 需要在发送消息时声明支持表单，处理 SSE 中的 `request_user_input`，并将答案提交给指定问题。收到的问题含有 `detached` 字段，它决定从哪个 HTTP 请求读取后续回复。

在 `csghub-portal` 中，`frontend/src/api/agents/geniusSandbox.js` 负责发送请求，可在现有 `respond` 方法传入 `user_input` 并增加提交答案的方法。`frontend/src/components/agents/CsgBotChat.vue` 负责解析 SSE 和提交答案，`CsgBotChat/AssistantMessage.vue` 负责显示表单。原生提问发生时 `isGenerating` 仍为 `true`，此时应允许提交当前表单，原回复继续读取。

## 1. 发送消息

在 Genius 客户端现有的 `POST /responses` 请求体中添加 `"user_input":true`。该字段默认为 `false`。使用 `multipart/form-data` 时，添加表单字段 `user_input`，值为字符串 `"true"`。

下面的示例需要两个终端。在两个终端中均设置这些变量。`ENDPOINT` 取所选实例的 `endpoint`，`AGENT` 取实例配置的 Agent 名称，`TOKEN` 取该实例的网关凭证。浏览器构造 URL 时应对每个动态路径参数使用 `encodeURIComponent`。

启用实例服务认证时，设置 `user_input: true` 发送消息、查询待回答的问题和提交答案都需要该实例的有效服务凭证。Portal 对这三个请求使用与附件访问相同的实例地址和网关授权方式。缺少凭证或凭证无效时，接口返回 `401 unauthorized`。仅发送普通文本时，省略 `user_input` 的 JSON 请求支持匿名访问。

```sh
ENDPOINT='https://aigateway.opencsg-stg.com/v1/sandboxes/<sandbox-name>'
AGENT='codex-worker'
SESSION_ID='knowledge-assistant-demo'
SESSION_URL="${ENDPOINT%/}/api/v1/agents/$AGENT/sessions/$SESSION_ID"
```

在第一个终端执行以下请求，保持连接打开。

```sh
curl --no-buffer --request POST "$SESSION_URL/responses" \
  --header "Authorization: Bearer $TOKEN" \
  --header 'Content-Type: application/json' \
  --data '{"input":"请使用 request_user_input 询问我要导入哪种知识库资料，收到答案后继续。","stream":true,"user_input":true}'
```

## 2. 处理 SSE 并显示表单

在聊天页面现有的 SSE 解析函数中增加 `request_user_input` 分支。下面是一组示例数据。实际问题的 ID、选项和时间由服务端提供。

```text
event: request_user_input
data: {"type":"request_user_input","interaction_id":"question-example","status":"pending","detached":false,"requested_at":"2026-11-05T09:38:45Z","questions":[{"id":"source","header":"资料来源","question":"要导入哪种知识库资料？","options":[{"label":"PDF","description":"上传文档"},{"label":"网页","description":"提供网址"}]}]}

```

- 把表单与当前实例、Agent、会话和 `interaction_id` 一起保存。收到同一个 ID 时更新已有表单。保留该事件前已经显示的文字，继续读取 SSE。
- 按 `questions` 的顺序显示 `header`、`question` 和 `options`。显示每个选项的 `label` 和可选的 `description`。`options` 为空或 `is_other: true` 时提供文本输入框。`is_secret: true` 时隐藏用户输入的字符，从页面历史和日志中排除答案原文。
- 如果事件包含 `auto_resolve_at`，可以显示截止时间。页面重新打开或倒计时结束时，根据服务端待回答列表更新表单。
- 保留消息中的 `detached`。对于 `false`，原回复正在等待答案。对于 `true`，原回复可以结束，表单在 `message_stop` 后仍然可以提交。

## 3. 提交答案

在第二个终端使用事件返回的 `interaction_id`。示例选择第一道题的 PDF 选项。

```sh
INTERACTION_ID='question-example'
curl --include --no-buffer --request POST "$SESSION_URL/interactions/$INTERACTION_ID/respond" \
  --header "Authorization: Bearer $TOKEN" \
  --header 'Content-Type: application/json' \
  --data '{"answers":{"source":{"answers":["PDF"]}},"stream":true}'
```

外层 `answers` 的键使用原始 `question.id`。选择选项时提交原始 `option.label`，保留标签中的 ` (Recommended)` 等文字。自由填写的答案使用 `"user_note: 内容"`。如果表单包含多道题，一次提交所有问题 ID。跳过某道题使用 `{"answers":[]}`，跳过整张表单使用 `{"answers":{}}`。

| 收到的问题 | 提交结果 | 前端读取后续内容的位置 |
| --- | --- | --- |
| `detached: false`，Codex 正在等待用户回答 | `204 No Content` | 保持原来的 `POST /responses` 打开。服务端将答案交给正在执行的 Codex，后续文字和 `message_stop` 从第一个终端返回。 |
| `detached: true`，命令已生成表单，用户提交了答案 | `200 OK` | 服务端在同一会话继续执行。上面的 `stream: true` 使第二个终端收到新的 SSE。复用原来的 SSE 解析函数处理这份响应。省略 `stream` 时，第二个终端收到最终的 JSON 回复。 |
| `detached: true`，用户跳过整张表单 | `204 No Content` | 将表单标记为已处理，本次提交结束。 |

Agentic 的 `action_signal` 中，`chat_input` 按钮会发送新的聊天消息。请为提问表单单独绑定提交函数，调用 `/interactions/{interaction_id}/respond`。成功后将表单设为只读。读取任何一条 SSE 时，遇到普通 `error` 事件仍然显示服务端错误，遇到下一组 `request_user_input` 则显示下一张表单。

## 4. 恢复表单和处理失败

进入已有会话或提交结果不明时，查询该实例、Agent 和会话当前等待回答的问题。

```sh
curl --silent --show-error "$SESSION_URL/interactions" \
  --header "Authorization: Bearer $TOKEN"
```

返回的 JSON 为 `{"interactions":[...]}`。每项包含与提问事件相同的表单字段。按 `interaction_id` 恢复仍待回答的表单，空数组表示当前没有待回答的问题。

- 提交时锁定按钮。网络中断后查询待回答列表，只有原问题仍在列表中才允许重新提交。
- 原生提问依赖原回复连接。页面关闭导致连接中断，或者用户调用 `POST /responses/cancel` 停止执行后，该轮不再等待答案。查询列表可以在其他页面恢复仍在等待的表单，无法重放已经断开的 SSE。同一会话开始新的对话轮次时，上一轮仍待回答的 `detached: true` 表单会失效。
- 同步客户端也可以设置 `"user_input":true,"stream":false`。遇到原生提问时，原请求会一直等待，由另一条连接查询问题并提交答案。遇到 `detached: true` 的表单时，原请求返回的 JSON 在顶层 `interactions` 中列出问题。

提交答案时，从错误 JSON 的 `error.code` 判断下一步操作。

| HTTP | `error.code` | 页面处理 |
| --- | --- | --- |
| 400 | `invalid_request` | 保留表单，检查所有问题 ID、选项原文、自由填写的前缀和答案数组，再次提交。 |
| 401 | `unauthorized` | 更新当前实例的授权凭证，再用同一实例地址查询待回答列表。 |
| 404 | `session_not_found` 或 `interaction_not_found` | 核对实例、Agent、会话和问题 ID，查询该会话仍可回答的问题。 |
| 409 | `interaction_already_resolved` | 停止重复提交。另一项提交可能正在处理，重新查询待回答列表。 |
| 410 | `interaction_gone` | 问题已过期或执行已取消，关闭该表单。 |
