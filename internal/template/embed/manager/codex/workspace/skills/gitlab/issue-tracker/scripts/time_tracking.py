"""GitLab issue time tracking helper.

Supports:
- Set time estimate for an issue
- Add spent time for an issue (with optional summary)
- Reset time estimate
- Reset spent time
- Retrieve time tracking stats

All operations use GitLab REST API with credentials loaded via gitlab_utils.load_credentials().
Entry with name == "gitlab".

Examples:
  python3 time_tracking.py --project-path group/repo --issue-iid 1142 --set-estimate --duration 1h
  python3 time_tracking.py --project-path group/repo --issue-iid 1142 --add-spent --duration 45m --summary "focus time"
  python3 time_tracking.py --project-path group/repo --issue-iid 1142 --reset-estimate
  python3 time_tracking.py --project-path group/repo --issue-iid 1142 --reset-spent
  python3 time_tracking.py --project-path group/repo --issue-iid 1142 --stats
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

from shared_gitlab_utils import load_credentials


def _project_ref(project_path: str | None, project_id: str | None) -> str:
    if project_path:
        return quote(project_path.strip(), safe="")
    if project_id is not None:
        return str(project_id).strip()
    raise ValueError("Provide --project-path or --project-id")


def _post(
    *,
    client: httpx.Client,
    url: str,
    token: str,
    params: dict[str, str] | None = None,
    json_body: dict | None = None,
) -> dict:
    headers = {"PRIVATE-TOKEN": token}
    resp = client.post(url, headers=headers, params=params, json=json_body)
    resp.raise_for_status()
    return resp.json()


def _get(
    *,
    client: httpx.Client,
    url: str,
    token: str,
) -> dict:
    headers = {"PRIVATE-TOKEN": token}
    resp = client.get(url, headers=headers)
    resp.raise_for_status()
    return resp.json()


def main() -> None:
    parser = argparse.ArgumentParser(description="Modify GitLab issue time tracking")
    grp = parser.add_mutually_exclusive_group(required=True)
    grp.add_argument("--project-path", help="Project path, e.g. group/sub/repo")
    grp.add_argument("--project-id", help="Project ID or URL-encoded path")

    parser.add_argument("--issue-iid", required=True, type=int, help="Issue IID (project-scoped)")

    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument("--set-estimate", action="store_true", help="Set time estimate (duration required)")
    action.add_argument("--add-spent", action="store_true", help="Add spent time (duration required)")
    action.add_argument("--reset-estimate", action="store_true", help="Reset time estimate to 0")
    action.add_argument("--reset-spent", action="store_true", help="Reset spent time to 0")
    action.add_argument("--stats", action="store_true", help="Retrieve time tracking stats")

    parser.add_argument("--duration", help="Duration like 1h, 3h30m, 45m (required for set/add)")
    parser.add_argument("--summary", help="Optional summary for spent time (GitLab add_spent_time)")

    args = parser.parse_args()

    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    token = credentials["access_token"]

    project_ref = _project_ref(args.project_path, args.project_id)
    base = f"{gitlab_url}/api/v4/projects/{project_ref}/issues/{args.issue_iid}"

    with httpx.Client(timeout=30.0) as client:
        try:
            if args.stats:
                url = f"{base}/time_stats"
                result = _get(client=client, url=url, token=token)
            elif args.set_estimate:
                if not args.duration:
                    raise ValueError("--duration is required for --set-estimate")
                url = f"{base}/time_estimate"
                result = _post(
                    client=client,
                    url=url,
                    token=token,
                    params={"duration": args.duration},
                )
            elif args.add_spent:
                if not args.duration:
                    raise ValueError("--duration is required for --add-spent")
                url = f"{base}/add_spent_time"
                params: dict[str, str] = {"duration": args.duration}
                if args.summary:
                    params["summary"] = args.summary
                result = _post(client=client, url=url, token=token, params=params)
            elif args.reset_estimate:
                url = f"{base}/reset_time_estimate"
                result = _post(client=client, url=url, token=token)
            elif args.reset_spent:
                url = f"{base}/reset_spent_time"
                result = _post(client=client, url=url, token=token)
            else:
                raise ValueError("Unknown action")
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

    print(json.dumps(result, indent=2, ensure_ascii=False))
    sys.exit(0)


if __name__ == "__main__":
    main()
