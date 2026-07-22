# CSGClaw 结构化 Skill 输出协议

[English](structured-output.md) | 中文

CSGClaw 为需要附加资源链接或打开交互式问答流程的 skill 脚本提供行式 stdout 协议。
该协议归 CSGClaw 所有，不绑定特定的 skill 引擎、模型或 Agent Runtime。
它不是 MCP，也不依赖对任意 JSON 的识别。

## 规范可执行示例

Manager 内置 skill [`csgclaw-interactive-output-demo`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/) 是完整的参考实现。
其中的 [`emit_demo.py`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py) 会在每个支持字段首次出现时解释其作用，并演示完整和最小资源链接、五个问题以及自动续接响应。
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
import os


if os.environ.get("CSGCLAW_STRUCTURED_OUTPUT_PROTOCOL") != "1":
    raise RuntimeError("CSGClaw structured output protocol version 1 is unavailable")


def emit(kind: str, payload: dict[str, object]) -> None:
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    print(f"::csgclaw-output::{kind} {encoded}")
```

实现该协议的 Runtime Adapter 会向 skill 命令环境注入 `CSGCLAW_STRUCTURED_OUTPUT_PROTOCOL=1`。
可移植 emitter 应在输出控制记录前检查这个能力，并在能力不存在时只返回普通诊断信息。
这个握手可以避免旧 runtime 把所有控制记录当作普通 stdout 时，脚本仍错误地宣称交互输出已经就绪。

## Turn 生命周期

1. Skill 脚本输出普通文本以及零条或多条控制记录。
2. CSGClaw Runtime Adapter 只从成功完成的命令执行中解码记录。
3. 有效记录会被缓冲，直到整个 Agent turn 成功结束。
4. CSGClaw 首先持久化正常的最终响应。
5. 资源链接以 Markdown 形式追加到响应末尾，如果提供了安全的 HTTP(S) 图标，还会渲染第一个有效图标。
6. 最终响应和链接显示后，CSGClaw 再激活独立的问答请求。
7. 用户提交答案后，CSGClaw 会更新原有问题 activity，在历史记录中添加可读且已脱敏的摘要，并携带精确的响应 JSON 自动续接原始 Agent 会话。

失败、取消、被替代、中断、过期或失效的 turn 不会激活缓冲输出或自动续接。
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

精确的响应 JSON 会通过自动续接提示词传入同一个原始 Agent 会话。
持久化的问题 activity 和可读摘要会对 secret 值脱敏，但继续执行的 skill 也不得在最终响应、日志或后续工具调用中回显 secret。
Demo skill 展示了推荐的两层结果格式，即先输出机器可解析且已脱敏的 JSON，再输出简洁的人类可读 Markdown。

客户端可以将该对象直接提交到：

```text
POST /api/v1/channels/{channel}/activities/{activity_id}:respond
```

请求正文就是响应对象本身，不需要额外的 room、responder、submission 或 behavior 包装。
CSGClaw 会从已存储的 activity 中推导 room 和当前 responder。

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
