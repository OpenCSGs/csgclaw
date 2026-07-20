#!/usr/bin/env python3
"""Create a pre-sale customer follow-up issue in GitLab (product/pre-sale/customers).

Issue title defaults to company name (客户名 / 公司名称); description holds the intake form.

脚本会将仓库根目录 `gitlab/scripts` 加入 `sys.path` 以加载 `shared_gitlab_utils`。
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from urllib.parse import quote

import httpx


_SCRIPT_PARENT = Path(__file__).resolve().parents[2] / "scripts"
if str(_SCRIPT_PARENT) not in sys.path:
    sys.path.insert(0, str(_SCRIPT_PARENT))

from shared_gitlab_utils import load_credentials  # noqa: E402


DEFAULT_PROJECT_PATH = "product/pre-sale/customers"

FIELD_KEYS: tuple[tuple[str, str], ...] = (
    ("company_name", "公司名称"),
    ("contact_name", "联系人姓名"),
    ("phone", "联系电话"),
    ("job_title", "职位"),
    ("is_decision_maker", "是否为决策者"),
    ("requirements", "需求描述"),
    ("products_interest", "关注的产品"),
    ("budget", "是否有预算，预算有多少"),
    ("project_duration", "项目周期"),
    ("potential_users", "潜在用户数量"),
    ("lead_source", "线索来源"),
    ("next_step", "下一步计划"),
    ("crm_link", "CRM线索链接"),
)


def _coerce_str(val: object) -> str:
    if val is None:
        return ""
    if isinstance(val, bool | int | float):
        return str(val)
    if isinstance(val, str):
        return val
    msg = "Field value must be string, number, bool, or null"
    raise TypeError(msg)


def merge_json_into_payload(data: dict, payload: dict[str, str]) -> None:
    """Fill payload from English keys and Chinese label keys."""
    for key, label_cn in FIELD_KEYS:
        if key in data:
            payload[key] = _coerce_str(data[key])
        elif label_cn in data:
            payload[key] = _coerce_str(data[label_cn])


def build_description(payload: dict[str, str]) -> str:
    """Render Markdown body from structured fields."""
    lines = [
        "## 售前初次回访 — 线索信息",
        "",
        "以下信息来自初次电话回访录入。",
        "",
    ]
    for key, label_cn in FIELD_KEYS:
        val = (payload.get(key) or "").strip()
        display = val if val else "—"
        lines.append(f"- **{label_cn}**：{display}")
    lines.append("")
    return "\n".join(lines)


def resolve_issue_title(payload: dict[str, str], *, prefer: str) -> str:
    """Pick issue title: company name first unless prefer=contact."""
    company = (payload.get("company_name") or "").strip()
    contact = (payload.get("contact_name") or "").strip()
    title = (contact or company) if prefer == "contact" else (company or contact)
    if not title:
        msg = "无法确定 issue 标题：请填写「公司名称」或「联系人姓名」，或使用 --issue-title"
        raise ValueError(msg)
    return title


def create_issue_api(project_path: str, title: str, description: str) -> dict:
    """POST /projects/:id/issues with URL-encoded project path."""
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"].rstrip("/")
    access_token = credentials["access_token"]
    encoded_path = quote(project_path, safe="")
    url = f"{gitlab_url}/api/v4/projects/{encoded_path}/issues"
    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }
    data = {"title": title, "description": description}
    with httpx.Client(timeout=60.0) as client:
        response = client.post(url, headers=headers, json=data)
        response.raise_for_status()
        return response.json()


def parse_payload(args: argparse.Namespace) -> dict[str, str]:
    """Build payload from CLI flags and/or JSON."""
    payload = dict.fromkeys([k for k, _ in FIELD_KEYS], "")

    if args.stdin_json:
        raw = sys.stdin.read()
        data = json.loads(raw)
        if not isinstance(data, dict):
            msg = "stdin JSON must be an object"
            raise ValueError(msg)
        merge_json_into_payload(data, payload)

    # CLI overrides stdin
    for key, _ in FIELD_KEYS:
        val = getattr(args, key, None)
        if val is not None and val != "":
            payload[key] = val

    return payload


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Create pre-sale customer follow-up issue (title = 公司名称 by default)",
    )
    parser.add_argument(
        "--project-path",
        default=DEFAULT_PROJECT_PATH,
        help=f"GitLab project path (default: {DEFAULT_PROJECT_PATH})",
    )
    parser.add_argument(
        "--title-from",
        choices=("company", "contact"),
        default="company",
        help="Issue title source: company name (客户公司名) or contact name (default: company)",
    )
    parser.add_argument(
        "--issue-title",
        default="",
        help="Override issue title explicitly (skips --title-from)",
    )
    parser.add_argument(
        "--stdin-json",
        action="store_true",
        help="Read a JSON object from stdin with camel/snake keys matching FIELD_KEYS",
    )
    for key, _ in FIELD_KEYS:
        parser.add_argument(f"--{key.replace('_', '-')}", default="", help=f"{key}")

    args = parser.parse_args()

    try:
        payload = parse_payload(args)
        description = build_description(payload)

        if args.issue_title.strip():
            title = args.issue_title.strip()
        else:
            title = resolve_issue_title(payload, prefer="contact" if args.title_from == "contact" else "company")

        issue = create_issue_api(args.project_path, title, description)
        out = {
            "iid": issue["iid"],
            "id": issue["id"],
            "title": issue["title"],
            "web_url": issue["web_url"],
            "state": issue["state"],
            "project_path": args.project_path,
        }
        print(json.dumps(out, indent=2, ensure_ascii=False))
    except (ValueError, TypeError, json.JSONDecodeError) as e:
        print(str(e), file=sys.stderr)
        sys.exit(1)
    except httpx.HTTPStatusError as e:
        print(f"GitLab API {e.response.status_code}: {e.response.text[:500]}", file=sys.stderr)
        sys.exit(2)
    except FileNotFoundError as e:
        print(str(e), file=sys.stderr)
        sys.exit(3)


if __name__ == "__main__":
    main()
