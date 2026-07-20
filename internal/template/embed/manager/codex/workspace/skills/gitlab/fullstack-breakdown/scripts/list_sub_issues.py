"""List sub-issues parsed from parent issue description checklist.

This script reads a parent issue description and extracts sub-issue checklist
entries created by breakdown_issue.py, supporting both formats:
1) - [ ] [title](url) (#123)
2) - [ ] #123 — title
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


LINKED_PATTERN = re.compile(
    r"^- \[(?P<checked>[ xX])\] \[(?P<title>.+?)\]\((?P<url>https?://[^)]+)\) \(#(?P<iid>\d+)\)\s*$",
)
PLAIN_PATTERN = re.compile(
    r"^- \[(?P<checked>[ xX])\] #(?P<iid>\d+)\b(?:\s+[—-]{1,2}\s+(?P<title>.*))?\s*$",
)


def _get_issue_description(project_id: int | str, issue_iid: int) -> str:
    """Fetch current issue description from GitLab."""
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    url = f"{gitlab_url}/api/v4/projects/{project_id}/issues/{issue_iid}"
    headers = {"PRIVATE-TOKEN": access_token}

    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=headers)
        resp.raise_for_status()
        return resp.json().get("description", "") or ""


def _parse_line(line: str) -> dict | None:
    """Parse one checklist line into a sub-issue record."""
    linked_match = LINKED_PATTERN.match(line)
    if linked_match:
        return {
            "iid": int(linked_match.group("iid")),
            "title": linked_match.group("title"),
            "web_url": linked_match.group("url"),
            "checked": linked_match.group("checked").lower() == "x",
            "format": "linked",
        }

    plain_match = PLAIN_PATTERN.match(line)
    if plain_match:
        return {
            "iid": int(plain_match.group("iid")),
            "title": (plain_match.group("title") or "").strip(),
            "web_url": "",
            "checked": plain_match.group("checked").lower() == "x",
            "format": "plain",
        }

    return None


def main() -> None:
    """Main entry point."""
    parser = argparse.ArgumentParser(description="List sub-issues from parent checklist")
    parser.add_argument("--project-id", required=True, help="Project ID")
    parser.add_argument("--parent-iid", required=True, type=int, help="Parent issue IID")
    args = parser.parse_args()

    description = _get_issue_description(args.project_id, args.parent_iid)
    lines = description.splitlines()
    sub_issues: list[dict] = []

    for line in lines:
        parsed = _parse_line(line.strip())
        if parsed:
            sub_issues.append(parsed)

    result = {
        "parent_iid": args.parent_iid,
        "sub_issues_count": len(sub_issues),
        "sub_issues": sub_issues,
    }
    print(json.dumps(result, indent=2, ensure_ascii=False))
    sys.exit(0)


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
