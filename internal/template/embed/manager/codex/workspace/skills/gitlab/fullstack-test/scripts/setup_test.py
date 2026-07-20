"""Clone GitLab project and prepare target/source directories for comparison testing.

Usage: python3 scripts/setup_test.py --project-path "group/project" \
    --source-branch "feature/fix" --target-branch "main"
"""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path

SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

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


def _find_existing_clone(project_name: str) -> Path | None:
    """Check if setup_dev has already cloned this project."""
    candidate = WORKSPACE / "gitlab_projects" / project_name
    if candidate.exists() and (candidate / ".git").is_dir():
        return candidate
    return None


def _clone_to(
    clone_url: str,
    dest: Path,
    branch: str,
    username: str,
    reference_dir: Path | None = None,
) -> None:
    """Clone repo to dest and checkout the specified branch.

    When *reference_dir* points to an existing local clone (e.g. left by
    setup_dev), the function clones from that directory first
    (fast, no network) and then points the remote back to the real GitLab
    URL so that ``fetch --all`` picks up every branch.
    """
    if dest.exists():
        shutil.rmtree(dest)
    dest.parent.mkdir(parents=True, exist_ok=True)

    if reference_dir:
        run_git_with_credentials(
            ["clone", "--no-hardlinks", str(reference_dir), str(dest)],
            cwd=dest.parent,
        )
        # Point remote to real GitLab URL so fetch gets all branches
        set_clean_origin(dest, clone_url)
    else:
        run_git_with_credentials(
            [
                "clone",
                "--depth",
                "50",
                "--no-single-branch",
                clone_url,
                str(dest),
            ],
            cwd=dest.parent,
        )
        configure_git_credentials(dest)

    # Configure git user
    run_git(["config", "user.name", username], cwd=dest)
    run_git(["config", "user.email", f"{username}@users.noreply.gitlab.com"], cwd=dest)

    # Fetch all remote branches and checkout target
    run_git_with_credentials(["fetch", "--all", "--prune"], cwd=dest)
    run_git(["checkout", branch], cwd=dest)


def main() -> None:
    parser = argparse.ArgumentParser(description="Setup test environment for MR verification")
    parser.add_argument("--project-path", required=True, help="GitLab project path (group/project)")
    parser.add_argument("--source-branch", required=True, help="MR source branch (fix branch)")
    parser.add_argument("--target-branch", required=True, help="MR target branch (e.g. main)")
    args = parser.parse_args()

    # Load credentials
    try:
        creds = load_credentials()
    except Exception as e:
        fail("credentials", str(e))

    username = creds.get("username", "csgbot")

    clone_url = git_repo_url(args.project_path, creds)
    project_name = args.project_path.rsplit("/", 1)[-1]

    base_dir = WORKSPACE / "gitlab_projects" / project_name
    target_dir = base_dir / "_target"
    source_dir = base_dir / "_source"

    # Reuse existing clone from setup_dev when available
    existing_clone = _find_existing_clone(project_name)

    # Clone target branch (baseline)
    try:
        _clone_to(clone_url, target_dir, args.target_branch, username, reference_dir=existing_clone)
    except Exception as e:
        fail("clone_target", str(e))

    # Clone source branch (fix)
    try:
        _clone_to(clone_url, source_dir, args.source_branch, username, reference_dir=existing_clone)
    except Exception as e:
        fail("clone_source", str(e))

    result = {
        "project_name": project_name,
        "source_dir": f"./gitlab_projects/{project_name}/_source",
        "target_dir": f"./gitlab_projects/{project_name}/_target",
        "source_branch": args.source_branch,
        "target_branch": args.target_branch,
        "reused_existing_clone": existing_clone is not None,
    }

    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except SystemExit:
        raise
    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)
