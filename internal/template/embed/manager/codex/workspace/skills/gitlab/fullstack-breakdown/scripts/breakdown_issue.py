"""Batch-create sub-issues from a breakdown plan and update the parent issue.

Reads a JSON task list from stdin or --tasks, creates a GitLab issue for each
task, then appends a Markdown checklist to the parent issue description
(with clickable [title](web_url) links to each sub-issue).

Usage:
    python3 breakdown_issue.py \
        --project-id 123 \
        --parent-iid 42 \
        --tasks '[{"title":"Task 1","description":"...","labels":["backend"]}]'
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import httpx

SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

from create_issue import create_issue
from shared_gitlab_utils import load_credentials
from update_issue import update_issue


_CHECKLIST_HEADER_RE = r"### 任务拆解（由 gitlab 自动生成，源自 #\d+）"


def _escape_md_link_text(text: str) -> str:
    """Escape characters that break Markdown [text](url) link labels."""
    return text.replace("\\", "\\\\").replace("[", "\\[").replace("]", "\\]")


def _build_checklist(parent_iid: int, sub_issues: list[dict], parent_issue_web_url: str) -> str:
    """Build a Markdown checklist with hyperlinks to each sub-issue (web_url)."""
    lines = [
        "",
        "---",
        f"### 任务拆解（由 gitlab 自动生成，源自 #{parent_iid}）",
        "",
    ]
    for si in sub_issues:
        iid = si["iid"]
        title = si["title"]
        # Build stable issue URL from parent issue URL to avoid accidental MR links.
        # parent_issue_web_url: .../-/issues/{parent_iid}
        parent_base = parent_issue_web_url.rsplit("/-/issues/", 1)[0]
        url = f"{parent_base}/-/issues/{iid}"
        if url:
            safe = _escape_md_link_text(title)
            lines.append(f"- [ ] [{safe}]({url}) (#{iid})")
        else:
            lines.append(f"- [ ] #{iid} — {title}")
    return "\n".join(lines)


def _get_issue(project_id: int | str, issue_iid: int) -> dict:
    """Fetch issue payload from GitLab."""
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    url = f"{gitlab_url}/api/v4/projects/{project_id}/issues/{issue_iid}"
    headers = {"PRIVATE-TOKEN": access_token}

    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=headers)
        resp.raise_for_status()
        return resp.json()


def _upsert_generated_checklist(description: str, checklist: str) -> str:
    """Replace previous auto-generated checklist block, then append the new one."""
    import re

    # Remove an existing generated section (if any) to avoid duplicates.
    pattern = re.compile(
        rf"\n?---\n{_CHECKLIST_HEADER_RE}\n(?:.*\n)*?(?=(?:\n---\n{_CHECKLIST_HEADER_RE}\n)|\Z)",
        flags=re.MULTILINE,
    )
    cleaned = re.sub(pattern, "", description).rstrip()
    return f"{cleaned}{checklist}"


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Create sub-issues from breakdown plan")
    parser.add_argument("--project-id", required=True, help="Project ID")
    parser.add_argument("--parent-iid", required=True, type=int, help="Parent issue IID (the story)")
    parser.add_argument(
        "--tasks",
        required=True,
        help='JSON array of tasks: [{"title":"...","description":"...","labels":["..."]}]',
    )

    args = parser.parse_args()

    try:
        tasks = json.loads(args.tasks)
    except json.JSONDecodeError as e:
        print(json.dumps({"error": f"Invalid tasks JSON: {e}"}), file=sys.stderr)
        sys.exit(1)

    if not tasks:
        print(json.dumps({"error": "Tasks array is empty"}), file=sys.stderr)
        sys.exit(1)

    created: list[dict] = []
    errors: list[dict] = []
    parent_issue: dict | None = None
    parent_milestone_id: int | None = None

    try:
        parent_issue = _get_issue(args.project_id, args.parent_iid)
        parent_milestone = parent_issue.get("milestone") or {}
        parent_milestone_id = parent_milestone.get("id")
    except Exception as e:
        errors.append({"title": "get_parent_issue", "error": str(e)})

    for i, task in enumerate(tasks):
        title = task.get("title", f"Task {i + 1}")
        description = task.get("description", "")
        labels = task.get("labels", [])

        # Prepend parent reference to description
        full_description = f"Parent story: #{args.parent_iid}\n\n{description}".strip()

        try:
            issue = create_issue(
                project_id=args.project_id,
                title=title,
                description=full_description,
                labels=labels,
                milestone_id=parent_milestone_id,
            )
            created.append(
                {
                    "iid": issue["iid"],
                    "title": issue["title"],
                    "web_url": issue["web_url"],
                }
            )
        except Exception as e:
            errors.append({"title": title, "error": str(e)})

    # Update parent issue description with checklist
    if created:
        try:
            if parent_issue is None:
                parent_issue = _get_issue(args.project_id, args.parent_iid)
            current_desc = parent_issue.get("description", "") or ""
            parent_web_url = (parent_issue.get("web_url", "") or "").strip()
            checklist = _build_checklist(args.parent_iid, created, parent_web_url)
            new_desc = _upsert_generated_checklist(current_desc, checklist)

            update_issue(
                project_id=args.project_id,
                issue_iid=args.parent_iid,
                description=new_desc,
            )
        except Exception as e:
            errors.append({"title": "update_parent", "error": str(e)})

    result = {
        "parent_iid": args.parent_iid,
        "parent_milestone_id": parent_milestone_id,
        "created_count": len(created),
        "sub_issues": created,
    }
    if errors:
        result["errors"] = errors

    print(json.dumps(result, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
