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
LANGUAGES = ("en", "zh")


def localize(language: str, english: str, chinese: str) -> str:
    """Select one of the demo's two user-facing languages."""

    return chinese if language == "zh" else english


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


def emit_start(language: str) -> None:
    """Emit resource links and option-based questions for stage 1."""

    # Ordinary stdout becomes the readable response when CSGClaw closes the
    # turn at the structured question boundary. Control records stay hidden.
    print(
        localize(
            language,
            "## Interactive output demo - step 1 of 3",
            "## 交互式输出演示 - 第 1/3 步",
        )
    )
    print()
    print(localize(language, "Choose the workflow branch.", "请选择工作流分支。"))
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
                    "header": localize(language, "Demo kind", "演示类型"),
                    # Required concrete UI title.
                    "question": localize(
                        language,
                        "What workflow should the demo execute?",
                        "演示应该执行哪种工作流？",
                    ),
                    # Show a freeform alternative when true or options are absent.
                    "isOther": False,
                    # Use password input and redact persisted values when true.
                    "isSecret": False,
                    # Optional list of at most 12 choices; null means freeform-only.
                    "options": [
                        {
                            # Exact submitted value; the suffix adds the badge.
                            "label": localize(
                                language,
                                "Bug fix (Recommended)",
                                "修复缺陷 (Recommended)",
                            ),
                            # Optional supporting text.
                            "description": localize(
                                language,
                                "Follow a focused repair workflow with reproduction and verification.",
                                "执行包含复现和验证的聚焦修复流程。",
                            ),
                        },
                        {
                            "label": localize(language, "New feature", "新功能"),
                            "description": localize(
                                language,
                                "Plan a user-facing capability from goal to test coverage.",
                                "规划从目标到测试覆盖的用户功能。",
                            ),
                        },
                        {
                            "label": localize(language, "Code review", "代码审查"),
                            "description": localize(
                                language,
                                "Inspect changes, concrete risks, and priorities.",
                                "检查变更、具体风险和优先级。",
                            ),
                        },
                    ],
                },
            ],
            # Optional timeout from 60000 through 240000 ms.
            # Omit it so this manual demo does not expire.
            # "autoResolutionMs": 240000,
        },
    )


def emit_context(workflow: str, language: str) -> None:
    """Emit option-or-freeform and freeform-only questions for stage 2."""

    print(
        localize(
            language,
            "## Interactive output demo - step 2 of 3",
            "## 交互式输出演示 - 第 2/3 步",
        )
    )
    print()
    print(
        localize(
            language,
            "Configure verification, destination, an optional freeform note, and presentation.",
            "请配置验证方式、目标位置、可选备注和展示方式。",
        )
    )
    emit_resource_links()
    emit(
        "request_user_input",
        {
            "questions": [
                {
                    "id": "verification",
                    "header": localize(language, "Checks", "检查"),
                    "question": localize(
                        language,
                        "How cautious should verification be?",
                        "验证应该多严格？",
                    ),
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": localize(language, "Standard", "标准"),
                            "description": localize(
                                language,
                                "Use targeted checks, normal punctuation, and practical coverage.",
                                "使用有针对性的检查、正常标点和实用覆盖。",
                            ),
                        },
                        {
                            "label": localize(
                                language, "Strict + Unicode 中文", "严格 + Unicode 中文"
                            ),
                            "description": localize(
                                language,
                                "Add broader verification, edge cases, and explicit acceptance criteria.",
                                "增加更广泛的验证、边界场景和明确的验收标准。",
                            ),
                        },
                        {
                            "label": localize(language, "Fast, focused", "快速、聚焦"),
                            "description": localize(
                                language,
                                "Keep validation lightweight and emphasize speed.",
                                "保持轻量验证并优先考虑速度。",
                            ),
                        },
                    ],
                },
                {
                    "id": "destination",
                    "header": localize(language, "Destination", "目标位置"),
                    "question": localize(
                        language,
                        f"Where should the {workflow} demo result go?",
                        "演示结果应该发送到哪里？",
                    ),
                    "isOther": True,
                    "isSecret": False,
                    "options": [
                        {
                            "label": localize(language, "Current room", "当前房间"),
                            "description": localize(
                                language,
                                "Keep the result in this CSGClaw conversation.",
                                "将结果保留在当前 CSGClaw 对话中。",
                            ),
                        },
                        {
                            "label": localize(
                                language, "Thread: QA / 验收", "线程：QA / 验收"
                            ),
                            "description": localize(
                                language,
                                "Use the option as written, including spaces, slash, and Unicode.",
                                "按原样使用包含空格、斜杠和 Unicode 的选项。",
                            ),
                        },
                    ],
                },
                {
                    "id": "freeform_note",
                    "header": localize(language, "Freeform", "自由输入"),
                    "question": localize(
                        language,
                        "Add an optional note with spaces, punctuation, or Unicode.",
                        "添加一条可包含空格、标点或 Unicode 的可选备注。",
                    ),
                    "isOther": True,
                    "isSecret": False,
                    "options": None,
                },
                {
                    "id": "presentation",
                    "header": localize(language, "Presentation", "展示方式"),
                    "question": localize(
                        language,
                        "How should the final execution receipt be presented?",
                        "最终执行回执应该如何展示？",
                    ),
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": localize(
                                language, "Concise (Recommended)", "简洁 (Recommended)"
                            ),
                            "description": localize(
                                language,
                                "Show a short branch and action receipt.",
                                "显示简短的分支和操作回执。",
                            ),
                        },
                        {
                            "label": localize(language, "Detailed", "详细"),
                            "description": localize(
                                language,
                                "Show every allowlisted selection in the receipt.",
                                "在回执中显示所有白名单选项。",
                            ),
                        },
                        {
                            "label": localize(
                                language,
                                "Bilingual 中文 + English",
                                "双语 中文 + English",
                            ),
                            "description": localize(
                                language,
                                "Exercise spaces, punctuation, and Unicode in an ordinary option.",
                                "在普通选项中测试空格、标点和 Unicode。",
                            ),
                        },
                    ],
                },
            ]
        },
    )


