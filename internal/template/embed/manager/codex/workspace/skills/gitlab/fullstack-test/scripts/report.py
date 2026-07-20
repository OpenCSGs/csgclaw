"""Generate a detailed Chinese test report in Markdown format.

Usage: python3 scripts/report.py \
    --mr-info '{fetch_mr output JSON}' \
    --strategy '{detect output JSON}' \
    --code-review '{code review JSON}' \
    [--test-results '{test output JSON}']

When --test-results is omitted, the report focuses on code review only
and skips regression test sections.
"""

from __future__ import annotations

import argparse
import json
import sys
from datetime import UTC, datetime
from pathlib import Path

SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

from shared_gitlab_utils import WORKSPACE


_VERDICT_LABELS = {
    "通过": "通过 - 修复有效，无回归问题",
    "部分通过": "部分通过 - 修复了部分问题，无回归，但仍有测试失败",
    "未通过": "未通过 - 存在回归或构建失败",
    "仅构建通过": "仅构建通过 - 项目无测试用例，建议补充",
    "仅分析": "仅分析 - 无法识别技术栈或无可用测试",
}

_STACK_LABELS = {
    "python": "Python",
    "node": "Node.js",
    "go": "Go",
    "unknown": "未知",
}

_FRAMEWORK_LABELS = {
    "vue": "Vue",
    "react": "React",
    "next": "Next.js",
    "python": "Python",
    "go": "Go",
    "generic": "通用",
    "unknown": "未知",
}


_REVIEW_VERDICT_LABELS = {
    "pass": "通过 - 代码变更正确且完整地解决了 Issue",
    "partial": "部分通过 - 代码变更部分解决了 Issue，但存在不足",
    "fail": "未通过 - 代码变更未能解决 Issue 或存在明显问题",
}

_ISSUE_TYPE_LABELS = {
    "bug_fix": "Bug 修复",
    "new_feature": "新功能",
    "improvement": "优化改进",
    "other": "其他",
}

_REVIEW_ITEM_VERDICT_LABELS = {
    "pass": "通过",
    "warning": "警告",
    "fail": "未通过",
}


def _phase_status_label(status: str) -> str:
    """Convert phase status to Chinese label."""
    labels = {"success": "成功", "failed": "失败", "skipped": "跳过"}
    return labels.get(status, status)


def _change_type_label(change: dict) -> str:
    """Determine change type label in Chinese."""
    if change.get("new_file"):
        return "新增"
    if change.get("deleted_file"):
        return "删除"
    if change.get("renamed_file"):
        return "重命名"
    return "修改"


