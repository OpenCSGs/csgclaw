"""Post test report as a note (comment) on a GitLab Merge Request.

Usage: python3 scripts/comment.py \
    --project-id 123 --mr-iid 42 --report-file "./reports/project-mr-42-report.md"
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

from shared_gitlab_utils import fail, load_credentials


def post_mr_note(project_id: int | str, mr_iid: int, body: str) -> dict:
    """Post a note on a GitLab MR.

    Args:
        project_id: GitLab project ID.
        mr_iid: Merge request IID.
        body: Note body (Markdown).

    Returns:
        Created note dict with id, body, etc.

    Raises:
        httpx.HTTPStatusError: If API request fails.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    url = f"{gitlab_url}/api/v4/projects/{project_id}/merge_requests/{mr_iid}/notes"
    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }
    data = {"body": body}

    with httpx.Client(timeout=30.0) as client:
        response = client.post(url, headers=headers, json=data)
        response.raise_for_status()
        return response.json()


def main() -> None:
    parser = argparse.ArgumentParser(description="Post test report to GitLab MR as a note")
    parser.add_argument("--project-id", required=True, help="GitLab project ID")
    parser.add_argument("--mr-iid", required=True, type=int, help="Merge request IID")
    parser.add_argument("--report-file", required=True, help="Path to the Markdown report file")
    args = parser.parse_args()

    report_path = Path(args.report_file)
    if not report_path.is_file():
        fail("report", f"Report file not found: {args.report_file}")

    body = report_path.read_text(encoding="utf-8")
    if not body.strip():
        fail("report", "Report file is empty")

    try:
        note = post_mr_note(args.project_id, args.mr_iid, body)
    except Exception as e:
        fail("comment", str(e))

    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]

    # Construct note URL from MR web_url
    note_url = ""
    try:
        mr_api = f"{gitlab_url}/api/v4/projects/{args.project_id}/merge_requests/{args.mr_iid}"
        headers = {"PRIVATE-TOKEN": credentials["access_token"]}
        with httpx.Client(timeout=15.0) as client:
            resp = client.get(mr_api, headers=headers)
            resp.raise_for_status()
            mr_web_url = resp.json().get("web_url", "")
            note_url = f"{mr_web_url}#note_{note.get('id', '')}"
    except Exception:  # noqa: S110
        pass  # Non-critical: note_url is informational

    result = {
        "note_id": note.get("id"),
        "note_url": note_url,
        "created_at": note.get("created_at", ""),
    }
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
