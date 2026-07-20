"""GitLab finalize: issue + commit + push + MR in one call.

Consolidates the entire post-approval phase into a single script call.
All operations are idempotent — safe to re-run after partial failures.
Usage: python3 scripts/finalize.py --project-id 123 --project-name "my-project" ...
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

from create_issue import create_issue
from create_mr import create_merge_request
from shared_gitlab_utils import (
    WORKSPACE,
    configure_git_credentials,
    fail,
    load_credentials,
    run_git,
    run_git_optional,
    run_git_with_credentials,
    set_clean_origin,
)


def get_active_branch(cwd: Path) -> str | None:
    """Read the active branch from local git config if present."""
    return run_git_optional(["config", "--get", "csgbot.activebranch"], cwd=cwd)


def find_existing_mr(
    project_id: int | str,
    source_branch: str,
    target_branch: str,
) -> dict | None:
    """Query GitLab for an existing open MR with the given source/target branches."""
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    url = (
        f"{gitlab_url}/api/v4/projects/{project_id}/merge_requests"
        f"?source_branch={quote(source_branch)}"
        f"&target_branch={quote(target_branch)}"
        f"&state=opened"
    )
    headers = {"PRIVATE-TOKEN": access_token}

    try:
        with httpx.Client(timeout=30.0) as client:
            response = client.get(url, headers=headers)
            response.raise_for_status()
            mrs = response.json()
            if mrs:
                return mrs[0]
    except Exception:  # noqa: S110
        pass
    return None


def prepare_git_auth(cwd: Path) -> None:
    """Scrub tokenized remotes and configure Git credentials from load_credentials()."""
    origin_url = run_git_optional(["remote", "get-url", "origin"], cwd=cwd)
    if origin_url:
        set_clean_origin(cwd, origin_url)
    else:
        configure_git_credentials(cwd)


def main() -> None:
    parser = argparse.ArgumentParser(description="Finalize: issue + commit + push + MR")
    parser.add_argument("--project-id", required=True, help="GitLab project ID")
    parser.add_argument("--project-name", required=True, help="Project directory name")
    parser.add_argument("--branch", help="Active branch name to commit and push")
    parser.add_argument("--branch-wip", help="Deprecated: current branch name")
    parser.add_argument("--branch-final", help="Deprecated: final branch name")
    parser.add_argument("--commit-message", required=True, help="Git commit message")
    parser.add_argument("--default-branch", default="main", help="Target branch for MR")
    # Issue args (skip creation if --issue-iid provided)
    parser.add_argument("--issue-iid", type=int, help="Existing issue IID (skip creation)")
    parser.add_argument("--issue-title", default="", help="Issue title (for creation)")
    parser.add_argument("--issue-description", default="", help="Issue description (for creation)")
    # MR args
    parser.add_argument("--mr-title", required=True, help="Merge request title")
    parser.add_argument("--mr-description", default="", help="Merge request description")
    args = parser.parse_args()

    project_dir = WORKSPACE / "gitlab_projects" / args.project_name
    result: dict = {}
    try:
        prepare_git_auth(project_dir)
    except Exception as e:
        fail("credentials", str(e), result)

    branch_name: str | None = None
    if args.branch:
        branch_name = args.branch
    elif args.branch_final or args.branch_wip:
        if args.branch_final and args.branch_wip and args.branch_final != args.branch_wip:
            fail("branch", "Branch rename is no longer supported; use a single branch name.")
        branch_name = args.branch_final or args.branch_wip
    if not branch_name:
        branch_name = get_active_branch(cwd=project_dir)
    if not branch_name:
        fail("branch", "Active branch not found; pass --branch or run setup first.")

    # 1. Create issue (or use existing)
    if args.issue_iid:
        result["issue_iid"] = args.issue_iid
        # Fetch issue web_url from API
        try:
            credentials = load_credentials()
            gitlab_url = credentials["gitlab_url"]
            url = f"{gitlab_url}/api/v4/projects/{args.project_id}/issues/{args.issue_iid}"
            headers = {"PRIVATE-TOKEN": credentials["access_token"]}
            with httpx.Client(timeout=15.0) as client:
                resp = client.get(url, headers=headers)
                resp.raise_for_status()
                result["issue_url"] = resp.json().get("web_url", "")
        except Exception:  # noqa: S110
            pass  # Non-critical: issue_url is informational
    else:
        try:
            issue = create_issue(
                project_id=args.project_id,
                title=args.issue_title,
                description=args.issue_description or None,
            )
            result["issue_iid"] = issue["iid"]
            result["issue_url"] = issue["web_url"]
        except Exception as e:
            fail("issue", str(e))

    # 2. Ensure branch exists and checkout
    try:
        run_git(["show-ref", "--verify", f"refs/heads/{branch_name}"], cwd=project_dir)
        run_git(["checkout", branch_name], cwd=project_dir)
        result["branch"] = branch_name
    except Exception as e:
        fail("branch", str(e), result)

    # 3. Stage all changes
    try:
        run_git(["add", "-A"], cwd=project_dir)
    except Exception as e:
        fail("add", str(e), result)

    # 4. Commit (skip if nothing to commit — idempotent)
    try:
        status = run_git(["status", "--porcelain"], cwd=project_dir)
        if status.strip():
            run_git(["commit", "-m", args.commit_message], cwd=project_dir)
        commit_hash = run_git(["rev-parse", "--short", "HEAD"], cwd=project_dir)
        result["commit"] = commit_hash
    except Exception as e:
        fail("commit", str(e), result)

    # 5. Push (treat "everything up-to-date" as success — idempotent)
    try:
        run_git_with_credentials(["push", "-u", "origin", branch_name], cwd=project_dir)
    except RuntimeError as e:
        if "everything up-to-date" in str(e).lower():
            pass  # Already pushed — idempotent success
        else:
            fail("push", str(e), result)
    except Exception as e:
        fail("push", str(e), result)

    # 6. Create MR (find existing on 409 — idempotent)
    try:
        iid = result["issue_iid"]
        mr_desc = args.mr_description or f"Closes #{iid}"
        try:
            mr = create_merge_request(
                project_id=args.project_id,
                source_branch=branch_name,
                target_branch=args.default_branch,
                title=args.mr_title,
                description=mr_desc,
            )
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 409:
                # MR already exists — find and return it
                mr = find_existing_mr(
                    args.project_id,
                    branch_name,
                    args.default_branch,
                )
                if not mr:
                    fail("mr", "MR already exists but could not be retrieved", result)
            else:
                raise
        result["mr_iid"] = mr["iid"]
        result["mr_url"] = mr["web_url"]
    except Exception as e:
        fail("mr", str(e), result)

    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