def generate_report(
    mr_info: dict,
    strategy: dict,
    code_review: dict | None = None,
    test_results: dict | None = None,
) -> str:
    """Generate the full Chinese test report.

    Args:
        mr_info: Output from fetch_mr.py.
        strategy: Output from detect.py.
        code_review: Output from Step 4 AI code review (optional).
        test_results: Output from test.py (optional). When None, regression
            test sections are omitted and the report focuses on code review.

    Returns:
        Markdown report string.
    """
    mr = mr_info.get("mr", {})
    issue = mr_info.get("issue")
    project = mr_info.get("project", {})
    changes = mr_info.get("changes", [])

    has_test_results = test_results is not None
    target_results = test_results.get("target_results", {}) if has_test_results else {}
    source_results = test_results.get("source_results", {}) if has_test_results else {}
    comparison = test_results.get("comparison", {}) if has_test_results else {}
    verdict = test_results.get("verdict", "未知") if has_test_results else None

    stack = strategy.get("stack", "unknown")
    framework = strategy.get("framework", "unknown")
    test_runner = strategy.get("test_runner", "无")
    confidence = strategy.get("confidence", "unknown")

    timestamp = datetime.now(tz=UTC).strftime("%Y-%m-%d %H:%M:%S UTC")

    lines = []
    report_title = "# MR 测试验证报告" if has_test_results else "# MR 代码审查报告"
    lines.append(report_title)
    lines.append("")

    # Basic info
    lines.append("## 基本信息")
    lines.append("")
    lines.append("| 项目 | 内容 |")
    lines.append("|------|------|")
    lines.append(f"| **项目** | {project.get('path_with_namespace', '')} |")
    lines.append(f"| **MR** | !{mr.get('iid', '')} - {mr.get('title', '')} |")
    lines.append(f"| **MR 地址** | [{mr.get('web_url', '')}]({mr.get('web_url', '')}) |")
    lines.append(f"| **源分支** | `{mr.get('source_branch', '')}` |")
    lines.append(f"| **目标分支** | `{mr.get('target_branch', '')}` |")
    lines.append(f"| **提交者** | {mr.get('author', '')} |")

    if issue:
        lines.append(f"| **关联 Issue** | #{issue.get('iid', '')} - {issue.get('title', '')} |")

    lines.append(f"| **测试时间** | {timestamp} |")
    lines.append("")

    # Issue description
    if issue and issue.get("description"):
        lines.append("## 关联 Issue 描述")
        lines.append("")
        lines.append(issue["description"])
        lines.append("")

    # Stack detection
    lines.append("## 技术栈检测")
    lines.append("")
    lines.append("| 项目 | 内容 |")
    lines.append("|------|------|")
    lines.append(f"| **语言** | {_STACK_LABELS.get(stack, stack)} |")
    lines.append(f"| **框架** | {_FRAMEWORK_LABELS.get(framework, framework)} |")
    lines.append(f"| **测试框架** | {test_runner or '无'} |")
    lines.append(f"| **检测置信度** | {confidence} |")
    lines.append("")

    # Changed files
    lines.append("## 变更文件清单")
    lines.append("")
    if changes:
        lines.append("| 文件路径 | 变更类型 |")
        lines.append("|----------|----------|")
        for c in changes:
            path = c.get("new_path") or c.get("old_path", "")
            change_type = _change_type_label(c)
            lines.append(f"| `{path}` | {change_type} |")
        lines.append("")
        lines.append(f"**变更摘要**: 共涉及 {len(changes)} 个文件。")
    else:
        lines.append("无变更文件信息。")
    lines.append("")

    # AI Code Review section
    if code_review:
        lines.append("## AI 代码审查")
        lines.append("")

        issue_type = code_review.get("issue_type", "other")
        issue_summary = code_review.get("issue_summary", "")
        overall_verdict = code_review.get("overall_verdict", "")
        overall_comment = code_review.get("overall_comment", "")
        review_items = code_review.get("review_items", [])
        suggestions = code_review.get("suggestions", [])

        lines.append("### 基本判断")
        lines.append("")
        lines.append("| 项目 | 内容 |")
        lines.append("|------|------|")
        lines.append(f"| **Issue 类型** | {_ISSUE_TYPE_LABELS.get(issue_type, issue_type)} |")
        lines.append(f"| **Issue 摘要** | {issue_summary} |")
        review_verdict_label = _REVIEW_VERDICT_LABELS.get(overall_verdict, overall_verdict)
        lines.append(f"| **审查结论** | {review_verdict_label} |")
        lines.append("")

        if overall_comment:
            lines.append(f"**总体评价：** {overall_comment}")
            lines.append("")

        if review_items:
            lines.append("### 逐文件审查")
            lines.append("")
            lines.append("| 文件 | 判定 | 说明 |")
            lines.append("|------|------|------|")
            for item in review_items:
                file_path = item.get("file", "")
                item_verdict = _REVIEW_ITEM_VERDICT_LABELS.get(item.get("verdict", ""), item.get("verdict", ""))
                comment = item.get("comment", "")
                lines.append(f"| `{file_path}` | {item_verdict} | {comment} |")
            lines.append("")

        if suggestions:
            lines.append("### 改进建议")
            lines.append("")
            lines.extend(f"- {s}" for s in suggestions)
            lines.append("")

    # Test execution results (only when regression test data is available)
    if has_test_results:
        lines.append("## 测试执行结果")
        lines.append("")

        # Environment preparation
        lines.append("### 环境准备")
        lines.append("")
        lines.append("| 阶段 | 修复前 (target) | 修复后 (source) |")
        lines.append("|------|----------------|----------------|")

        for phase in ["install", "build", "lint"]:
            target_status = _phase_status_label(target_results.get(phase, {}).get("status", "跳过"))
            source_status = _phase_status_label(source_results.get(phase, {}).get("status", "跳过"))
            phase_labels = {"install": "依赖安装", "build": "项目构建", "lint": "代码检查"}
            lines.append(f"| {phase_labels.get(phase, phase)} | {target_status} | {source_status} |")
        lines.append("")

        # Unit test comparison
        target_test = target_results.get("test", {})
        source_test = source_results.get("test", {})
        target_counts = target_test.get("counts", {})
        source_counts = source_test.get("counts", {})

        lines.append("### 单元测试对比")
        lines.append("")

        if target_test.get("status") == "skipped" and source_test.get("status") == "skipped":
            lines.append("项目未检测到可用的测试命令，跳过单元测试阶段。")
        else:
            lines.append("| 指标 | 修复前 (target) | 修复后 (source) | 变化 |")
            lines.append("|------|----------------|----------------|------|")

            for metric, label in [("passed", "通过"), ("failed", "失败"), ("skipped", "跳过"), ("errors", "错误")]:
                tv = target_counts.get(metric, 0)
                sv = source_counts.get(metric, 0)
                delta = sv - tv
                delta_str = f"+{delta}" if delta > 0 else str(delta) if delta != 0 else "-"
                lines.append(f"| {label} | {tv} | {sv} | {delta_str} |")

            tt = target_counts.get("total", 0)
            st = source_counts.get("total", 0)
            lines.append(f"| **总计** | {tt} | {st} | - |")
        lines.append("")

        # Fix verification
        lines.append("### 修复验证")
        lines.append("")

        fixed_tests = comparison.get("fixed_tests", [])
        if fixed_tests:
            lines.append("**已修复的测试用例：**")
            lines.append("")
            lines.append("| 测试用例 | 修复前状态 | 修复后状态 |")
            lines.append("|----------|-----------|-----------|")
            lines.extend(f"| `{t}` | 失败 | 通过 |" for t in fixed_tests)
        else:
            lines.append("**已修复的测试用例：** 无（目标分支无失败测试或无法解析测试名称）")
        lines.append("")

        regressions = comparison.get("regressions", [])
        if regressions:
            lines.append("**新增失败（回归）：**")
            lines.append("")
            lines.append("| 测试用例 | 修复前状态 | 修复后状态 |")
            lines.append("|----------|-----------|-----------|")
            lines.extend(f"| `{t}` | 通过 | 失败 |" for t in regressions)
        else:
            lines.append("**新增失败（回归）：** 无")
        lines.append("")

        still_failing = comparison.get("still_failing", [])
        if still_failing:
            lines.append("**仍然失败的测试：**")
            lines.append("")
            lines.extend(f"- `{t}`" for t in still_failing)
            lines.append("")

        # Test output details
        lines.append("### 测试输出详情")
        lines.append("")

        target_output = target_test.get("output", "")
        source_output = source_test.get("output", "")

        if target_output:
            lines.append("<details>")
            lines.append("<summary>修复前 (target) 测试输出</summary>")
            lines.append("")
            lines.append("```")
            if len(target_output) > 5000:
                lines.append(target_output[:5000])
                lines.append(f"\n... (输出已截断，共 {len(target_output)} 字符)")
            else:
                lines.append(target_output)
            lines.append("```")
            lines.append("")
            lines.append("</details>")
            lines.append("")

        if source_output:
            lines.append("<details>")
            lines.append("<summary>修复后 (source) 测试输出</summary>")
            lines.append("")
            lines.append("```")
            if len(source_output) > 5000:
                lines.append(source_output[:5000])
                lines.append(f"\n... (输出已截断，共 {len(source_output)} 字符)")
            else:
                lines.append(source_output)
            lines.append("```")
            lines.append("")
            lines.append("</details>")
            lines.append("")

    # Conclusion
    if has_test_results:
        lines.append("## 测试结论")
        lines.append("")
        verdict_label = _VERDICT_LABELS.get(verdict, verdict)
        lines.append(f"**判定结果：{verdict_label}**")
        lines.append("")

        # Recommendations
        lines.append("### 建议")
        lines.append("")
        regressions = comparison.get("regressions", [])
        if verdict == "通过":
            lines.append("- 修复验证通过，可以进行代码评审并合并。")
        elif verdict == "部分通过":
            lines.append("- 修复了部分问题但仍有测试失败，建议检查仍然失败的测试用例。")
            lines.append("- 确认仍然失败的测试是否与本次修复相关。")
        elif verdict == "未通过":
            if regressions:
                lines.append("- 修复引入了新的回归问题，请检查以上列出的回归测试用例。")
            source_build = source_results.get("build", {})
            if source_build.get("status") == "failed":
                lines.append("- 项目构建失败，请先修复构建错误。")
            lines.append("- 建议修复后重新提交并再次运行测试验证。")
        elif verdict == "仅构建通过":
            lines.append("- 项目构建成功但无自动化测试用例。")
            lines.append("- 强烈建议为修复的 Bug 补充对应的测试用例。")
        elif verdict == "仅分析":
            lines.append("- 无法自动检测项目技术栈或无可用测试。")
            lines.append("- 建议手动验证修复是否正确。")
    else:
        # Code-review-only conclusion
        lines.append("## 审查结论")
        lines.append("")
        if code_review:
            overall = code_review.get("overall_verdict", "")
            review_label = _REVIEW_VERDICT_LABELS.get(overall, overall)
            lines.append(f"**判定结果：{review_label}**")
        else:
            lines.append("**判定结果：仅分析 - 代码审查未执行**")
        lines.append("")
        lines.append("### 建议")
        lines.append("")
        lines.append("- 本次报告仅包含代码审查，未执行回归测试。")
        lines.append("- 如需回归测试验证，请单独使用测试模式运行。")
    lines.append("")

    lines.append("---")
    lines.append(f"*报告由 gitlab 自动生成 | {timestamp}*")

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate Chinese test report for MR verification")
    parser.add_argument("--mr-info", required=True, help="JSON string from fetch_mr.py output")
    parser.add_argument(
        "--test-results", required=False, default=None, help="JSON string from test.py output (optional)",
    )
    parser.add_argument("--strategy", required=True, help="JSON string from detect.py output")
    parser.add_argument("--code-review", required=False, default=None, help="JSON string from Step 4 AI code review")
    args = parser.parse_args()

    try:
        mr_info = json.loads(args.mr_info)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"Invalid mr-info JSON: {e}"}), file=sys.stderr)
        sys.exit(1)

    test_results = None
    if args.test_results:
        try:
            test_results = json.loads(args.test_results)
        except json.JSONDecodeError as e:
            print(json.dumps({"error": f"Invalid test-results JSON: {e}"}), file=sys.stderr)
            sys.exit(1)

    try:
        strategy = json.loads(args.strategy)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"Invalid strategy JSON: {e}"}), file=sys.stderr)
        sys.exit(1)

    code_review = None
    if args.code_review:
        try:
            code_review = json.loads(args.code_review)
        except json.JSONDecodeError as e:
            print(json.dumps({"error": f"Invalid code-review JSON: {e}"}), file=sys.stderr)
            sys.exit(1)

    report = generate_report(mr_info, strategy, code_review, test_results)

    # Save report to file
    mr_iid = mr_info.get("mr", {}).get("iid", "unknown")
    project_name = mr_info.get("project", {}).get("name", "unknown")
    reports_dir = WORKSPACE / "reports"
    reports_dir.mkdir(parents=True, exist_ok=True)
    report_path = reports_dir / f"{project_name}-mr-{mr_iid}-report.md"
    report_path.write_text(report, encoding="utf-8")

    result = {
        "report_path": str(report_path),
        "report_content": report,
    }
    print(json.dumps(result, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
