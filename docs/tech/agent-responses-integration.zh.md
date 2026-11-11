# 第三方客户端接入 CSGClaw Agent

使用 `POST /api/v1/agents/{agent}/sessions/{session_id}/responses` 向一个正在运行的 Agent 发送消息。CSGClaw 会调用 Agent 的实际 Runtime，并维护多轮上下文。这个入口适合自有网页、桌面客户端和服务端应用。路径中带 `/llm/` 的 Responses 接口用于代理模型流量。

## 1. 准备实例、Agent 和会话

- **实例地址**由部署方提供。直连实例和 OpenCSG 云端转发使用不同的公开路径，见下文。
- **Agent** 可以由部署方指定。直连实例也可以从 `GET /api/v1/agents` 获取列表。选择正在运行的 Agent。CSGClaw 接受 Agent ID 或不区分大小写的唯一名称，云端调用使用平台分配的 Agent ID。
- **会话 ID** 由客户端生成，例如用 `crypto.randomUUID()`。允许 1–128 个 ASCII 字符，首字符须为字母或数字，其余可使用字母、数字和 `._~-`。首次发送时会按需建立会话绑定。继续对话时复用同一实例、Agent 和会话 ID，每次只发送本轮输入。新建聊天时生成新的 ID。
- **直连凭证** 使用当前实例的服务端 access token，通过 `Authorization: Bearer <token>` 传入。启用服务认证时，附件上传、引用以及启用和处理提问表单都需要有效凭证。没有待处理的受保护表单时，省略 `user_input` 或设为 `false` 的纯文本 JSON 请求可以匿名发送。云端请求还受 OpenCSG 网关的认证规则约束。

会话 ID 用于续接上下文。对外提供 Web UI 时，由应用后端校验登录用户是否有权访问对应的实例、Agent 和会话，再转发请求并注入实例凭证。将凭证保存在后端，并确保代理透传 SSE 且关闭响应缓冲。客户端自行保存用于展示的聊天记录，CSGClaw 会在服务端维护 Agent 的执行上下文。

### OpenCSG 云端转发

OpenCSG 托管的 Agent 通过网关接收请求。以下是直连路径与 OpenCSG QA 中观察到的转发路径。主机名随环境变化，接入时使用平台提供的公开地址和 Agent ID。

| 部署方式 | Responses 请求地址 |
| --- | --- |
| 直连 CSGClaw | `http://127.0.0.1:18080/api/v1/agents/{agent}/sessions/{session_id}/responses` |
| OpenCSG 云端转发 | `https://{opencsg-host}/aigateway/v1/agent/csgclaw/agents/{agent}/sessions/{session_id}/responses` |

此网关路由以 `/aigateway/v1/agent/csgclaw` 替换直连路径里的 `/api/v1`。如果平台已提供完整的 Responses URL，可直接使用该 URL。请求仍为 POST，沿用下文的 JSON 请求体、`stream: true` 和 SSE 处理方式。

使用当前 OpenCSG 环境为该 Agent 授予的网关凭证，凭证获取方式和请求头以部署方的网关接入说明为准。Portal 浏览器中看到的 HTTP 200 仅表明当前登录会话的请求已通过网关。第三方后端应先确认自己的调用身份能访问该 Agent，并向部署方确认取消、附件和提问表单各自的公开路径及权限。

## 2. 先跑通一轮 JSON 请求

将 `AGENT` 改成正在运行的 Agent ID。示例默认使用本地实例和纯文本请求。需要发送实例凭证时，预先设置环境变量 `CSGCLAW_TOKEN`。

```bash
BASE_URL=http://127.0.0.1:18080
AGENT=reviewer
SESSION_ID="$(uuidgen)"
RESPONSES_URL="${BASE_URL%/}/api/v1/agents/${AGENT}/sessions/${SESSION_ID}/responses"
AUTH=()
if [ -n "${CSGCLAW_TOKEN:-}" ]; then
  AUTH=(-H "Authorization: Bearer ${CSGCLAW_TOKEN}")
fi

curl -sS "${AUTH[@]}" "$RESPONSES_URL" \
  -H 'Content-Type: application/json' \
  --data '{"input":"请用一句话介绍你能做什么。"}'
```

