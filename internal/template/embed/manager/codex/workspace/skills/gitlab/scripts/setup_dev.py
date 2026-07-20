"""GitLab project setup: credentials + project info + clone + git config + branch.

Consolidates the entire setup phase into a single script call.
All operations are idempotent — safe to re-run on the same project.
Usage: python3 scripts/setup_dev.py "group/project" --branch "add-quicksort"
"""

from __future__ import annotations

import argparse
import json
import shutil
from pathlib import Path  # noqa: TC003

from get_project import get_project
from shared_gitlab_utils import (
    WORKSPACE,
    configure_git_credentials,
    fail,
    git_repo_url,
    load_credentials,
    run_git,
    run_git_with_credentials,
    set_clean_origin,
)


def list_local_branches(cwd: Path) -> set[str]:
    """Return local branch names."""
    output = run_git(["for-each-ref", "--format=%(refname:short)", "refs/heads"], cwd=cwd)
    return {line.strip() for line in output.splitlines() if line.strip()}


def list_remote_branches(cwd: Path) -> set[str]:
    """Return remote branch names for origin, excluding origin/HEAD."""
    output = run_git(["for-each-ref", "--format=%(refname:short)", "refs/remotes/origin"], cwd=cwd)
    branches = {line.strip() for line in output.splitlines() if line.strip()}
    normalized: set[str] = set()
    for name in branches:
        if name == "origin/HEAD":
            continue
        if name.startswith("origin/"):
            normalized.add(name[len("origin/") :])
        else:
            normalized.add(name)
    return normalized


def ensure_unique_branch_name(base: str, existing: set[str]) -> str:
    """Ensure branch name is unique by appending numeric suffixes."""
    if base not in existing:
        return base
    index = 2
    while True:
        candidate = f"{base}-{index}"
        if candidate not in existing:
            return candidate
        index += 1


def set_active_branch(cwd: Path, branch_name: str) -> None:
    """Persist active branch in local git config for later runs."""
    run_git(["config", "csgbot.activebranch", branch_name], cwd=cwd)


def clone_or_update(
    clone_dir: Path,
    clone_url: str,
    default_branch: str,
) -> None:
    """Clone the repository, or reset and update if already cloned.

    Handles three states:
    - Directory doesn't exist: fresh clone
    - Directory exists as git repo: reset to default branch and fetch
    - Directory exists but not a git repo: remove and re-clone
    """
    if clone_dir.exists() and (clone_dir / ".git").is_dir():
        # Already cloned — reset to clean state and fetch updates
        set_clean_origin(clone_dir, clone_url)
        run_git_with_credentials(["fetch", "--all", "--prune"], cwd=clone_dir)
        run_git(["checkout", default_branch], cwd=clone_dir)
        run_git(["reset", "--hard", f"origin/{default_branch}"], cwd=clone_dir)
        run_git(["clean", "-fd"], cwd=clone_dir)
        return

    if clone_dir.exists():
        # Exists but not a git repo — remove and start fresh
        shutil.rmtree(clone_dir)

    clone_dir.parent.mkdir(parents=True, exist_ok=True)
    # --no-single-branch: fetch all branch tips so remote branches are visible
    run_git_with_credentials(
        ["clone", "--depth", "1", "--no-single-branch", clone_url, str(clone_dir)],
        cwd=clone_dir.parent,
    )
    configure_git_credentials(clone_dir)


def main() -> None:
    parser = argparse.ArgumentParser(description="Setup GitLab project workspace")
    parser.add_argument("project_path", help='GitLab project path (e.g. "group/project")')
    parser.add_argument("--branch", required=True, help="Branch short description (kebab-case)")
    parser.add_argument(
        "--base-branch",
        default=None,
        help="Create feature branch from this branch instead of default branch (for dependent sub-issues)",
    )
    args = parser.parse_args()

    # 1. Credentials
    try:
        creds = load_credentials()
    except Exception as e:
        fail("credentials", str(e))
    username = creds.get("username", "csgbot")

    # 2. Project info
    try:
        project = get_project(args.project_path)
    except Exception as e:
        fail("project", str(e))
    name = project["name"]
    project_id = project["id"]
    default_branch = project.get("default_branch", "main")

    # 3. Clone or update (idempotent)
    clone_dir = WORKSPACE / "gitlab_projects" / name
    clone_url = git_repo_url(args.project_path, creds)
    try:
        clone_or_update(clone_dir, clone_url, default_branch)
    except Exception as e:
        fail("clone", str(e))

    # 4. Git config (idempotent — overwrites silently)
    try:
        run_git(["config", "user.name", username], cwd=clone_dir)
        run_git(["config", "user.email", f"{username}@users.noreply.gitlab.com"], cwd=clone_dir)
    except Exception as e:
        fail("config", str(e))

    # 5. Fetch remote branches and create/switch to feature branch
    try:
        run_git(["fetch", "--all", "--prune"], cwd=clone_dir)
        local_branches = list_local_branches(cwd=clone_dir)
        remote_branches = list_remote_branches(cwd=clone_dir)
        existing = local_branches | remote_branches

        base_branch = f"feature/{args.branch}"
        if base_branch in local_branches:
            # Branch exists from prior run — just switch to it
            run_git(["checkout", base_branch], cwd=clone_dir)
            branch_name = base_branch
        else:
            # If --base-branch specified, checkout that branch first
            if args.base_branch:
                start_point = args.base_branch
                if start_point in local_branches:
                    run_git(["checkout", start_point], cwd=clone_dir)
                elif start_point in remote_branches:
                    run_git(["checkout", "-b", start_point, f"origin/{start_point}"], cwd=clone_dir)
                else:
                    fail("branch", f"Base branch '{start_point}' not found locally or remotely")
            branch_name = ensure_unique_branch_name(base_branch, existing)
            run_git(["checkout", "-b", branch_name], cwd=clone_dir)

        set_active_branch(cwd=clone_dir, branch_name=branch_name)
    except Exception as e:
        fail("branch", str(e))

    print(
        json.dumps(
            {
                "project_id": project_id,
                "project_name": name,
                "project_path": project["path_with_namespace"],
                "default_branch": default_branch,
                "web_url": project["web_url"],
                "username": username,
                "branch": branch_name,
                "active_branch": branch_name,
                "base_branch": args.base_branch or default_branch,
                "clone_dir": f"./gitlab_projects/{name}",
            },
            indent=2,
        ),
    )


if __name__ == "__main__":
    main()
