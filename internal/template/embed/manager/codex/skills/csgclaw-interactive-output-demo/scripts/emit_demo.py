#!/usr/bin/env python3
"""Emit the complete CSGClaw structured-output protocol and acceptance demo.

This executable is also a reference implementation for skill authors.
Each supported protocol field is documented at its first use below.
"""

import json


# A skill returns ordinary Markdown after the emitter command finishes. CSGClaw
# persists this response first, appends resource links, and then opens questions.
INITIAL_RESPONSE_MARKDOWN = """## Interactive output demo

The complete interactive output demo is ready."""


# This is the exact Codex RequestUserInputResponse shape delivered to the skill's
# automatic continuation. Values here are disposable examples, not credentials.
EXAMPLE_REQUEST_USER_INPUT_RESPONSE = {
    "answers": {  # Map keyed by the stable question id from each question below.
        "demo_kind": {
            # Codex uses an array: [] skips; CSGClaw currently sends zero or one
            # value, while retaining wire compatibility for future multi-values.
            # Use an option label, or "user_note: " followed by freeform text.
            "answers": ["Bug fix (Recommended)"],
        },
        "verification": {"answers": ["Strict + Unicode 中文"]},
        "destination": {"answers": ["user_note: Documentation example / 示例"]},
        "freeform_note": {
            "answers": ["user_note: Show the exact JSON response shape."]
        },
        "test_secret": {"answers": ["user_note: disposable-example-only"]},
    }
}


# Human-facing labels for the suggested Markdown presentation.
QUESTION_LABELS = {
    "demo_kind": "Demo kind",
    "verification": "Verification",
    "destination": "Destination",
    "freeform_note": "Note",
    "test_secret": "Test secret",
}


def render_answer_markdown(response: dict[str, object]) -> str:
    """Return structured JSON followed by a suggested Markdown presentation.

    A real continuation receives the exact response JSON. It must redact secret
    values before placing the response in Markdown or any other persisted text.
    """

    safe_response = json.loads(json.dumps(response, ensure_ascii=False))
    answers = safe_response.get("answers")
    secret_answer = answers.get("test_secret") if isinstance(answers, dict) else None
    if isinstance(secret_answer, dict) and secret_answer.get("answers"):
        secret_answer["answers"] = ["<redacted>"]
    encoded = json.dumps(safe_response, ensure_ascii=False, indent=2)
    lines = [
        "## Submitted `RequestUserInputResponse`",
        "",
        "```json",
        encoded,
        "```",
        "",
        "## Suggested Markdown presentation",
        "",
    ]
    if isinstance(answers, dict):
        for question_id, answer in answers.items():
            label = QUESTION_LABELS.get(question_id, question_id)
            values = answer.get("answers") if isinstance(answer, dict) else None
            if question_id == "test_secret" and values:
                display = "Secret recorded"
            elif not isinstance(values, list) or not values:
                display = "Skipped"
            else:
                display_values = [
                    value.removeprefix("user_note: ")
                    for value in values
                    if isinstance(value, str)
                ]
                display = ", ".join(display_values) or "Skipped"
            lines.append(f"- **{label}:** {display}")
    return "\n".join(lines)


# Concrete Markdown example for developers to copy or adapt in another skill.
ANSWER_RESPONSE_MARKDOWN_EXAMPLE = render_answer_markdown(
    EXAMPLE_REQUEST_USER_INPUT_RESPONSE
)


def emit(kind: str, payload: dict[str, object]) -> None:
    """Print one single-line CSGClaw control record."""

    # `kind` selects a registered decoder: request_user_input or resource_link.
    # `payload` remains source-compatible with the corresponding Codex/MCP type.
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    print(f"::csgclaw-output::{kind} {encoded}")


