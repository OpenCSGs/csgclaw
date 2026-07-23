# CSGClaw Structured Skill Output Protocol

English | [中文](structured-output.zh.md)

CSGClaw provides a line-oriented stdout protocol for skill scripts that need to attach resource links or open an interactive question flow.
The protocol belongs to CSGClaw and is not bound to a particular skill engine, model, or agent runtime.
It is not MCP and does not rely on arbitrary JSON detection.

## Canonical executable example

The built-in Manager skill [`csgclaw-interactive-output-demo`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/) is the complete reference implementation.
Its [`emit_demo.py`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py) documents every supported request and resource-link field at its first use and emits three successive question stages.
The first two stages each emit the same full and minimal `ResourceLink` examples, while the third stage focuses only on final action and secret input.
The stages cover ordinary options, Recommended and Unicode labels, a four-question navigation page, an option-or-freeform question, a freeform-only question, and a secret question.
The Manager reads each automatic continuation response and selects the next allowlisted script stage itself, while the Python emitter never receives or parses response JSON.
Each continuation executes exactly one stage command, and CSGClaw ends that turn as soon as the successful command emits the next question request.
It never treats emitter stdout as an answer or enters a later stage until a new continuation prompt contains that stage's required question IDs.
The root `SKILL.md` uses progressive disclosure and routes each response to one stage-specific file under `references/`, so a model does not receive later-stage commands as current instructions.
Manager provisioning installs a bundled skill when its skill directory is missing and preserves an existing installed or customized copy.

Invoke it in a Manager conversation with:

```text
Use $csgclaw-interactive-output-demo to run the complete interactive output demo.
```

The skill is explicit-only and is not selected implicitly.

## Record format

A control record occupies one complete stdout line:

```text
::csgclaw-output::<kind> <single-line JSON object>
```

Two kinds are registered:

```text
::csgclaw-output::request_user_input <RequestUserInputArgs JSON>
::csgclaw-output::resource_link <ResourceLink JSON>
```

The prefix must begin at the first character of the line.
Encode each JSON payload on one physical line and write ordinary logs or status text on separate lines.
CSGClaw removes valid control lines from visible tool output and preserves ordinary stdout.
Unknown, malformed, oversized, or invalid records are ignored and remain visible as ordinary output.

A minimal Python emitter is:

```python
import json


def emit(kind: str, payload: dict[str, object]) -> None:
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    print(f"::csgclaw-output::{kind} {encoded}")
```

## Turn lifecycle

1. A skill script prints ordinary output and zero or more control records.
2. A CSGClaw runtime adapter decodes records only from a successfully completed command execution.
3. A valid `request_user_input` record becomes a runtime-enforced turn boundary, so the model cannot execute another stage before the user answers.
4. CSGClaw intentionally interrupts that turn and treats the boundary as successful.
5. Ordinary stdout from the emitting command becomes the readable response when the model has not already produced one.
6. CSGClaw persists that normal response first.
7. Resource links are appended to the end of that response as Markdown, including the first safe HTTP(S) icon when one is provided.
8. A detached question request is activated after the final response and links are visible.
9. A submitted answer first updates the existing agent-owned question message and persists a separate readable local-user answer message.
10. CSGClaw then automatically continues the same originating agent session with a wire-compatible response JSON copy whose submitted secret values are replaced with `<redacted>`.

Skill authors should print concise readable Markdown before the control record, as shown by the demo emitter.
If a command emits no ordinary stdout, CSGClaw uses `Please answer the questions below.` as the response fallback.
A normal failure, cancellation, supersession, interruption, expiration, or stale turn does not activate buffered output or automatic continuations.
Only the runtime-requested structured-output boundary is treated as successful.
A newer user turn, session reset, room closure, duplicate response, or server restart also prevents a stale continuation.

## `request_user_input`

The payload uses CSGClaw's `RequestUserInputArgs` schema, whose field names remain source-compatible with Codex:

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

### Request fields

| Field | Required | Meaning |
| --- | --- | --- |
| `questions` | Yes | Ordered list containing 1 through 32 questions. |
| `autoResolutionMs` | No | Optional expiration window from 60000 through 240000 milliseconds. |

