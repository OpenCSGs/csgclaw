#!/usr/bin/env python3

import argparse
import os
import subprocess
import sys
from pathlib import Path


def run_git(repo: Path, args: list[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["git", *args],
        cwd=repo,
        text=True,
        capture_output=True,
        check=check,
    )


def ensure_git_repo(repo: Path) -> None:
    result = run_git(repo, ["rev-parse", "--show-toplevel"], check=False)
    if result.returncode != 0:
        message = result.stderr.strip() or f"{repo} is not a git repository"
        raise SystemExit(message)


def has_staged_changes(repo: Path) -> bool:
    result = run_git(repo, ["diff", "--cached", "--quiet"], check=False)
    return result.returncode == 1


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Commit staged changes with a prepared Conventional Commit message.")
    parser.add_argument("--repo", default=".", help="Path to the git repository")
    parser.add_argument("--message", required=True, help="Commit subject line")
    parser.add_argument("--body", default="", help="Optional commit body")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    repo = Path(os.path.expanduser(args.repo)).resolve()
    ensure_git_repo(repo)

    if not has_staged_changes(repo):
        raise SystemExit("No staged changes to commit")

    commit_args = ["commit", "-m", args.message]
    if args.body:
        commit_args.extend(["-m", args.body])

    result = run_git(repo, commit_args, check=False)
    if result.returncode != 0:
        sys.stderr.write(result.stderr)
        return result.returncode

    sys.stdout.write(result.stdout)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
