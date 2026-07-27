# CSGClaw 结构化 Skill 输出协议

[English](structured-output.md) | 中文

CSGClaw 为需要附加资源链接或打开交互式问答流程的 skill 脚本提供行式 stdout 协议。
该协议归 CSGClaw 所有，不绑定特定的 skill 引擎、模型或 Agent Runtime。
它不是 MCP，也不依赖对任意 JSON 的识别。

## 规范可执行示例

Manager 内置 skill [`csgclaw-interactive-output-demo`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/) 是完整的参考实现。
其中的 [`emit_demo.py`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py) 会在每个支持字段首次出现时解释其作用，并连续发出三个问答阶段。
前两个阶段都会发出相同的完整与最小 `ResourceLink` 示例，第三个阶段仅演示最终操作和 secret 输入。
这三个阶段覆盖普通选项、Recommended 与 Unicode label、包含四个问题的导航页面、选项或自由输入、仅自由输入以及 secret 问题。
Manager 会读取每次自动续接响应并自行选择下一个白名单脚本阶段，而 Python emitter 永远不会接收或解析响应 JSON。
每次续接只执行一个阶段命令，成功命令发出下一个问题请求后，CSGClaw 会立即结束当前 turn。
它不会把 emitter stdout 当作答案，也不会在新的续接提示词包含下一阶段所需问题 ID 之前进入后续阶段。
根 `SKILL.md` 使用渐进披露，根据每次响应只路由到 `references/` 下的一个阶段文件，使模型不会把后续阶段命令当成当前指令。
Manager provision 会在内置 skill 目录缺失时安装它，并保留已经安装或自定义的副本。

在 Manager 会话中使用以下提示词调用它：

```text
Use $csgclaw-interactive-output-demo to run the complete interactive output demo.
```

该 skill 只能显式调用，不会被隐式选择。

## 记录格式

一条控制记录必须独占一整行 stdout：

```text
::csgclaw-output::<kind> <single-line JSON object>
```

当前注册了两种类型：

```text
::csgclaw-output::request_user_input <RequestUserInputArgs JSON>
::csgclaw-output::resource_link <ResourceLink JSON>
```

前缀必须从该行的第一个字符开始。
每个 JSON payload 必须编码为一个物理行，普通日志或状态文本需要输出到其他行。
CSGClaw 会从可见工具输出中移除有效控制行，同时保留普通 stdout。
未知、格式错误、过大或无效的记录会被忽略，并作为普通输出继续显示。

最小 Python emitter 如下：

```python
import json


def emit(kind: str, payload: dict[str, object]) -> None:
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    print(f"::csgclaw-output::{kind} {encoded}")
```

## Turn 生命周期

1. Skill 脚本输出普通文本以及零条或多条控制记录。
2. CSGClaw Runtime Adapter 只从成功完成的命令执行中解码记录。
3. 有效的 `request_user_input` 记录会成为 Runtime 强制执行的 turn 边界，因此模型不能在用户回答前执行后续阶段。
4. CSGClaw 会主动中断该 turn，并把这个边界视为成功结束。
5. 如果模型尚未生成响应，发出控制记录的命令所产生的普通 stdout 会成为可读响应。
6. CSGClaw 首先持久化该正常响应。
7. 资源链接以 Markdown 形式追加到响应末尾，如果提供了安全的 HTTP(S) 图标，还会渲染第一个有效图标。
8. 最终响应和链接显示后，CSGClaw 再激活独立的问答请求。
9. 用户提交答案后，CSGClaw 会先更新原有的 Agent 问题消息，并持久化一条独立且可读的本地用户答案消息。
10. 随后，CSGClaw 会携带 wire 兼容的响应 JSON 副本自动续接同一个原始 Agent 会话，其中已提交的 secret 值会替换为 `<redacted>`。

