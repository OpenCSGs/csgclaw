# CSGClaw Structured Skill Output Protocol

English | [中文](structured-output.zh.md)

CSGClaw provides a line-oriented stdout protocol for skill scripts that need to attach resource links or open an interactive question flow.
The protocol belongs to CSGClaw and is not bound to a particular skill engine, model, or agent runtime.
It is not MCP and does not rely on arbitrary JSON detection.

## Canonical executable example

The built-in Manager skill [`csgclaw-interactive-output-demo`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/) is the complete reference implementation.
Its [`emit_demo.py`](../internal/template/embed/manager/codex/skills/csgclaw-interactive-output-demo/scripts/emit_demo.py) documents every supported request and resource-link field at its first use, emits full and minimal link variants, asks five questions, and demonstrates the automatic continuation response.
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
import os


if os.environ.get("CSGCLAW_STRUCTURED_OUTPUT_PROTOCOL") != "1":
    raise RuntimeError("CSGClaw structured output protocol version 1 is unavailable")


def emit(kind: str, payload: dict[str, object]) -> None:
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    print(f"::csgclaw-output::{kind} {encoded}")
```

Runtime adapters that implement this protocol inject `CSGCLAW_STRUCTURED_OUTPUT_PROTOCOL=1` into skill command environments.
Portable emitters should check this capability before writing control records and return an ordinary diagnostic instead when it is absent.
This handshake prevents a script from claiming that interactive output is ready when an older runtime would treat every control record as plain stdout.

## Turn lifecycle

1. A skill script prints ordinary output and zero or more control records.
2. A CSGClaw runtime adapter decodes records only from a successfully completed command execution.
3. Valid records are buffered until the entire agent turn succeeds.
4. CSGClaw persists the normal final response first.
5. Resource links are appended to the end of that response as Markdown, including the first safe HTTP(S) icon when one is provided.
6. A detached question request is activated after the final response and links are visible.
7. A submitted answer updates the existing question activity, adds a readable redacted summary to history, and automatically continues the same originating agent session with the exact response JSON.

Failed, canceled, superseded, interrupted, expired, or stale turns do not activate buffered output or automatic continuations.
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

The exact response JSON is supplied to the same originating agent session in an automatic continuation prompt.
Persisted question activity and human-readable summaries redact secret values, but the continuing skill must also avoid echoing a secret into its final response, logs, or later tool calls.
The demo skill shows the recommended two-tier result: machine-readable redacted JSON followed by concise human-readable Markdown.

Clients can submit this object directly to:

```text
POST /api/v1/channels/{channel}/activities/{activity_id}:respond
```

The request body is the response object itself, with no extra room, responder, submission, or behavior wrapper.
CSGClaw derives the room and current responder from the stored activity.

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
