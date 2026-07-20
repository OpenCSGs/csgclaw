"""Generate a formal Chinese test report for API test mode.

Tech-stack agnostic — the caller (Agent) pre-parses test output and passes
structured results. Supports any test runner (pytest, jest, vitest, go test,
shell scripts, etc.).

Usage: python3 scripts/api_report.py \
    --project-info '{"project_name":"my-app","project_path":"group/my-app","project_id":123}' \
    --test-config '{"commands":{"smoke":"pytest -m smoke"},"setup":"pip install -r req.txt"}' \
    --test-results '[{"command_key":"smoke","command":"pytest -m smoke","status":"success",
        "exit_code":0,"stdout":"...","stderr":"...","duration_seconds":12.3,
        "counts":{"passed":5,"failed":1,"skipped":0,"errors":0,"total":6},
        "failed_tests":[{"name":"test_x","error":"AssertionError: ..."}]}]'
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
    "通过": "通过 - 所有测试均已通过",
    "部分通过": "部分通过 - 存在失败用例，但部分测试通过",
    "未通过": "未通过 - 测试执行失败或存在严重错误",
}


def _determine_verdict(results: list[dict]) -> str:
    """Determine overall verdict from all test results."""
    if not results:
        return "未通过"

    has_failure = False
    has_success = False

    for r in results:
        counts = r.get("counts", {})
        if r.get("status") == "failed" or counts.get("failed", 0) > 0 or counts.get("errors", 0) > 0:
            has_failure = True
        if counts.get("passed", 0) > 0:
            has_success = True

    if has_failure and has_success:
        return "部分通过"
    if has_failure:
        return "未通过"
    return "通过"


def generate_api_report(
    project_info: dict,
    test_config: dict,
    test_results: list[dict],
) -> str:
    """Generate the full Chinese API test report.

    Args:
        project_info: Project metadata (project_name, project_path, project_id).
        test_config: AgenticHub-test.yaml content (commands, setup, report).
        test_results: List of per-command results with counts and failed_tests.

    Returns:
        Markdown report string.
    """
    project_name = project_info.get("project_name", "unknown")
    project_path = project_info.get("project_path", "unknown")
    timestamp = datetime.now(tz=UTC).strftime("%Y-%m-%d %H:%M:%S UTC")
    verdict = _determine_verdict(test_results)

    lines: list[str] = []
    lines.append("# API 测试报告")
    lines.append("")

    # Section 1: Basic info
    lines.append("## 基本信息")
    lines.append("")
    lines.append("| 项目 | 内容 |")
    lines.append("|------|------|")
    lines.append(f"| **项目** | {project_path} |")
    lines.append(f"| **项目名称** | {project_name} |")

    config_source = "AgenticHub-test.yaml" if test_config.get("commands") else "自动生成"
    lines.append(f"| **配置来源** | {config_source} |")
    lines.append(f"| **测试时间** | {timestamp} |")
    lines.append("")

    # Section 2: Test configuration
    commands = test_config.get("commands", {})
    setup_cmd = test_config.get("setup")

    lines.append("## 测试配置")
    lines.append("")

    if setup_cmd:
        lines.append(f"**环境准备命令：** `{setup_cmd}`")
        lines.append("")

    if commands:
        lines.append("| 测试类型 | 命令 |")
        lines.append("|----------|------|")
        for key, cmd in commands.items():
            lines.append(f"| {key} | `{cmd}` |")
        lines.append("")

    # Section 3: Test execution results
    lines.append("## 测试执行结果")
    lines.append("")

    total_passed = 0
    total_failed = 0
    total_skipped = 0
    total_errors = 0
    total_count = 0

    for r in test_results:
        command_key = r.get("command_key", "unknown")
        command = r.get("command", "")
        status = r.get("status", "unknown")
        exit_code = r.get("exit_code", -1)
        counts = r.get("counts", {})
        duration = r.get("duration_seconds")

        status_label = "成功" if status == "success" else "失败"
        lines.append(f"### {command_key}")
        lines.append("")
        lines.append("| 项目 | 内容 |")
        lines.append("|------|------|")
        lines.append(f"| **命令** | `{command}` |")
        lines.append(f"| **状态** | {status_label} |")
        lines.append(f"| **退出码** | {exit_code} |")
        if duration is not None:
            lines.append(f"| **耗时** | {duration:.1f}s |")
        lines.append("")

        passed = counts.get("passed", 0)
        failed = counts.get("failed", 0)
        skipped = counts.get("skipped", 0)
        errors = counts.get("errors", 0)
        count = counts.get("total", 0)

        total_passed += passed
        total_failed += failed
        total_skipped += skipped
        total_errors += errors
        total_count += count

        if count > 0:
            lines.append("| 指标 | 数量 |")
            lines.append("|------|------|")
            lines.append(f"| 通过 | {passed} |")
            lines.append(f"| 失败 | {failed} |")
            lines.append(f"| 跳过 | {skipped} |")
            lines.append(f"| 错误 | {errors} |")
            lines.append(f"| **总计** | **{count}** |")
            lines.append("")

    # Overall summary
    if len(test_results) > 1:
        lines.append("### 汇总")
        lines.append("")
        lines.append("| 指标 | 数量 |")
        lines.append("|------|------|")
        lines.append(f"| 通过 | {total_passed} |")
        lines.append(f"| 失败 | {total_failed} |")
        lines.append(f"| 跳过 | {total_skipped} |")
        lines.append(f"| 错误 | {total_errors} |")
        lines.append(f"| **总计** | **{total_count}** |")
        lines.append("")

    # Section 4: Failed test details
    all_failed: list[dict] = []
    for r in test_results:
        for ft in r.get("failed_tests", []):
            ft_with_key = {**ft, "_command_key": r.get("command_key", "")}
            all_failed.append(ft_with_key)

    if all_failed:
        lines.append("## 失败用例详情")
        lines.append("")
        lines.append("| 测试类型 | 测试用例 | 错误信息 |")
        lines.append("|----------|----------|----------|")
        for ft in all_failed:
            name = ft.get("name", "unknown")
            error = ft.get("error", "").replace("\n", " ")[:200]
            key = ft.get("_command_key", "")
            lines.append(f"| {key} | `{name}` | {error} |")
        lines.append("")

    # Section 5: Test output details
    has_output = any(r.get("stdout") or r.get("stderr") for r in test_results)
    if has_output:
        lines.append("## 测试输出详情")
        lines.append("")

        for r in test_results:
            command_key = r.get("command_key", "unknown")
            stdout = r.get("stdout", "")
            stderr = r.get("stderr", "")
            output = stdout
            if stderr:
                output = f"{stdout}\n\n--- stderr ---\n{stderr}" if stdout else stderr

            if output.strip():
                lines.append("<details>")
                lines.append(f"<summary>{command_key} 测试输出</summary>")
                lines.append("")
                lines.append("```")
                if len(output) > 5000:
                    lines.append(output[:5000])
                    lines.append(f"\n... (输出已截断，共 {len(output)} 字符)")
                else:
                    lines.append(output)
                lines.append("```")
                lines.append("")
                lines.append("</details>")
                lines.append("")

    # Section 6: Conclusion
    lines.append("## 测试结论")
    lines.append("")
    verdict_label = _VERDICT_LABELS.get(verdict, verdict)
    lines.append(f"**判定结果：{verdict_label}**")
    lines.append("")

    lines.append("### 建议")
    lines.append("")
    if verdict == "通过":
        lines.append("- 所有测试通过，服务运行正常。")
    elif verdict == "部分通过":
        lines.append("- 部分测试失败，建议检查上方失败用例详情。")
        lines.append("- 确认失败用例是否与近期变更相关。")
    else:
        lines.append("- 测试未通过，建议优先排查失败原因。")
        lines.append("- 检查服务是否正常部署并可访问。")
    lines.append("")

    lines.append("---")
    lines.append(f"*报告由 gitlab API 测试模式自动生成 | {timestamp}*")

    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate Chinese API test report")
    parser.add_argument("--project-info", required=True, help="JSON string with project metadata")
    parser.add_argument("--test-config", required=True, help="JSON string with AgenticHub-test.yaml content")
    parser.add_argument("--test-results", required=True, help="JSON array of per-command test results")
    args = parser.parse_args()

    try:
        project_info = json.loads(args.project_info)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"Invalid project-info JSON: {e}"}), file=sys.stderr)
        sys.exit(1)

    try:
        test_config = json.loads(args.test_config)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"Invalid test-config JSON: {e}"}), file=sys.stderr)
        sys.exit(1)

    try:
        test_results = json.loads(args.test_results)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"Invalid test-results JSON: {e}"}), file=sys.stderr)
        sys.exit(1)

    report = generate_api_report(project_info, test_config, test_results)

    # Save report to file
    project_name = project_info.get("project_name", "unknown")
    reports_dir = WORKSPACE / "reports"
    reports_dir.mkdir(parents=True, exist_ok=True)
    report_path = reports_dir / f"{project_name}-api-test-report.md"
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
