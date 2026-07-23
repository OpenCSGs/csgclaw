#!/usr/bin/env python3
"""Emit the three-stage CSGClaw structured-output acceptance demo.

This executable is also a reference implementation for skill authors.
Each supported protocol field is documented at its first use below.
The script deliberately has no response JSON input: the agent reads each
RequestUserInputResponse and selects the next allowlisted stage arguments.
"""

import argparse
import json


WORKFLOWS = ("bug-fix", "new-feature", "code-review", "custom")
DESTINATIONS = ("current-room", "qa-thread", "custom", "unspecified")
VERIFICATIONS = ("standard", "strict", "fast", "unspecified")
PRESENTATIONS = ("concise", "detailed", "bilingual", "unspecified")
ACTIONS = ("execute", "revise", "stop", "skip")


def emit(kind: str, payload: dict[str, object]) -> None:
    """Print one single-line CSGClaw control record."""

    # `kind` selects a registered decoder: request_user_input or resource_link.
    # `payload` remains source-compatible with the corresponding Codex/MCP type.
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    print(f"::csgclaw-output::{kind} {encoded}")


def emit_resource_links() -> None:
    """Emit the full and minimal ResourceLink examples used by stages 1 and 2."""

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


def emit_start() -> None:
    """Emit resource links and option-based questions for stage 1."""

    # Ordinary stdout becomes the readable response when CSGClaw closes the
    # turn at the structured question boundary. Control records stay hidden.
    print("## Interactive output demo - step 1 of 3")
    print()
    print("Choose the workflow branch.")
    emit_resource_links()
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
                    "question": "What workflow should the demo execute?",
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
                            "description": "Follow a focused repair workflow with reproduction and verification.",
                        },
                        {
                            "label": "New feature",
                            "description": "Plan a user-facing capability from goal to test coverage.",
                        },
                        {
                            "label": "Code review",
                            "description": "Inspect changes, concrete risks, and priorities.",
                        },
                    ],
                },
            ],
            # Optional timeout from 60000 through 240000 ms.
            # Omit it so this manual demo does not expire.
            # "autoResolutionMs": 240000,
        },
    )


def emit_context(workflow: str) -> None:
    """Emit option-or-freeform and freeform-only questions for stage 2."""

    print("## Interactive output demo - step 2 of 3")
    print()
    print(
        "Configure verification, destination, an optional freeform note, "
        "and presentation."
    )
    emit_resource_links()
    emit(
        "request_user_input",
        {
            "questions": [
                {
                    "id": "verification",
                    "header": "Checks",
                    "question": "How cautious should verification be?",
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": "Standard",
                            "description": "Use targeted checks, normal punctuation, and practical coverage.",
                        },
                        {
                            "label": "Strict + Unicode 中文",
                            "description": "Add broader verification, edge cases, and explicit acceptance criteria.",
                        },
                        {
                            "label": "Fast, focused",
                            "description": "Keep validation lightweight and emphasize speed.",
                        },
                    ],
                },
                {
                    "id": "destination",
                    "header": "Destination",
                    "question": f"Where should the {workflow} demo result go?",
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
                    "question": "Add an optional note with spaces, punctuation, or Unicode.",
                    "isOther": True,
                    "isSecret": False,
                    "options": None,
                },
                {
                    "id": "presentation",
                    "header": "Presentation",
                    "question": "How should the final execution receipt be presented?",
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": "Concise (Recommended)",
                            "description": "Show a short branch and action receipt.",
                        },
                        {
                            "label": "Detailed",
                            "description": "Show every allowlisted selection in the receipt.",
                        },
                        {
                            "label": "Bilingual 中文 + English",
                            "description": "Exercise spaces, punctuation, and Unicode in an ordinary option.",
                        },
                    ],
                },
            ]
        },
    )


def emit_confirmation(
    workflow: str, destination: str, verification: str, presentation: str
) -> None:
    """Emit final action options and optional secret input for stage 3."""

    print("## Interactive output demo - step 3 of 3")
    print()
    print(
        "Choose the final action and optionally enter a disposable secret test value."
    )
    emit(
        "request_user_input",
        {
            "questions": [
                {
                    "id": "final_action",
                    "header": "Final action",
                    "question": "What should the demo execute next?",
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": "Execute demo (Recommended)",
                            "description": "Complete the selected branch and show its execution receipt.",
                        },
                        {
                            "label": "Revise context",
                            "description": "Finish with a receipt requesting revised context.",
                        },
                        {
                            "label": "Stop here",
                            "description": "Finish without executing the selected demo branch.",
                        },
                    ],
                },
                {
                    "id": "test_secret",
                    "header": "Test secret",
                    "question": "Optionally enter a disposable test value only - never a real credential.",
                    "isOther": True,
                    "isSecret": True,
                    "options": None,
                },
            ]
        },
    )


def complete(
    workflow: str,
    destination: str,
    verification: str,
    presentation: str,
    action: str,
) -> None:
    """Print a safe final receipt selected by the agent, never by parsed JSON."""

    print(
        "FINAL_RECEIPT_EMITTED. STOP CURRENT TURN. Return only the Markdown below "
        "and do not execute another command."
    )
    print("## Interactive output demo complete")
    print()
    print(f"- Workflow branch: `{workflow}`")
    print(f"- Destination branch: `{destination}`")
    print(f"- Verification branch: `{verification}`")
    print(f"- Presentation branch: `{presentation}`")
    print(f"- Executed action: `{action}`")
    print("- Secret handling: no secret value was passed to this script")


def parse_args() -> argparse.Namespace:
    """Parse only allowlisted stage selectors chosen by the agent."""

    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="stage", required=True)
    subparsers.add_parser("start")

    context = subparsers.add_parser("context")
    context.add_argument("--workflow", required=True, choices=WORKFLOWS)

    confirm = subparsers.add_parser("confirm")
    confirm.add_argument("--workflow", required=True, choices=WORKFLOWS)
    confirm.add_argument("--destination", required=True, choices=DESTINATIONS)
    confirm.add_argument("--verification", required=True, choices=VERIFICATIONS)
    confirm.add_argument("--presentation", required=True, choices=PRESENTATIONS)

    finish = subparsers.add_parser("complete")
    finish.add_argument("--workflow", required=True, choices=WORKFLOWS)
    finish.add_argument("--destination", required=True, choices=DESTINATIONS)
    finish.add_argument("--verification", required=True, choices=VERIFICATIONS)
    finish.add_argument("--presentation", required=True, choices=PRESENTATIONS)
    finish.add_argument("--action", required=True, choices=ACTIONS)
    return parser.parse_args()


def main() -> None:
    """Emit one requested stage without reading any prior response."""

    args = parse_args()
    if args.stage == "start":
        emit_start()
    elif args.stage == "context":
        emit_context(args.workflow)
    elif args.stage == "confirm":
        emit_confirmation(
            args.workflow, args.destination, args.verification, args.presentation
        )
    else:
        complete(
            args.workflow,
            args.destination,
            args.verification,
            args.presentation,
            args.action,
        )


if __name__ == "__main__":
    main()