Skill 作者应当像 demo emitter 一样，在控制记录之前输出简洁、可读的 Markdown。
如果命令没有普通 stdout，CSGClaw 会使用 `Please answer the questions below.` 作为响应兜底文本。
正常的失败、取消、被替代、中断、过期或失效 turn 不会激活缓冲输出或自动续接。
只有 Runtime 主动发起的结构化输出边界才会被视为成功。
较新的用户 turn、会话重置、房间关闭、重复响应或服务重启也会阻止过期续接。

## `request_user_input`

Payload 使用 CSGClaw 的 `RequestUserInputArgs` schema，其字段名与 Codex 保持源码兼容：

```json
{
  "questions": [
    {
      "id": "verification",
      "header": "Checks",
      "question": "How cautious should verification be?",
      "isOther": true,
      "isSecret": false,
      "options": [
        {
          "label": "Standard (Recommended)",
          "description": "Use normal checks and targeted tests."
        },
        {
          "label": "Strict",
          "description": "Add broader verification and explicit acceptance criteria."
        }
      ]
    }
  ],
  "autoResolutionMs": 240000
}
```

### 请求字段

| 字段 | 必填 | 含义 |
| --- | --- | --- |
| `questions` | 是 | 包含 1 至 32 个问题的有序列表。 |
| `autoResolutionMs` | 否 | 可选的过期时间，范围为 60000 至 240000 毫秒。 |

### 问题字段

| 字段 | 必填 | 含义 |
| --- | --- | --- |
| `id` | 是 | 响应 map 使用的稳定且唯一的 key。 |
| `header` | 是 | Activity 和历史记录中使用的短标签。 |
| `question` | 是 | 作为 composer 标题渲染的具体问题。 |
| `isOther` | 否 | 为 `true` 时显示自由输入选项。 |
| `isSecret` | 否 | 为 `true` 时使用密码输入框，并对持久化历史进行脱敏。 |
| `options` | 否 | `null` 或最多包含 12 个选项的数组。 |

每个选项必须包含 `label`，并可选包含 `description`。
在 label 末尾追加精确后缀 ` (Recommended)` 可以渲染 Recommended 标签。
包含该后缀的原始 label 会作为提交值保留。

当 `isOther` 为 `true` 时，composer 会在选项之外显示自由输入框。
当 options 缺失或为空时，composer 只显示自由输入框。
当前 UI 将选项和自由输入文本视为互斥答案。
`isSecret` 只应用于一次性测试值或真正敏感的值，示例中的 secret 问题应明确提醒用户不要输入生产凭据。

不要添加 `actions`、`recommended`、`submission`、`behavior`、`need_input` 或 `action_signal` 等私有字段。

## 响应结构

Web UI 提交精确的 CSGClaw `RequestUserInputResponse` 对象，该对象与 Codex 保持源码兼容：

```json
{
  "answers": {
    "verification": {
      "answers": ["Standard (Recommended)"]
    },
    "note": {
      "answers": ["user_note: Keep the report concise."]
    },
    "test_secret": {
      "answers": []
    }
  }
}
```

外层 `answers` 对象以问题 ID 为 key。
内层 `answers` 值保留数组结构，以维持 wire 兼容性并支持未来扩展。
由于选项和自由输入文本互斥，当前 composer 通常产生零个或一个值。

空的外层对象会跳过整个请求，并且不会创建自动续接：

```json
{"answers": {}}
```

非空响应必须包含每个问题 ID。
使用空的内层数组跳过单个问题。
自由输入值使用 `user_note: ` 前缀。

自动续接提示词会收到一个 wire 兼容的响应 JSON 副本。
非 secret 答案保持精确，已提交的 secret 值变成 `<redacted>`，跳过的 secret 数组仍然为空。
持久化的问题 activity 和本地用户答案记录会对 secret 值脱敏，但继续执行的 skill 也不得在最终响应、日志或后续工具调用中回显 secret。
响应 JSON 是 Agent brain 的 Runtime 输入，而不是发出请求的脚本输入。
Demo skill 展示了推荐的多阶段模式，即 Manager 解释稳定的问题 ID 和答案值，选择下一个白名单阶段命令，并且只把安全的分支选择器传给 emitter。
该 skill 强制每个 turn 只执行一个命令，确保每个发出的请求都会先显示并收到真实用户答案，然后才执行下一阶段。
为了让本地和远程模型都稳定执行，应把后续命令放在独立的阶段 reference 中。
可读答案 Markdown 已经由 CSGClaw 作为独立的本地用户消息持久化，因此 skill 不需要重新构造或回显它。

