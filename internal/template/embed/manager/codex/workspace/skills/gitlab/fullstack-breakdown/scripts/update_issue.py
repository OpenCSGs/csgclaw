"""GitLab issue update script.

Updates an existing issue's description, title, labels, or state.
Usage: python3 update_issue.py --project-id 123 --issue-iid 42 --description "New description"
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

from shared_gitlab_utils import load_credentials


def update_issue(
    project_id: int | str,
    issue_iid: int,
    *,
    title: str | None = None,
    description: str | None = None,
    labels: list[str] | None = None,
    state_event: str | None = None,
) -> dict:
    """Update an existing issue in GitLab.

    Args:
        project_id: Project ID or URL-encoded path.
        issue_iid: Issue IID to update.
        title: New title (optional).
        description: New description (optional).
        labels: New label list (optional, replaces existing).
        state_event: "close" or "reopen" (optional).

    Returns:
        Updated issue dict.

    Raises:
        httpx.HTTPStatusError: If API request fails.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    url = f"{gitlab_url}/api/v4/projects/{project_id}/issues/{issue_iid}"

    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }

    data: dict = {}
    if title is not None:
        data["title"] = title
    if description is not None:
        data["description"] = description
    if labels is not None:
        data["labels"] = ",".join(labels)
    if state_event:
        data["state_event"] = state_event

    with httpx.Client(timeout=30.0) as client:
        response = client.put(url, headers=headers, json=data)
        response.raise_for_status()
        return response.json()


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Update a GitLab issue")
    parser.add_argument("--project-id", required=True, help="Project ID or URL-encoded path")
    parser.add_argument("--issue-iid", required=True, type=int, help="Issue IID to update")
    parser.add_argument("--title", default=None, help="New issue title")
    parser.add_argument("--description", default=None, help="New issue description")
    parser.add_argument("--labels", default=None, help="Comma-separated list of labels (replaces existing)")
    parser.add_argument("--state-event", choices=["close", "reopen"], default=None, help="Change issue state")

    args = parser.parse_args()

    labels = [lbl.strip() for lbl in args.labels.split(",") if lbl.strip()] if args.labels else None

    try:
        issue = update_issue(
            project_id=args.project_id,
            issue_iid=args.issue_iid,
            title=args.title,
            description=args.description,
            labels=labels,
            state_event=args.state_event,
        )

        output = {
            "iid": issue["iid"],
            "title": issue["title"],
            "web_url": issue["web_url"],
            "state": issue["state"],
            "updated_at": issue.get("updated_at", ""),
        }
        print(json.dumps(output, indent=2))
        sys.exit(0)

    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            print("Error: Authentication failed. Check your access token.", file=sys.stderr)
        elif e.response.status_code == 404:
            print(f"Error: Issue #{args.issue_iid} not found in project {args.project_id}", file=sys.stderr)
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
