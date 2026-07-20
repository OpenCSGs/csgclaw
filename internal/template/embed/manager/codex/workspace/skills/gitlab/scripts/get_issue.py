"""GitLab issue retrieval script.

Retrieves issue details from a GitLab project, including description,
labels, assignees, and related merge requests.

Usage: python3 get_issue.py --project-path "group/project" --issue-iid 42
"""

from __future__ import annotations

import argparse
import json
import sys
from urllib.parse import quote

import httpx
from shared_gitlab_utils import load_credentials


def get_issue(project_path: str, issue_iid: int) -> dict:
    """Get issue details from GitLab API.

    Args:
        project_path: Project path in format "group/project".
        issue_iid: Issue IID (project-scoped ID).

    Returns:
        Issue info dict.

    Raises:
        httpx.HTTPStatusError: If API request fails.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    encoded_path = quote(project_path, safe="")
    url = f"{gitlab_url}/api/v4/projects/{encoded_path}/issues/{issue_iid}"

    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }

    with httpx.Client(timeout=30.0) as client:
        response = client.get(url, headers=headers)
        response.raise_for_status()
        return response.json()


def get_issue_notes(project_path: str, issue_iid: int) -> list[dict]:
    """Get comments/notes on an issue.

    Args:
        project_path: Project path in format "group/project".
        issue_iid: Issue IID.

    Returns:
        List of note dicts with id, body, author, created_at.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    encoded_path = quote(project_path, safe="")
    url = f"{gitlab_url}/api/v4/projects/{encoded_path}/issues/{issue_iid}/notes"

    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }

    with httpx.Client(timeout=30.0) as client:
        response = client.get(url, headers=headers, params={"sort": "asc", "per_page": 50})
        response.raise_for_status()
        return response.json()


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Get GitLab issue details")
    parser.add_argument(
        "--project-path",
        required=True,
        help="Project path (e.g. 'group/project')",
    )
    parser.add_argument(
        "--issue-iid",
        required=True,
        type=int,
        help="Issue IID",
    )
    parser.add_argument(
        "--with-notes",
        action="store_true",
        help="Include issue comments/notes",
    )

    args = parser.parse_args()

    try:
        issue = get_issue(args.project_path, args.issue_iid)

        output = {
            "iid": issue["iid"],
            "id": issue["id"],
            "title": issue["title"],
            "description": issue.get("description", ""),
            "state": issue["state"],
            "labels": issue.get("labels", []),
            "assignee": issue.get("assignee", {}).get("username") if issue.get("assignee") else None,
            "author": issue.get("author", {}).get("username"),
            "web_url": issue["web_url"],
            "created_at": issue.get("created_at"),
            "updated_at": issue.get("updated_at"),
        }

        if args.with_notes:
            notes = get_issue_notes(args.project_path, args.issue_iid)
            output["notes"] = [
                {
                    "id": n["id"],
                    "body": n["body"],
                    "author": n.get("author", {}).get("username"),
                    "created_at": n.get("created_at"),
                }
                for n in notes
                if not n.get("system", False)
            ]

        print(json.dumps(output, indent=2, ensure_ascii=False))
        sys.exit(0)

    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            print("Error: Authentication failed. Check your access token.", file=sys.stderr)
        elif e.response.status_code == 404:
            print(
                f"Error: Issue #{args.issue_iid} not found in {args.project_path}",
                file=sys.stderr,
            )
        else:
            print(f"Error: GitLab API error: {e.response.status_code}", file=sys.stderr)
            try:
                error_detail = e.response.json()
                print(f"Detail: {error_detail}", file=sys.stderr)
            except Exception:  # noqa: S110
                pass
        sys.exit(2)

    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)


if __name__ == "__main__":
    main()