客户端可以将该对象直接提交到：

```text
POST /api/v1/channels/{channel}/activities/{activity_id}:respond
```

请求正文就是响应对象本身，不需要额外的 room、responder、submission 或 behavior 包装。
CSGClaw 会从已存储的 activity 中推导 room 和当前 responder。

## 持久化对话与 JSONL

每个请求会被存储为一条由提问 Agent 所有的独立消息。
请求进入 pending 状态后，其 `content` 会立即包含可读 Markdown，而 `metadata.csgclaw.agent_activity` 会保留交互式 UI 使用的结构化 activity。
请求被回答、整体跳过、过期、取消或中断时，CSGClaw 会更新同一个消息 ID。

```markdown
## Questions

- demo_kind：What kind of CSGClaw demo should this be?
  - Bug fix (Recommended) (Plans a focused repair workflow.)
  - New feature (Plans a user-facing feature.)
- freeform_note：Add a freeform note.
- test_secret：Enter a disposable test value only.
```

非空提交会创建一条由当前已认证本地用户所有的独立消息。
答案消息与问题位于同一个 room 和 thread，并由 `metadata.csgclaw.request_user_input` 标记。
该消息会通过普通 IM event 更新 UI，但绝不会作为第二个 participant prompt 分发。
自动续接仍然是唯一的新 Agent turn。

```markdown
## Answers

- demo_kind：Bug fix (Recommended) (Plans a focused repair workflow.)
- destination：QA / 验收 (Custom answer)
- freeform_note：Skipped (No answer provided)
- test_secret：Secret recorded (Secret value redacted)
```

答案行使用精确的全角分隔符 `：`。
选中选项会保留原始 label 和 description。
缺失的选项 description 会变成 `No description provided`。
自由输入值会移除一个开头的 `user_note: ` 前缀，并使用 `Custom answer` 作为 description。
当选项和备注同时存在时，label 会变成 `<option>; <note>`，并保留选项 description。
单独跳过的问题会变成 `Skipped (No answer provided)`。
已提交的 secret 会变成 `Secret recorded (Secret value redacted)`，而跳过的 secret 仍然显示为 `Skipped (No answer provided)`。
空的外层 `answers` 对象会跳过整个请求，并且不创建本地用户答案消息或自动续接。

以下两个物理行展示了提交非 secret 答案后的持久化 JSONL 表示。
与示例无关的字段，例如 attachments，会按正常 `omitempty` 规则省略。

```jsonl
{"id":"question-request-1","sender_id":"u-manager","content":"## Questions\n\n- demo_kind：What kind of CSGClaw demo should this be?\n  - Bug fix (Recommended) (Plans a focused repair workflow.)","metadata":{"csgclaw":{"agent_activity":{"type":"com.opencsg.csgclaw.agent.activity","version":1,"event_id":"question-request-1","sender":"u-manager","channel":"csgclaw","room_id":"room-1","origin_server_ts":1784736000000,"content":{"msgtype":"com.opencsg.csgclaw.agent.question","body":"Question answered","question":{"id":"request-1","status":"answered","questions":[{"id":"demo_kind","header":"Demo kind","question":"What kind of CSGClaw demo should this be?","options":[{"label":"Bug fix (Recommended)","description":"Plans a focused repair workflow."}]}],"answers":{"demo_kind":{"answered":true,"option_index":1,"option_label":"Bug fix (Recommended)"}}}}}}},"created_at":"2026-07-22T12:00:00Z","mentions":[]}
{"id":"answer-request-1","sender_id":"user-admin","content":"## Answers\n\n- demo_kind：Bug fix (Recommended) (Plans a focused repair workflow.)","metadata":{"csgclaw":{"request_user_input":{"kind":"answer","request_id":"request-1"}}},"created_at":"2026-07-22T12:01:00Z","mentions":[]}
```

