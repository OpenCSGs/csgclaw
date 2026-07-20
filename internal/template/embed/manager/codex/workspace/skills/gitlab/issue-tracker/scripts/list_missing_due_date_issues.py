"""List issues in a milestone that do not have due dates.

Examples:
  python3 list_missing_due_date_issues.py --milestone 0.5.2
  python3 list_missing_due_date_issues.py --milestone v0.5.2 --state opened --format markdown
  python3 list_missing_due_date_issues.py --project-path group/repo --milestone 0.5.2 --all-states
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

from milestone_utils import milestone_matches, normalize_milestone_version
from shared_gitlab_utils import load_credentials

DEFAULT_PROJECT_PATH = "product/agentichub/requirements"


def _project_ref(project_path: str | None, project_id: str | None) -> str:
    if project_path:
        return quote(project_path.strip(), safe="")
    if project_id is not None:
        return str(project_id).strip()
    raise ValueError("Provide --project-path or --project-id")


def _fetch_project_issues(
    *,
    gitlab_url: str,
    token: str,
    project_ref: str,
    state: str,
    per_page: int,
    max_pages: int,
) -> list[dict]:
    url = f"{gitlab_url}/api/v4/projects/{project_ref}/issues"
    headers = {"PRIVATE-TOKEN": token, "Content-Type": "application/json"}
    params_base = {"state": state, "order_by": "updated_at", "sort": "desc"}

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


def _to_row(issue: dict) -> dict:
    milestone = issue.get("milestone") or {}
    return {
        "iid": issue.get("iid"),
        "title": issue.get("title"),
        "state": issue.get("state"),
        "due_date": issue.get("due_date"),
        "milestone": milestone.get("title"),
        "labels": issue.get("labels", []),
        "web_url": issue.get("web_url"),
    }


def _to_markdown_table(project_path: str, rows: list[dict]) -> str:
    lines = [
        "| repo | iid | title | state | due_date | milestone | labels | web_url |",
        "|---|---:|---|---|---|---|---|---|",
    ]
    for row in rows:
        labels = ",".join(row["labels"]) if row.get("labels") else ""
        line = (
            f"| {project_path} | {row.get('iid')} | {row.get('title') or ''} | "
            f"{row.get('state') or ''} | {row.get('due_date') or ''} | "
            f"{row.get('milestone') or ''} | {labels} | {row.get('web_url') or ''} |"
        )
        lines.append(line)
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser(
        description="List issues in a milestone without due dates",
    )
    parser.add_argument(
        "--project-path",
        default=DEFAULT_PROJECT_PATH,
        help=f"Project path, default: {DEFAULT_PROJECT_PATH}",
    )
    parser.add_argument("--project-id", default=None, help="Project ID or URL-encoded path")
    parser.add_argument(
        "--milestone",
        required=True,
        help="Milestone title or version (supports 0.5.2 / v0.5.2)",
    )
    parser.add_argument(
        "--state",
        default="opened",
        choices=("opened", "closed", "all"),
        help="Issue state when fetching from GitLab",
    )
    parser.add_argument(
        "--all-states",
        action="store_true",
        help="Shortcut for --state all",
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

    effective_state = "all" if args.all_states else args.state

    try:
        credentials = load_credentials()
        gitlab_url = credentials["gitlab_url"]
        token = credentials["access_token"]
        project_ref = _project_ref(args.project_path, args.project_id)

        raw_issues = _fetch_project_issues(
            gitlab_url=gitlab_url,
            token=token,
            project_ref=project_ref,
            state=effective_state,
            per_page=args.per_page,
            max_pages=args.max_pages,
        )

        milestone_issues = [issue for issue in raw_issues if milestone_matches(issue, args.milestone)]

        missing_due_date_rows = [
            _to_row(issue)
            for issue in milestone_issues
            if not issue.get("due_date")
        ]

        result = {
            "project_path": args.project_path,
            "milestone_query": args.milestone,
            "matched_milestone_versions": sorted(
                {
                    normalize_milestone_version((issue.get("milestone") or {}).get("title", ""))
                    for issue in milestone_issues
                    if (issue.get("milestone") or {}).get("title")
                },
            ),
            "state": effective_state,
            "total_issues_in_milestone": len(milestone_issues),
            "missing_due_date_count": len(missing_due_date_rows),
            "missing_due_date_issues": missing_due_date_rows,
        }

        if args.format == "json":
            print(json.dumps(result, indent=2, ensure_ascii=False))
        else:
            print(f"milestone={args.milestone}")
            print(f"state={effective_state}")
            print(f"total_issues_in_milestone={len(milestone_issues)}")
            print(f"missing_due_date_count={len(missing_due_date_rows)}")
            print()
            print(_to_markdown_table(args.project_path, missing_due_date_rows))
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