通过 OpenCSG 网关调用时，在执行 curl 前将 `RESPONSES_URL` 改为以下形式，并按部署方要求设置网关凭证。如果该环境为第三方调用签发 Bearer 凭证，可将请求头设置为 `AUTH=(-H "Authorization: Bearer ${OPENCSG_GATEWAY_TOKEN}")`，其中 `OPENCSG_GATEWAY_TOKEN` 为部署方提供的网关凭证。

```bash
RESPONSES_URL="https://${OPENCSG_HOST}/aigateway/v1/agent/csgclaw/agents/${AGENT}/sessions/${SESSION_ID}/responses"
```

省略 `stream` 或传 `false` 时，服务端在本轮完成后返回 HTTP 200 和 JSON。下面只展示客户端通常需要读取的字段。

```json
{
  "id": "resp_example",
  "object": "response",
  "status": "completed",
  "output": [
    {
      "type": "message",
      "role": "assistant",
      "content": [
        {"type": "output_text", "text": "我可以帮你分析文档和代码。"}
      ]
    }
  ],
  "metadata": {
    "agent_id": "reviewer",
    "session_id": "c89f03a4-7ba4-4e1b-92a6-bf37edc2932b",
    "room_id": ""
  }
}
```

遍历 `output[].content[]`，按顺序读取 `type: "output_text"` 的 `text`。后续请求继续使用 `SESSION_ID`。响应的 `id` 标识本轮执行，会话由请求路径标识。

## 3. 逐步显示回复

沿用上面的 `RESPONSES_URL`。等待上一轮完成后发送流式请求。

```bash
curl -sS -N "${AUTH[@]}" "$RESPONSES_URL" \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  --data '{"input":"请举一个例子。","stream":true}'
```

返回的 `Content-Type` 为 `text/event-stream`。事件采用 Anthropic 风格。浏览器使用 `fetch` 发起 POST，并从 `response.body` 读取 SSE。使用支持 POST 的 SSE 解析器，按空行分隔事件，拼接同一个事件的多行 `data:`，并忽略 `: heartbeat` 等注释。网络读取的 chunk 可能切在事件或 UTF-8 字符中间。

| 事件 | 客户端处理 |
| --- | --- |
| `message_start` | 初始化本轮状态。记录 `message.id`，读取可选的顶层 `input_attachments`。 |
| `content_block_start` / `content_block_stop` | 按 `index` 管理内容块。仅显示正文的客户端可以跳过这两种事件。 |
| `content_block_delta` | 仅当 `delta.type === "text_delta"` 时，将 `delta.text` 追加到当前回复。 |
| `message_delta` | 读取 `delta.stop_reason`。`end_turn` 表示成功结束，`error` 表示失败。结果附件位于事件顶层的 `attachments`。 |
| `message_stop` | 本轮事件已发完。结合前面的 `stop_reason` 确认结果。 |
| `error` | 读取 `error.message`，将本轮标为失败。HTTP 状态仍可能是 200。 |

下面的浏览器示例可以连接同源应用后端代理，也可以用于已允许跨源访问的可信测试环境。它展示纯文本回复如何逐段更新。

```js
async function streamReply(url, input, onText, signal) {
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify({ input, stream: true }),
    signal,
  })

  if (!res.ok) {
    const body = await res.json().catch(() => null)
    throw new Error(body?.error?.message ?? `HTTP ${res.status}`)
  }
  if (!res.body || !res.headers.get("Content-Type")?.includes("text/event-stream")) {
    throw new Error("预期收到 SSE 响应")
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  let text = ""
  let stopReason

  try {
    while (true) {
      const { done, value } = await reader.read()
      buffer += decoder.decode(value, { stream: !done })
      let boundary
      while ((boundary = /\r?\n\r?\n/.exec(buffer))) {
        const block = buffer.slice(0, boundary.index)
        buffer = buffer.slice(boundary.index + boundary[0].length)
        const data = block.split(/\r?\n/)
          .filter(line => line.startsWith("data:"))
          .map(line => line.slice(5).replace(/^ /, ""))
          .join("\n")
        if (!data) continue

        const event = JSON.parse(data)
        if (event.type === "content_block_delta" && event.delta?.type === "text_delta") {
          text += event.delta.text
          onText(text)
        }
        if (event.type === "error") {
          throw new Error(event.error?.message ?? "Agent 执行失败")
        }
        if (event.type === "message_delta") stopReason = event.delta?.stop_reason
        if (event.type === "message_stop") {
          if (stopReason !== "end_turn") throw new Error("Agent 执行未成功完成")
          return text
        }
      }
      if (done) throw new Error("回复完成前连接已断开")
    }
  } finally {
    await reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}
```