### Question fields

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | Yes | Stable, unique key used in the response map. |
| `header` | Yes | Short activity and history label. |
| `question` | Yes | Concrete question rendered as the composer title. |
| `isOther` | No | Shows a freeform alternative when true. |
| `isSecret` | No | Uses a password input and redacts persisted history when true. |
| `options` | No | `null` or an array containing at most 12 options. |

Each option has a required `label` and an optional `description`.
Append the exact suffix ` (Recommended)` to a label to render the Recommended badge.
The original label, including the suffix, remains the submitted value.

When `isOther` is true, the composer displays a freeform input in addition to any options.
When options are absent or empty, the composer displays a freeform-only input.
The current UI treats an option and freeform text as mutually exclusive.
`isSecret` should be used only for disposable or genuinely sensitive values, and secret prompts should explicitly warn users not to enter production credentials in examples.

Do not add private fields such as `actions`, `recommended`, `submission`, `behavior`, `need_input`, or `action_signal`.

## Response shape

The Web UI submits the exact CSGClaw `RequestUserInputResponse` object, which remains source-compatible with Codex:

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

The outer `answers` object is keyed by question ID.
Each inner `answers` value remains an array for wire compatibility and future flexibility.
The current composer normally produces zero or one value because choices and freeform text are mutually exclusive.

An empty outer object skips the entire request and creates no automatic continuation:

```json
{"answers": {}}
```

In a non-empty response, include every question ID.
Use an empty inner array to skip one question.
Freeform values use the `user_note: ` prefix.

The automatic continuation prompt receives a wire-compatible response JSON copy.
Non-secret answers remain exact, submitted secret values become `<redacted>`, and skipped secret arrays remain empty.
Persisted question activity and the local-user answer transcript redact secret values, but the continuing skill must also avoid echoing a secret into its final response, logs, or later tool calls.
The response JSON is runtime input for the agent brain, not input for the script that emitted the request.
The demo skill shows the recommended multi-stage pattern: the Manager interprets stable question IDs and values, chooses an allowlisted next-stage command, and passes only safe branch selectors to the emitter.
The skill enforces a one-command-per-turn boundary so each emitted request becomes visible and receives a real user answer before the next stage executes.
Keep later commands in separate stage references for reliable behavior across local and remote models.
The readable answer Markdown is already persisted by CSGClaw as a separate local-user message, so the skill neither reconstructs nor echoes it.

Clients can submit this object directly to:

```text
POST /api/v1/channels/{channel}/activities/{activity_id}:respond
```

The request body is the response object itself, with no extra room, responder, submission, or behavior wrapper.
CSGClaw derives the room and current responder from the stored activity.

## Persisted dialogue and JSONL

Each request is stored as a separate message owned by the requesting agent.
Its `content` is readable Markdown from the moment the request becomes pending, while `metadata.csgclaw.agent_activity` retains the structured activity used by the interactive UI.
Resolving, skipping, expiring, canceling, or interrupting the request updates the same message ID.

```markdown
## Questions

- demo_kind：What kind of CSGClaw demo should this be?
  - Bug fix (Recommended) (Plans a focused repair workflow.)
  - New feature (Plans a user-facing feature.)
- freeform_note：Add a freeform note.
- test_secret：Enter a disposable test value only.
```

A non-empty submission creates one separate message owned by the authenticated current local user.
The answer message stays in the same room and thread as the question and is marked by `metadata.csgclaw.request_user_input`.
It updates the UI through normal IM events but is never dispatched as a second participant prompt.
The automatic continuation remains the only new agent turn.

```markdown
## Answers

- demo_kind：Bug fix (Recommended) (Plans a focused repair workflow.)
- destination：QA / 验收 (Custom answer)
- freeform_note：Skipped (No answer provided)
- test_secret：Secret recorded (Secret value redacted)
```

Answer lines use the exact full-width separator `：`.
Selected options preserve their original labels and descriptions.
A missing option description becomes `No description provided`.
A freeform value removes one leading `user_note: ` prefix and uses `Custom answer` as its description.
If an option and note are both present, the label becomes `<option>; <note>` and retains the option description.
An individually skipped question becomes `Skipped (No answer provided)`.
A submitted secret becomes `Secret recorded (Secret value redacted)`, while a skipped secret remains `Skipped (No answer provided)`.
An empty outer `answers` object skips the whole request and creates no local-user answer message or continuation.

