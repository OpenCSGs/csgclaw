r"""List GitLab issues (project scope or cross-project 'my' issues) with pagination.

Usage:
  python3 list_issues.py --project-path group/repo --labels iteration-2025.03 --assignee-id 12
  python3 list_issues.py --my-issues --labels team-backend --state opened
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from urllib.parse import quote

import httpx

SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

from milestone_utils import (
    group_path_from_project,
    milestone_matches,
    resolve_group_milestone_title,
)
from shared_gitlab_utils import load_credentials

DEFAULT_GROUP_PATH = "product/agentichub"


def _project_ref(project_path: str | None, project_id: str | None) -> str:
    if project_path:
        return quote(project_path.strip(), safe="")
    if project_id is not None:
        return str(project_id).strip()
    msg = "Provide --project-path or --project-id, or use --my-issues"
    raise ValueError(msg)


def _slim(issue: dict) -> dict:
    assignee = issue.get("assignee")
    author = issue.get("author")
    return {
        "iid": issue.get("iid"),
        "title": issue.get("title"),
        "state": issue.get("state"),
        "web_url": issue.get("web_url"),
        "labels": issue.get("labels", []),
        "assignee": assignee.get("username") if assignee else None,
        "author": author.get("username") if author else None,
        "updated_at": issue.get("updated_at"),
        "project_id": issue.get("project_id"),
    }


def list_issues_paginated(
    *,
    gitlab_url: str,
    access_token: str,
    list_url: str,
    params_base: dict,
    per_page: int,
    max_pages: int,
) -> list[dict]:
    headers = {"PRIVATE-TOKEN": access_token, "Content-Type": "application/json"}
    all_items: list[dict] = []
    with httpx.Client(timeout=60.0) as client:
        for page in range(1, max_pages + 1):
            params = {**params_base, "page": page, "per_page": per_page}
            response = client.get(list_url, headers=headers, params=params)
            response.raise_for_status()
            batch = response.json()
            if not batch:
                break
            all_items.extend(batch)
            if len(batch) < per_page:
                break
    return all_items


def main() -> None:
    parser = argparse.ArgumentParser(description="List GitLab issues (paginated)")
    src = parser.add_mutually_exclusive_group(required=True)
    src.add_argument("--project-path", help="Project path e.g. group/repo")
    src.add_argument("--project-id", help="Numeric project id or URL-encoded path")
    src.add_argument(
        "--my-issues",
        action="store_true",
        help="Cross-project issues (uses /api/v4/issues); combine with --scope",
    )
    parser.add_argument(
        "--scope",
        default=None,
        choices=("assigned_to_me", "created_by_me", "all"),
        help="Only for --my-issues (default: assigned_to_me)",
    )
    parser.add_argument("--labels", default=None, help="Comma-separated labels (AND)")
    parser.add_argument(
        "--milestone",
        default=None,
        help="Milestone title or version (supports 0.6.1 / v0.6.1)",
    )
    parser.add_argument(
        "--group-path",
        default=None,
        help="Group path for milestone resolution (default: parent of --project-path)",
    )
    parser.add_argument("--assignee-id", type=int, default=None)
    parser.add_argument("--assignee-username", default=None)
    parser.add_argument("--author-username", default=None)
    parser.add_argument(
        "--state",
        default="opened",
        choices=("opened", "closed", "all"),
    )
    parser.add_argument("--search", default=None)
    parser.add_argument("--order-by", default="updated_at")
    parser.add_argument("--sort", default="desc", choices=("asc", "desc"))
    parser.add_argument("--per-page", type=int, default=100)
    parser.add_argument("--max-pages", type=int, default=50)
    parser.add_argument(
        "--full",
        action="store_true",
        help="Emit full API objects instead of slim rows",
    )
    args = parser.parse_args()

    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    token = credentials["access_token"]

    if args.my_issues:
        list_url = f"{gitlab_url}/api/v4/issues"
        scope = args.scope or "assigned_to_me"
        params_base: dict = {
            "state": args.state,
            "order_by": args.order_by,
            "sort": args.sort,
            "scope": scope,
        }
    else:
        ref = _project_ref(args.project_path, args.project_id)
        list_url = f"{gitlab_url}/api/v4/projects/{ref}/issues"
        params_base = {
            "state": args.state,
            "order_by": args.order_by,
            "sort": args.sort,
        }

    milestone_query = args.milestone
    resolved_milestone = None
    if milestone_query:
        group_path = args.group_path
        if not group_path:
            if args.project_path:
                group_path = group_path_from_project(args.project_path)
            else:
                group_path = DEFAULT_GROUP_PATH
        if group_path:
            resolved_milestone = resolve_group_milestone_title(
                gitlab_url=gitlab_url,
                access_token=token,
                group_path=group_path,
                user_input=milestone_query,
            )
        if resolved_milestone and not args.my_issues:
            params_base["milestone"] = resolved_milestone
        elif not args.my_issues:
            params_base["milestone"] = milestone_query

    if args.labels:
        params_base["labels"] = ",".join(s.strip() for s in args.labels.split(",") if s.strip())
    if args.assignee_id is not None:
        params_base["assignee_id"] = args.assignee_id
    if args.assignee_username:
        params_base["assignee_username"] = args.assignee_username
    if args.author_username:
        params_base["author_username"] = args.author_username
    if args.search:
        params_base["search"] = args.search

    try:
        raw = list_issues_paginated(
            gitlab_url=gitlab_url,
            access_token=token,
            list_url=list_url,
            params_base=params_base,
            per_page=args.per_page,
            max_pages=args.max_pages,
        )
        if milestone_query:
            raw = [issue for issue in raw if milestone_matches(issue, milestone_query)]
        if args.full:
            print(json.dumps(raw, indent=2, ensure_ascii=False))
        else:
            slim = [_slim(i) for i in raw]
            print(json.dumps(slim, indent=2, ensure_ascii=False))
    except (FileNotFoundError, ValueError) as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    except httpx.HTTPStatusError as e:
        print(f"Error: GitLab API {e.response.status_code}", file=sys.stderr)
        try:
            print(e.response.json(), file=sys.stderr)
        except Exception:
            print(e.response.text[:500], file=sys.stderr)
        sys.exit(2)
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)


if __name__ == "__main__":
    main()