例如，将 `onText` 设为 `text => { replyElement.textContent = text }`，每次收到增量都会更新回复区域。如果要展示工具状态，读取 `content_block_start` 的 `content_block`，用 `tool_use` 块的 `id` 和 `tool_result` 块的 `tool_use_id` 关联调用及结果。工具参数和输出分别通过 `input_json_delta`、`tool_result_delta` 传输。

## 4. 处理并发、取消和错误

同一 Agent 的同一会话同时只能执行一轮。发送后禁用该会话的发送按钮，等本轮结束或取消完成再开放。不同会话可以并发。用户点击停止时，用单独的请求调用同一 URL 下的 `POST /responses/cancel`，等待 HTTP 204 再允许下一轮。

```bash
curl -sS -o /dev/null -w '%{http_code}\n' "${AUTH[@]}" \
  -X POST "$RESPONSES_URL/cancel"
```

没有正在执行的轮次时，取消仍返回 204。直接断开流也会触发服务端取消，但客户端收不到 Runtime 清理完成的确认。断线时把已收到的文字标为未完成。重新发送会尝试执行新的一轮，特别是 Agent 可能已执行过工具时，应由用户决定是否重发。

非 2xx 响应通常使用 `{"error":{"message":"…","type":"…","param":"…","code":"…"}}`。流开启前检查 HTTP 状态，开启后检查 SSE 的 `error` 及结束事件。

| HTTP 状态 | 常见情况 |
| --- | --- |
| 400 | `invalid_session_id` 或 `invalid_request`，检查路径及输入字段。 |
| 401 | `unauthorized`，检查实例凭证。 |
| 404 | `agent_not_found`，或引用的附件在当前会话不可用。 |
| 409 | JSON 模式下的 `session_busy`，等待前一轮结束。流已经开启时，会话忙通过 SSE `error` 报告。 |
| 413 | 请求体或文件超出大小限制。 |
| 503 | Agent 未运行、Runtime 不可用或执行失败，按 `error.code` 区分。 |

## 5. 按需接入附件和提问表单

**附件上传**。先对同一会话的 `/attachments` 发起 multipart POST，每个文件使用一个 `files` 字段。成功时返回 HTTP 201 和 `attachments[].id`。在随后的 JSON 请求中带上这些 ID。

```bash
curl -sS "${AUTH[@]}" -F 'files=@report.pdf' "${RESPONSES_URL%/responses}/attachments"
```

```json
{"input":"总结这份报告。","attachment_ids":["<上传返回的附件 ID>"],"stream":true}
```

也可以一次用 multipart 请求直接调用 `/responses`。用 `JSON.stringify(prompt)` 编码 `input`，可选的 `stream` 使用字符串 `"true"` 或 `"false"`，每个文件使用一个 `file` 字段。使用浏览器的 `FormData` 时，由浏览器设置包含 boundary 的 `Content-Type`。两种方式每次最多提交 10 个非空文件，每个最多 1 GiB，合计最多 2 GiB。

JSON 响应在 `input_attachments` 返回用户文件，在 `output[].attachments` 返回 Agent 发布的结果文件。流式响应在 `message_start` 返回上传附件，在成功的 `message_delta` 返回结果附件。聊天记录保存附件 ID、名称和所属实例，通过该实例的 `GET /api/v1/attachments/{id}` 读取文件，并使用当前实例凭证。

**提问表单**。基础聊天可以省略 `user_input`，Agent 会通过普通回复提问。支持表单时，传 `"user_input": true` 并监听 `request_user_input` 事件。显示事件中的 `questions`，用同一会话的 `POST /interactions/{interaction_id}/respond` 提交。可通过 `GET /interactions` 查询待答表单。答案对象以问题 ID 为键。

```json
{"answers":{"source":{"answers":["PDF"]}}}
```

`detached: false` 的表单提交后返回 204，原流继续输出。`detached: true` 的表单可在原轮结束后提交，`/respond` 默认返回新的 JSON 回复，传顶层 `"stream": true` 则返回新的 SSE。表单字段、可选的模型参数和上下文参数见 [完整 API 契约](api.zh.md#post-apiv1agentsagentsessionssession_idresponses)。内置 UI 实现见 [Session API Demo 指南](web/session-api-demo.zh.md)。
