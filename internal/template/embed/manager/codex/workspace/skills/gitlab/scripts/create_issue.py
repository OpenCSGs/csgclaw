#!/usr/bin/env python3
"""GitLab issue creation script.

Creates an issue with the given title and description only.
Does not fetch, list, or merge templates — use get_issue_template.py only (no --list-* template flags).

Usage:
  python3 get_issue_template.py product/agentichub/requirements
  # LLM renders template with user input, user confirms
  python3 create_issue.py --project-id product/agentichub/requirements \\
    --title "标题" --description "$(cat confirmed.md)"
"""

from __future__ import annotations

import argparse
import json
import sys
from urllib.parse import quote

import httpx
from shared_gitlab_utils import load_credentials


def _project_ref(project_id: int | str) -> str:
    """Return API-safe project id or URL-encoded path."""
    if isinstance(project_id, int):
        return str(project_id)
    project_path = str(project_id)
    if "/" in project_path and "%" not in project_path:
        return quote(project_path, safe="")
    return project_path


def create_issue(
    project_id: int | str,
    title: str,
    description: str | None = None,
    labels: list[str] | None = None,
    assignee_ids: list[int] | None = None,
    milestone_id: int | None = None,
) -> dict:
    """Create an issue in GitLab project.

    Args:
        project_id: Project ID or URL-encoded path.
        title: Issue title.
        description: Issue description (LLM-rendered template body after user confirms).
        labels: List of label names (optional).
        assignee_ids: List of user IDs to assign (optional).
        milestone_id: Milestone ID to assign (optional).

    Returns:
        Created issue dict with iid, web_url, title, etc.

    Raises:
        httpx.HTTPStatusError: If API request fails.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    ref = _project_ref(project_id)
    url = f"{gitlab_url}/api/v4/projects/{ref}/issues"

    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }

    data = {"title": title}
    if description and description.strip():
        data["description"] = description
    if labels:
        data["labels"] = ",".join(labels)
    if assignee_ids:
        data["assignee_ids"] = assignee_ids
    if milestone_id is not None:
        data["milestone_id"] = milestone_id

    with httpx.Client(timeout=30.0) as client:
        response = client.post(url, headers=headers, json=data)
        response.raise_for_status()
        return response.json()


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Create a GitLab issue")
    parser.add_argument(
        "--project-id",
        required=True,
        help="Project ID or URL-encoded path",
    )
    parser.add_argument(
        "--title",
        required=True,
        help="Issue title",
    )
    parser.add_argument(
        "--description",
        default="",
        help="Confirmed issue body (LLM-rendered from issues_template)",
    )
    parser.add_argument(
        "--labels",
        default="",
        help="Comma-separated list of labels",
    )
    parser.add_argument(
        "--milestone-id",
        type=int,
        default=None,
        help="Milestone ID to assign",
    )

    args = parser.parse_args()

    labels = [label.strip() for label in args.labels.split(",") if label.strip()] if args.labels else None

    try:
        issue = create_issue(
            project_id=args.project_id,
            title=args.title,
            description=args.description or None,
            labels=labels,
            milestone_id=args.milestone_id,
        )

        output = {
            "iid": issue["iid"],
            "id": issue["id"],
            "title": issue["title"],
            "description": issue.get("description", ""),
            "web_url": issue["web_url"],
            "state": issue["state"],
        }
        print(json.dumps(output, ensure_ascii=False, indent=2))
        sys.exit(0)

    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            print("Error: Authentication failed. Check your access token.", file=sys.stderr)
        elif e.response.status_code == 404:
            print(f"Error: Project not found: {args.project_id}", file=sys.stderr)
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