def main() -> None:
    """Emit ordinary stdout followed by links and one question request."""

    print("Interactive output demo controls emitted successfully.")

    emit(
        "resource_link",
        {
            # Required ResourceLink discriminator.
            "type": "resource_link",
            # Required stable machine-readable name.
            "name": "csgclaw-repository",
            # Optional display title.
            "title": "CSGClaw - structured output source",
            # Required absolute HTTP(S) URL.
            "uri": "https://github.com/OpenCSGs/csgclaw",
            # Optional human-readable context.
            "description": "Source code, issues, and implementation details for CSGClaw.",
            # Optional MIME type of the linked resource.
            "mimeType": "text/html",
            # Optional resource size in bytes.
            "size": 2048,
            # Optional standard MCP presentation hints.
            "annotations": {
                # Intended roles: user and/or assistant.
                "audience": ["user"],
                # Relative importance from 0.0 through 1.0.
                "priority": 0.9,
                # RFC 3339 timestamp.
                "lastModified": "2026-07-20T00:00:00Z",
            },
            # Optional application metadata passed through unchanged.
            "_meta": {"demo": True, "variant": "full"},
            # Optional icon candidates for the resource.
            "icons": [
                {
                    # Required icon URL.
                    "src": "https://github.githubassets.com/favicons/favicon.svg",
                    # Optional icon MIME type.
                    "mimeType": "image/svg+xml",
                    # Optional supported icon sizes.
                    "sizes": ["any"],
                    # Optional light or dark presentation theme.
                    "theme": "dark",
                }
            ],
        },
    )

    # Minimal ResourceLink: only type, name, and uri are required by CSGClaw.
    emit(
        "resource_link",
        {
            "type": "resource_link",
            "name": "opencsg-home",
            "uri": "https://opencsg.com",
        },
    )

    emit(
        "request_user_input",
        {
            # Required list containing 1 through 32 questions.
            "questions": [
                {
                    # Required unique response-map key.
                    "id": "demo_kind",
                    # Required short activity/history label.
                    "header": "Demo kind",
                    # Required concrete UI title.
                    "question": "What kind of CSGClaw demo should this be?",
                    # Show a freeform alternative when true or options are absent.
                    "isOther": False,
                    # Use password input and redact persisted values when true.
                    "isSecret": False,
                    # Optional list of at most 12 choices; null means freeform-only.
                    "options": [
                        {
                            # Exact submitted value; the suffix adds the badge.
                            "label": "Bug fix (Recommended)",
                            # Optional supporting text.
                            "description": "Plans a focused repair workflow with reproduction and verification.",
                        },
                        {
                            "label": "New feature",
                            "description": "Plans a user-facing feature from goal to test coverage.",
                        },
                        {
                            "label": "Code review",
                            "description": "Shows review findings, concrete risks, and priorities.",
                        },
                    ],
                },
                {
                    "id": "verification",
                    "header": "Checks",
                    "question": "How cautious should verification be?",
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": "Standard",
                            "description": "Uses targeted checks, normal punctuation, and practical coverage.",
                        },
                        {
                            "label": "Strict + Unicode 中文",
                            "description": "Adds broader verification, edge cases, and explicit acceptance criteria.",
                        },
                        {
                            "label": "Fast, focused",
                            "description": "Keeps validation lightweight and emphasizes speed.",
                        },
                    ],
                },
                {
                    "id": "destination",
                    "header": "Destination",
                    "question": "Where should the demo result go?",
                    "isOther": True,
                    "isSecret": False,
                    "options": [
                        {
                            "label": "Current room",
                            "description": "Keep the result in this CSGClaw conversation.",
                        },
                        {
                            "label": "Thread: QA / 验收",
                            "description": "Use the option as written, including spaces, slash, and Unicode.",
                        },
                    ],
                },
                {
                    "id": "freeform_note",
                    "header": "Freeform",
                    "question": "Add a freeform-only note with spaces, punctuation, or Unicode.",
                    "isOther": True,
                    "isSecret": False,
                    "options": None,
                },
                {
                    "id": "test_secret",
                    "header": "Test secret",
                    "question": "Enter a disposable test value only - never a real credential.",
                    "isOther": True,
                    "isSecret": True,
                    "options": None,
                },
            ],
            # Optional timeout from 60000 through 240000 ms.
            # Keep it commented out so this manual demo does not expire.
            # "autoResolutionMs": 240000,
        },
    )


if __name__ == "__main__":
    main()