def emit_confirmation(
    workflow: str,
    destination: str,
    verification: str,
    presentation: str,
    language: str,
) -> None:
    """Emit final action options and optional secret input for stage 3."""

    print(
        localize(
            language,
            "## Interactive output demo - step 3 of 3",
            "## 交互式输出演示 - 第 3/3 步",
        )
    )
    print()
    print(
        localize(
            language,
            "Choose the final action and optionally enter a disposable secret test value.",
            "请选择最终操作，并可选择输入一次性秘密测试值。",
        )
    )
    emit(
        "request_user_input",
        {
            "questions": [
                {
                    "id": "final_action",
                    "header": localize(language, "Final action", "最终操作"),
                    "question": localize(
                        language,
                        "What should the demo execute next?",
                        "演示接下来应该执行什么？",
                    ),
                    "isOther": False,
                    "isSecret": False,
                    "options": [
                        {
                            "label": localize(
                                language,
                                "Execute demo (Recommended)",
                                "执行演示 (Recommended)",
                            ),
                            "description": localize(
                                language,
                                "Complete the selected branch and show its execution receipt.",
                                "完成所选分支并显示执行回执。",
                            ),
                        },
                        {
                            "label": localize(language, "Revise context", "修改上下文"),
                            "description": localize(
                                language,
                                "Finish with a receipt requesting revised context.",
                                "通过请求修改上下文的回执结束。",
                            ),
                        },
                        {
                            "label": localize(language, "Stop here", "在此停止"),
                            "description": localize(
                                language,
                                "Finish without executing the selected demo branch.",
                                "结束且不执行所选演示分支。",
                            ),
                        },
                    ],
                },
                {
                    "id": "test_secret",
                    "header": localize(language, "Test secret", "秘密测试值"),
                    "question": localize(
                        language,
                        "Optionally enter a disposable test value only - never a real credential.",
                        "可选择输入一次性测试值，但绝不能输入真实凭据。",
                    ),
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
    language: str,
) -> None:
    """Print a safe final receipt selected by the agent, never by parsed JSON."""

    print(
        "FINAL_RECEIPT_EMITTED. STOP CURRENT TURN. Return only the Markdown below "
        "and do not execute another command."
    )
    print(
        localize(
            language, "## Interactive output demo complete", "## 交互式输出演示完成"
        )
    )
    print()
    print(
        localize(
            language, f"- Workflow branch: `{workflow}`", f"- 工作流分支：`{workflow}`"
        )
    )
    print(
        localize(
            language,
            f"- Destination branch: `{destination}`",
            f"- 目标分支：`{destination}`",
        )
    )
    print(
        localize(
            language,
            f"- Verification branch: `{verification}`",
            f"- 验证分支：`{verification}`",
        )
    )
    print(
        localize(
            language,
            f"- Presentation branch: `{presentation}`",
            f"- 展示分支：`{presentation}`",
        )
    )
    print(
        localize(
            language, f"- Executed action: `{action}`", f"- 已执行操作：`{action}`"
        )
    )
    print(
        localize(
            language,
            "- Secret handling: no secret value was passed to this script",
            "- 秘密值处理：未向此脚本传递任何秘密值",
        )
    )


def parse_args() -> argparse.Namespace:
    """Parse only allowlisted stage selectors chosen by the agent."""

    parser = argparse.ArgumentParser()
    subparsers = parser.add_subparsers(dest="stage", required=True)
    start = subparsers.add_parser("start")
    start.add_argument("--language", choices=LANGUAGES, default="en")

    context = subparsers.add_parser("context")
    context.add_argument("--workflow", required=True, choices=WORKFLOWS)
    context.add_argument("--language", choices=LANGUAGES, default="en")

    confirm = subparsers.add_parser("confirm")
    confirm.add_argument("--workflow", required=True, choices=WORKFLOWS)
    confirm.add_argument("--destination", required=True, choices=DESTINATIONS)
    confirm.add_argument("--verification", required=True, choices=VERIFICATIONS)
    confirm.add_argument("--presentation", required=True, choices=PRESENTATIONS)
    confirm.add_argument("--language", choices=LANGUAGES, default="en")

    finish = subparsers.add_parser("complete")
    finish.add_argument("--workflow", required=True, choices=WORKFLOWS)
    finish.add_argument("--destination", required=True, choices=DESTINATIONS)
    finish.add_argument("--verification", required=True, choices=VERIFICATIONS)
    finish.add_argument("--presentation", required=True, choices=PRESENTATIONS)
    finish.add_argument("--action", required=True, choices=ACTIONS)
    finish.add_argument("--language", choices=LANGUAGES, default="en")
    return parser.parse_args()


def main() -> None:
    """Emit one requested stage without reading any prior response."""

    args = parse_args()
    if args.stage == "start":
        emit_start(args.language)
    elif args.stage == "context":
        emit_context(args.workflow, args.language)
    elif args.stage == "confirm":
        emit_confirmation(
            args.workflow,
            args.destination,
            args.verification,
            args.presentation,
            args.language,
        )
    else:
        complete(
            args.workflow,
            args.destination,
            args.verification,
            args.presentation,
            args.action,
            args.language,
        )


if __name__ == "__main__":
    main()
