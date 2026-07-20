"""Check milestone health for a version (issue completion, not GitLab Release objects).

Examples:
  python3 check_milestone_health.py --milestone 0.6.1
  python3 check_milestone_health.py --milestone v0.6.1 --my-issues --format markdown
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

from milestone_utils import (  # noqa: E402
    fetch_group_milestones,
    group_path_from_project,
    milestone_matches,
    normalize_milestone_version,
    resolve_milestone_title,
)
from shared_gitlab_utils import load_credentials  # noqa: E402

DEFAULT_PROJECT_PATH = "product/agentichub/requirements"


def _fetch_project_issues(
    *,
    gitlab_url: str,
    token: str,
    project_ref: str,
    per_page: int,
    max_pages: int,
) -> list[dict]:
    url = f"{gitlab_url.rstrip('/')}/api/v4/projects/{project_ref}/issues"
    headers = {"PRIVATE-TOKEN": token, "Content-Type": "application/json"}
    params_base = {"state": "all", "order_by": "updated_at", "sort": "desc"}
    all_items: list[dict] = []
    with httpx.Client(timeout=60.0) as client:
        for page in range(1, max_pages + 1):
            params = {**params_base, "page": page, "per_page": per_page}
            response = client.get(url, headers=headers, params=params)
            response.raise_for_status()
            batch = response.json()
            if not batch:
                break
            all_items.extend(batch)
            if len(batch) < per_page:
                break
    return all_items


def _slim_issue(issue: dict, *, project_path: str) -> dict:
    milestone = issue.get("milestone") or {}
    assignee = issue.get("assignee")
    return {
        "repo": project_path,
        "iid": issue.get("iid"),
        "title": issue.get("title"),
        "state": issue.get("state"),
        "due_date": issue.get("due_date"),
        "milestone": milestone.get("title"),
        "labels": issue.get("labels", []),
        "assignee": assignee.get("username") if assignee else None,
        "web_url": issue.get("web_url"),
    }


def _health_verdict(*, opened_count: int, missing_due_date_count: int, total_count: int) -> str:
    if total_count == 0:
        return "no_issues"
    if opened_count == 0:
        if missing_due_date_count > 0:
            return "closed_with_data_gaps"
        return "closed_clean"
    if missing_due_date_count > 0:
        return "in_progress_with_data_gaps"
    return "in_progress"


def _to_markdown(result: dict) -> str:
    milestone = result.get("milestone") or {}
    counts = result.get("counts") or {}
    lines = [
        f"milestone_query={result.get('milestone_query')}",
        f"milestone_title={milestone.get('title')}",
        f"milestone_due_date={milestone.get('due_date')}",
        f"health_verdict={result.get('health_verdict')}",
        f"total={counts.get('total')}",
        f"opened={counts.get('opened')}",
        f"closed={counts.get('closed')}",
        f"missing_due_date={counts.get('missing_due_date')}",
        "",
        "| repo | iid | title | state | due_date | milestone | labels | assignee | web_url |",
        "|---|---:|---|---|---|---|---|---|---|",
    ]
    for row in result.get("opened_issues", []):
        labels = ",".join(row.get("labels") or [])
        lines.append(
            f"| {row.get('repo')} | {row.get('iid')} | {row.get('title') or ''} | "
            f"{row.get('state') or ''} | {row.get('due_date') or ''} | "
            f"{row.get('milestone') or ''} | {labels} | {row.get('assignee') or ''} | "
            f"{row.get('web_url') or ''} |",
        )
    if not result.get("opened_issues"):
        lines.append("| (no opened issues in milestone) | | | | | | | | |")
    return "\n".join(lines)


def _fetch_current_user_id(*, gitlab_url: str, token: str) -> int | None:
    url = f"{gitlab_url.rstrip('/')}/api/v4/user"
    headers = {"PRIVATE-TOKEN": token, "Content-Type": "application/json"}
    with httpx.Client(timeout=30.0) as client:
        response = client.get(url, headers=headers)
        response.raise_for_status()
        payload = response.json()
        user_id = payload.get("id")
        return int(user_id) if user_id is not None else None


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Check milestone health for a version (issue status, not GitLab Release)",
    )
    parser.add_argument(
        "--project-path",
        default=DEFAULT_PROJECT_PATH,
        help=f"Project path, default: {DEFAULT_PROJECT_PATH}",
    )
    parser.add_argument(
        "--milestone",
        required=True,
        help="Milestone version (supports 0.6.1 / v0.6.1)",
    )
    parser.add_argument(
        "--my-issues",
        action="store_true",
        help="Only include opened issues assigned to the current token user",
    )
    parser.add_argument("--per-page", type=int, default=100)
    parser.add_argument("--max-pages", type=int, default=50)
    parser.add_argument(
        "--format",
        default="json",
        choices=("json", "markdown"),
        help="Output format",
    )
    args = parser.parse_args()

    try:
        credentials = load_credentials()
        gitlab_url = credentials["gitlab_url"]
        token = credentials["access_token"]
        group_path = group_path_from_project(args.project_path)
        group_milestones = fetch_group_milestones(
            gitlab_url=gitlab_url,
            access_token=token,
            group_path=group_path,
        )
        resolved_title = resolve_milestone_title(group_milestones, args.milestone)
        milestone_meta = None
        if resolved_title:
            for milestone in group_milestones:
                if milestone.get("title") == resolved_title:
                    milestone_meta = milestone
                    break

        project_ref = quote(args.project_path.strip(), safe="")
        raw_issues = _fetch_project_issues(
            gitlab_url=gitlab_url,
            token=token,
            project_ref=project_ref,
            per_page=args.per_page,
            max_pages=args.max_pages,
        )
        milestone_issues = [issue for issue in raw_issues if milestone_matches(issue, args.milestone)]
        opened_issues = [issue for issue in milestone_issues if issue.get("state") == "opened"]
        if args.my_issues:
            current_user_id = _fetch_current_user_id(gitlab_url=gitlab_url, token=token)
            if current_user_id is not None:
                opened_issues = [
                    issue
                    for issue in opened_issues
                    if (issue.get("assignee") or {}).get("id") == current_user_id
                ]

        missing_due_date_count = sum(1 for issue in milestone_issues if not issue.get("due_date"))
        counts = {
            "total": len(milestone_issues),
            "opened": len(opened_issues),
            "closed": sum(1 for issue in milestone_issues if issue.get("state") == "closed"),
            "missing_due_date": missing_due_date_count,
        }
        result = {
            "project_path": args.project_path,
            "group_path": group_path,
            "milestone_query": args.milestone,
            "milestone": milestone_meta,
            "resolved_milestone_title": resolved_title,
            "matched_milestone_versions": sorted(
                {
                    normalize_milestone_version((issue.get("milestone") or {}).get("title", ""))
                    for issue in milestone_issues
                    if (issue.get("milestone") or {}).get("title")
                },
            ),
            "counts": counts,
            "health_verdict": _health_verdict(
                opened_count=counts["opened"],
                missing_due_date_count=missing_due_date_count,
                total_count=counts["total"],
            ),
            "opened_issues": [_slim_issue(issue, project_path=args.project_path) for issue in opened_issues],
            "note": (
                "Milestone health is based on group milestone issues in the project. "
                "This is NOT GitLab Release API (/releases)."
            ),
        }

        if args.format == "json":
            print(json.dumps(result, indent=2, ensure_ascii=False))
        else:
            print(_to_markdown(result))
    except (FileNotFoundError, ValueError) as exc:
        print(f"Error: {exc}", file=sys.stderr)
        sys.exit(1)
    except httpx.HTTPStatusError as exc:
        print(f"Error: GitLab API {exc.response.status_code}", file=sys.stderr)
        try:
            print(exc.response.json(), file=sys.stderr)
        except Exception:
            print(exc.response.text[:500], file=sys.stderr)
        sys.exit(2)
    except Exception as exc:
        print(f"Unexpected error: {exc}", file=sys.stderr)
        sys.exit(3)


if __name__ == "__main__":
    main()