对于 thread 中的问题，答案行还会包含 `"relates_to":{"rel_type":"m.thread","event_id":"<thread-root-id>"}`。
精确接收的 `RequestUserInputResponse` 只作为瞬时 broker 输入，因为其中可能含有 secret，所以不会持久化。
只有经过 secret 脱敏且 wire 兼容的副本会进入自动模型续接。
只有自动续接推进工作流时，后续结果才会成为一条独立的 Agent 消息。
CSGClaw 不会合成一条由 Agent 所有的用户答案回显。

Manager 内置 demo skill 仍然是 wire 协议、可读 transcript 所有权以及 Agent 驱动多阶段续接的完整可执行参考。

## `resource_link`

Payload 使用 CSGClaw 的源码兼容 `ResourceLink` 字段名：

```json
{
  "type": "resource_link",
  "name": "csgclaw-repository",
  "title": "CSGClaw source",
  "uri": "https://github.com/OpenCSGs/csgclaw",
  "description": "Source code and implementation details.",
  "mimeType": "text/html",
  "size": 2048,
  "annotations": {
    "audience": ["user"],
    "priority": 0.9,
    "lastModified": "2026-07-20T00:00:00Z"
  },
  "_meta": {
    "variant": "full"
  },
  "icons": [
    {
      "src": "https://example.com/icon.svg",
      "mimeType": "image/svg+xml",
      "sizes": ["any"],
      "theme": "dark"
    }
  ]
}
```

### 链接字段

| 字段 | 必填 | 含义 |
| --- | --- | --- |
| `type` | 是 | 必须是字面值 `resource_link`。 |
| `name` | 是 | 稳定的机器可读资源名，同时作为备用显示标签。 |
| `uri` | 是 | 绝对 HTTP(S) URL。 |
| `title` | 否 | 首选的可见链接标签。 |
| `description` | 否 | 渲染在链接后的上下文说明。 |
| `mimeType` | 否 | 资源的 MIME type。 |
| `size` | 否 | 以字节为单位的资源大小。 |
| `annotations` | 否 | 标准 audience、priority 和修改时间提示。 |
| `_meta` | 否 | 原样保留的应用元数据。 |
| `icons` | 否 | 资源图标候选项。 |

CSGClaw 每个 turn 最多接受 16 个唯一链接，并按 `uri` 去重。
CSGClaw 会在 Markdown 链接旁渲染第一个使用绝对 HTTP(S) `src` 的图标。
不安全的链接或图标 scheme 不会被渲染。

最小链接只需要 `type`、`name` 和 `uri`：

```text
::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}
```

## 限制与兼容性

- 每条控制记录最大为 256 KiB。
- 每个成功 turn 只接受一个 `request_user_input` 请求。
- 每个请求包含 1 至 32 个问题，每个问题最多包含 12 个选项。
- 每个 turn 最多包含 16 个按 URI 去重的 HTTP(S) 资源链接。
- 当前 Codex Runtime Adapter 会解码 `commandExecution` 输出以及旧版 `exec_command_end` 和 `function_call_output` event 结构。
- 其他 Runtime Adapter 可以实现相同的行式协议，而无需修改 skill 或 emitter。
- Codex 原生阻塞式 `item/tool/requestUserInput` 请求复用相同的 CSGClaw activity 和响应模型，但不使用此 stdout 协议。
- Permission action 和特权 `csgclaw.action_card` activity 属于独立协议。

请保持文档中的 JSON 字段名稳定，使相同的 skill 输出可以在不同 Runtime Adapter 中得到一致处理。
