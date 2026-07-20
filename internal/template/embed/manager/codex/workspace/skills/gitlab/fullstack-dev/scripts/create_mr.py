#!/usr/bin/env python3
"""GitLab merge request creation script.

Creates a merge request in a GitLab project.
Usage: python3 create_mr.py --project-id 123 --source-branch "feature/42-desc" --target-branch "main" --title "MR title"
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


def create_merge_request(
    project_id: int | str,
    source_branch: str,
    target_branch: str,
    title: str,
    description: str | None = None,
    *,
    remove_source_branch: bool = True,
    squash: bool = False,
    labels: list[str] | None = None,
) -> dict:
    """Create a merge request in GitLab project.

    Args:
        project_id: Project ID or URL-encoded path.
        source_branch: Source branch name.
        target_branch: Target branch name.
        title: MR title.
        description: MR description (optional).
        remove_source_branch: Whether to remove source branch after merge.
        squash: Whether to squash commits on merge.
        labels: List of label names (optional).

    Returns:
        Created MR dict with iid, web_url, title, etc.

    Raises:
        httpx.HTTPStatusError: If API request fails.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    url = f"{gitlab_url}/api/v4/projects/{project_id}/merge_requests"

    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }

    data = {
        "source_branch": source_branch,
        "target_branch": target_branch,
        "title": title,
        "remove_source_branch": remove_source_branch,
        "squash": squash,
    }
    if description:
        data["description"] = description
    if labels:
        data["labels"] = ",".join(labels)

    with httpx.Client(timeout=30.0) as client:
        response = client.post(url, headers=headers, json=data)
        response.raise_for_status()
        return response.json()


def main():
    """Main entry point."""
    parser = argparse.ArgumentParser(description="Create a GitLab merge request")
    parser.add_argument(
        "--project-id",
        required=True,
        help="Project ID or URL-encoded path",
    )
    parser.add_argument(
        "--source-branch",
        required=True,
        help="Source branch name",
    )
    parser.add_argument(
        "--target-branch",
        required=True,
        help="Target branch name",
    )
    parser.add_argument(
        "--title",
        required=True,
        help="Merge request title",
    )
    parser.add_argument(
        "--description",
        default="",
        help="Merge request description",
    )
    parser.add_argument(
        "--labels",
        default="",
        help="Comma-separated list of labels",
    )
    parser.add_argument(
        "--no-remove-source-branch",
        action="store_true",
        help="Keep source branch after merge",
    )
    parser.add_argument(
        "--squash",
        action="store_true",
        help="Squash commits on merge",
    )

    args = parser.parse_args()

    labels = [label.strip() for label in args.labels.split(",") if label.strip()] if args.labels else None

    try:
        mr = create_merge_request(
            project_id=args.project_id,
            source_branch=args.source_branch,
            target_branch=args.target_branch,
            title=args.title,
            description=args.description or None,
            remove_source_branch=not args.no_remove_source_branch,
            squash=args.squash,
            labels=labels,
        )

        output = {
            "iid": mr["iid"],
            "id": mr["id"],
            "title": mr["title"],
            "web_url": mr["web_url"],
            "state": mr["state"],
            "source_branch": mr["source_branch"],
            "target_branch": mr["target_branch"],
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
            print(f"Error: Project not found: {args.project_id}", file=sys.stderr)
        elif e.response.status_code == 409:
            print("Error: Merge request already exists for this source/target branch.", file=sys.stderr)
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
