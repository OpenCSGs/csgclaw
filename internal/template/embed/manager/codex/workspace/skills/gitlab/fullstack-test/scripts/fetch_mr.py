"""Fetch MR details, associated issue, and file changes from GitLab API.

Usage: python3 scripts/fetch_mr.py --project-path "group/project" --mr-iid 42
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from urllib.parse import quote

import httpx

SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

from shared_gitlab_utils import fail, load_credentials


def _build_headers(access_token: str) -> dict:
    return {"PRIVATE-TOKEN": access_token, "Content-Type": "application/json"}


def _get_mr(gitlab_url: str, headers: dict, project_id: str, mr_iid: int) -> dict:
    """Fetch MR details including changes."""
    url = f"{gitlab_url}/api/v4/projects/{project_id}/merge_requests/{mr_iid}"
    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=headers)
        resp.raise_for_status()
        return resp.json()


def _get_mr_changes(gitlab_url: str, headers: dict, project_id: str, mr_iid: int) -> list[dict]:
    """Fetch file changes for the MR."""
    url = f"{gitlab_url}/api/v4/projects/{project_id}/merge_requests/{mr_iid}/changes"
    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=headers)
        resp.raise_for_status()
        data = resp.json()
        return data.get("changes", [])


def _get_issue(gitlab_url: str, headers: dict, project_id: str, issue_iid: int) -> dict | None:
    """Fetch issue details. Returns None if not found."""
    url = f"{gitlab_url}/api/v4/projects/{project_id}/issues/{issue_iid}"
    try:
        with httpx.Client(timeout=30.0) as client:
            resp = client.get(url, headers=headers)
            resp.raise_for_status()
            return resp.json()
    except httpx.HTTPStatusError:
        return None


def _extract_issue_iid(mr_description: str) -> int | None:
    """Extract issue IID from MR description (e.g. 'Closes #15', 'Fixes #42')."""
    if not mr_description:
        return None
    match = re.search(r"(?:closes|fixes|resolves)\s+#(\d+)", mr_description, re.IGNORECASE)
    if match:
        return int(match.group(1))
    return None


def _get_project(gitlab_url: str, headers: dict, project_path: str) -> dict:
    """Fetch project info by path."""
    encoded = quote(project_path, safe="")
    url = f"{gitlab_url}/api/v4/projects/{encoded}"
    with httpx.Client(timeout=30.0) as client:
        resp = client.get(url, headers=headers)
        resp.raise_for_status()
        return resp.json()


def main() -> None:
    parser = argparse.ArgumentParser(description="Fetch MR details from GitLab")
    parser.add_argument("--project-path", required=True, help="GitLab project path (group/project)")
    parser.add_argument("--mr-iid", required=True, type=int, help="Merge request IID")
    args = parser.parse_args()

    try:
        creds = load_credentials()
    except Exception as e:
        fail("credentials", str(e))

    gitlab_url = creds["gitlab_url"]
    headers = _build_headers(creds["access_token"])

    # Fetch project info
    try:
        project = _get_project(gitlab_url, headers, args.project_path)
    except Exception as e:
        fail("project", str(e))

    project_id = project["id"]

    # Fetch MR details
    try:
        mr = _get_mr(gitlab_url, headers, str(project_id), args.mr_iid)
    except Exception as e:
        fail("mr", str(e))

    # Fetch MR file changes
    try:
        raw_changes = _get_mr_changes(gitlab_url, headers, str(project_id), args.mr_iid)
    except Exception as e:
        fail("changes", str(e))

    changes = [
        {
            "old_path": c.get("old_path", ""),
            "new_path": c.get("new_path", ""),
            "new_file": c.get("new_file", False),
            "deleted_file": c.get("deleted_file", False),
            "renamed_file": c.get("renamed_file", False),
        }
        for c in raw_changes
    ]

    # Try to extract and fetch associated issue
    issue_data = None
    issue_iid = _extract_issue_iid(mr.get("description", ""))
    if issue_iid:
        issue = _get_issue(gitlab_url, headers, str(project_id), issue_iid)
        if issue:
            issue_data = {
                "iid": issue["iid"],
                "title": issue["title"],
                "description": issue.get("description", ""),
                "state": issue.get("state", ""),
                "labels": issue.get("labels", []),
            }

    result = {
        "mr": {
            "iid": mr["iid"],
            "title": mr["title"],
            "description": mr.get("description", ""),
            "source_branch": mr["source_branch"],
            "target_branch": mr["target_branch"],
            "state": mr.get("state", ""),
            "web_url": mr.get("web_url", ""),
            "author": mr.get("author", {}).get("username", ""),
        },
        "issue": issue_data,
        "changes": changes,
        "project": {
            "id": project_id,
            "name": project["name"],
            "path_with_namespace": project["path_with_namespace"],
            "default_branch": project.get("default_branch", "main"),
            "web_url": project.get("web_url", ""),
        },
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
