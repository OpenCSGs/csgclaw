"""Track sub-issue completion: check off parent checklist and close the sub-issue.

After a sub-issue's MR is created, this script:
1. Updates the parent issue's checklist (unchecked → checked; supports #iid lines and [title](url) (#iid) lines)
2. Closes the sub-issue

Usage:
    python3 scripts/track_progress.py \
        --project-id 123 \
        --parent-iid 42 \
        --sub-issue-iid 101
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

import httpx

SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

from shared_gitlab_utils import load_credentials
from update_issue import update_issue

CHECKLIST_HEADER = "### 任务拆解（由 gitlab 自动生成，源自 #{parent_iid}）"


def _escape_md_link_text(text: str) -> str:
    """Escape characters that break Markdown link labels."""
    return text.replace("\\", "\\\\").replace("[", "\\[").replace("]", "\\]")


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


def _get_issue_description(project_id: int | str, issue_iid: int) -> str:
    """Fetch current issue description from GitLab."""
    return _get_issue(project_id, issue_iid).get("description", "") or ""


def _line_matches_unchecked_sub_issue(line: str, iid: int) -> bool:
    """True if this line is an unchecked checklist item for sub-issue iid."""
    stripped = line.strip()
    if not stripped.startswith("- [ ] "):
        return False
    # Hyperlink format from breakdown_issue.py: - [ ] [title](url) (#iid)
    if stripped.endswith(f"(#{iid})"):
        return True
    # Plain format: - [ ] #iid — title
    return bool(re.match(rf"^- \[ \] #{iid}\b", stripped))


def _line_matches_checked_sub_issue(line: str, iid: int) -> bool:
    """True if this line is a checked checklist item for sub-issue iid."""
    stripped = line.strip()
    if not stripped.startswith("- [x] ") and not stripped.startswith("- [X] "):
        return False
    if stripped.endswith(f"(#{iid})"):
        return True
    return bool(re.match(rf"^- \[[xX]\] #{iid}\b", stripped))


def _build_checked_checklist_line(sub_issue: dict) -> str:
    """Build a checked checklist line for one sub-issue."""
    iid = sub_issue.get("iid")
    title = (sub_issue.get("title") or "").strip()
    web_url = (sub_issue.get("web_url") or "").strip()
    if web_url:
        safe_title = _escape_md_link_text(title)
        return f"- [x] [{safe_title}]({web_url}) (#{iid})"
    return f"- [x] #{iid} — {title}"


def _append_checked_entry_to_parent(
    description: str,
    parent_iid: int,
    checklist_line: str,
) -> str:
    """Append a checked checklist entry to parent issue description."""
    header = CHECKLIST_HEADER.format(parent_iid=parent_iid)
    cleaned = description.rstrip()

    if header in cleaned:
        return f"{cleaned}\n{checklist_line}\n"

    if not cleaned:
        return f"{header}\n\n{checklist_line}\n"

    return f"{cleaned}\n\n---\n{header}\n\n{checklist_line}\n"


def check_off_sub_issue(
    project_id: int | str,
    parent_iid: int,
    sub_issue_iid: int,
) -> bool:
    """Update parent issue checklist: mark sub-issue as completed.

    Returns:
        True if the checklist was updated, False if no matching entry found.
    """
    description = _get_issue_description(project_id, parent_iid)
    lines = description.splitlines(keepends=True)
    new_lines: list[str] = []
    updated = False
    already_checked = False

    for line in lines:
        body = line.rstrip("\r\n")
        ending = line[len(body) :]
        if _line_matches_unchecked_sub_issue(body, sub_issue_iid):
            new_body = body.replace("- [ ] ", "- [x] ", 1)
            new_lines.append(new_body + ending)
            updated = True
        elif _line_matches_checked_sub_issue(body, sub_issue_iid):
            new_lines.append(line)
            already_checked = True
        else:
            new_lines.append(line)

    if updated:
        update_issue(
            project_id=project_id,
            issue_iid=parent_iid,
            description="".join(new_lines),
        )
        return True

    if already_checked:
        return True

    # Parent has no matching checklist entry: append a checked entry automatically.
    sub_issue = _get_issue(project_id, sub_issue_iid)
    checklist_line = _build_checked_checklist_line(sub_issue)
    new_description = _append_checked_entry_to_parent(description, parent_iid, checklist_line)
    update_issue(
        project_id=project_id,
        issue_iid=parent_iid,
        description=new_description,
    )
    return True


def close_sub_issue(project_id: int | str, sub_issue_iid: int) -> dict:
    """Close the sub-issue."""
    return update_issue(
        project_id=project_id,
        issue_iid=sub_issue_iid,
        state_event="close",
    )


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Track sub-issue completion")
    parser.add_argument("--project-id", required=True, help="Project ID")
    parser.add_argument("--parent-iid", required=True, type=int, help="Parent story issue IID")
    parser.add_argument("--sub-issue-iid", required=True, type=int, help="Completed sub-issue IID")

    args = parser.parse_args()

    result: dict = {
        "parent_iid": args.parent_iid,
        "sub_issue_iid": args.sub_issue_iid,
    }
    errors: list[str] = []

    # 1. Check off in parent checklist
    try:
        checked = check_off_sub_issue(args.project_id, args.parent_iid, args.sub_issue_iid)
        result["checklist_updated"] = checked
        if not checked:
            errors.append(f"No checklist entry found for #{args.sub_issue_iid} in parent #{args.parent_iid}")
    except Exception as e:
        result["checklist_updated"] = False
        errors.append(f"Failed to update checklist: {e}")

    # 2. Close the sub-issue
    try:
        closed = close_sub_issue(args.project_id, args.sub_issue_iid)
        result["sub_issue_closed"] = closed.get("state") == "closed"
    except Exception as e:
        result["sub_issue_closed"] = False
        errors.append(f"Failed to close sub-issue: {e}")

    if errors:
        result["errors"] = errors

    print(json.dumps(result, indent=2, ensure_ascii=False))
    sys.exit(0 if not errors else 1)


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