The following two physical lines illustrate the durable JSONL representation after a non-secret answer.
Fields unrelated to the example, such as attachments, are omitted by normal `omitempty` behavior.

```jsonl
{"id":"question-request-1","sender_id":"u-manager","content":"## Questions\n\n- demo_kind：What kind of CSGClaw demo should this be?\n  - Bug fix (Recommended) (Plans a focused repair workflow.)","metadata":{"csgclaw":{"agent_activity":{"type":"com.opencsg.csgclaw.agent.activity","version":1,"event_id":"question-request-1","sender":"u-manager","channel":"csgclaw","room_id":"room-1","origin_server_ts":1784736000000,"content":{"msgtype":"com.opencsg.csgclaw.agent.question","body":"Question answered","question":{"id":"request-1","status":"answered","questions":[{"id":"demo_kind","header":"Demo kind","question":"What kind of CSGClaw demo should this be?","options":[{"label":"Bug fix (Recommended)","description":"Plans a focused repair workflow."}]}],"answers":{"demo_kind":{"answered":true,"option_index":1,"option_label":"Bug fix (Recommended)"}}}}}}},"created_at":"2026-07-22T12:00:00Z","mentions":[]}
{"id":"answer-request-1","sender_id":"user-admin","content":"## Answers\n\n- demo_kind：Bug fix (Recommended) (Plans a focused repair workflow.)","metadata":{"csgclaw":{"request_user_input":{"kind":"answer","request_id":"request-1"}}},"created_at":"2026-07-22T12:01:00Z","mentions":[]}
```

For a threaded request, the answer row additionally contains `"relates_to":{"rel_type":"m.thread","event_id":"<thread-root-id>"}`.
The exact received `RequestUserInputResponse` remains transient broker input and is not persisted because it may contain a secret.
Only its secret-redacted, wire-compatible copy enters the automatic model continuation.
The later workflow result is a separate agent message only when the continuation advances the workflow.
CSGClaw does not synthesize an agent-owned echo of the user answer.

The built-in demo skill remains the full executable reference for the wire protocol, readable transcript ownership, and agent-directed multi-stage continuation.

## `resource_link`

The payload uses CSGClaw's source-compatible `ResourceLink` field names:

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

### Link fields

| Field | Required | Meaning |
| --- | --- | --- |
| `type` | Yes | Must be the literal `resource_link`. |
| `name` | Yes | Stable machine-readable resource name and fallback label. |
| `uri` | Yes | Absolute HTTP(S) URL. |
| `title` | No | Preferred visible link label. |
| `description` | No | Context rendered after the link. |
| `mimeType` | No | MIME type of the resource. |
| `size` | No | Resource size in bytes. |
| `annotations` | No | Standard audience, priority, and modification hints. |
| `_meta` | No | Application metadata retained unchanged. |
| `icons` | No | Resource icon candidates. |

CSGClaw accepts at most 16 unique links per turn and deduplicates them by `uri`.
The first icon with an absolute HTTP(S) `src` is rendered beside the Markdown link.
Unsafe link or icon schemes are not rendered.

A minimal link needs only `type`, `name`, and `uri`:

```text
::csgclaw-output::resource_link {"type":"resource_link","name":"docs","uri":"https://example.com/docs"}
```

## Limits and compatibility

- Each control record is limited to 256 KiB.
- One `request_user_input` request is accepted per successful turn.
- A request contains 1 through 32 questions and at most 12 options per question.
- A turn contains at most 16 deduplicated HTTP(S) resource links.
- The current Codex runtime adapter decodes `commandExecution` output and the legacy `exec_command_end` and `function_call_output` event shapes.
- Other runtime adapters can implement the same line protocol without changing a skill or its emitter.
- Native blocking Codex `item/tool/requestUserInput` requests reuse the same CSGClaw activity and response model but do not use this stdout protocol.
- Permission actions and privileged `csgclaw.action_card` activities are separate protocols.

Keep the documented JSON field names stable so the same skill output can be handled consistently across runtime adapters.
