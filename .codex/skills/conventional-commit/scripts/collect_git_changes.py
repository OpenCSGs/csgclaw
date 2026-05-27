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


def git_output(repo: Path, args: list[str], check: bool = True) -> str:
    return run_git(repo, args, check=check).stdout


def has_staged_changes(repo: Path) -> bool:
    result = subprocess.run(
        ["git", "diff", "--cached", "--quiet"],
        cwd=repo,
        text=True,
        capture_output=True,
    )
    return result.returncode == 1


def ensure_git_repo(repo: Path) -> None:
    result = subprocess.run(
        ["git", "rev-parse", "--show-toplevel"],
        cwd=repo,
        text=True,
        capture_output=True,
    )
    if result.returncode != 0:
        message = result.stderr.strip() or f"{repo} is not a git repository"
        raise SystemExit(message)


def clip_lines(text: str, max_lines: int) -> str:
    lines = text.splitlines()
    if len(lines) <= max_lines:
        return text.rstrip()
    clipped = lines[:max_lines]
    clipped.append(f"... ({len(lines) - max_lines} more lines omitted)")
    return "\n".join(clipped)


def file_patch_for_untracked(repo: Path, relpath: str) -> str:
    file_path = repo / relpath
    result = subprocess.run(
        ["git", "diff", "--no-index", "--", "/dev/null", str(file_path)],
        cwd=repo,
        text=True,
        capture_output=True,
    )
    if result.returncode not in (0, 1):
        return f"# Unable to diff untracked file: {relpath}\n"
    return result.stdout


def collect(repo: Path, mode: str, max_patch_lines: int, include_untracked: bool) -> str:
    status_args = ["status", "--short", "--branch"]
    if not include_untracked:
        status_args.append("--untracked-files=no")
    status = git_output(repo, status_args)

    if mode == "auto":
        selected_mode = "staged" if has_staged_changes(repo) else "worktree"
    else:
        selected_mode = mode

    sections: list[tuple[str, str]] = [("Repository", str(repo.resolve())), ("Mode", selected_mode), ("Status", status.rstrip())]

    if selected_mode == "staged":
        name_status = git_output(repo, ["diff", "--cached", "--name-status"])
        diff_stat = git_output(repo, ["diff", "--cached", "--stat"])
        patch = git_output(repo, ["diff", "--cached", "--unified=3"])
    else:
        name_status = git_output(repo, ["diff", "--name-status"])
        diff_stat = git_output(repo, ["diff", "--stat"])
        patch = git_output(repo, ["diff", "--unified=3"])
        if include_untracked:
            untracked = git_output(repo, ["ls-files", "--others", "--exclude-standard"]).splitlines()
            if untracked:
                sections.append(("Untracked Files", "\n".join(untracked)))
                extra_patches = []
                for relpath in untracked:
                    extra_patches.append(file_patch_for_untracked(repo, relpath))
                if extra_patches:
                    if patch and not patch.endswith("\n"):
                        patch += "\n"
                    patch += "\n".join(extra_patches)

    sections.append(("Changed Files", name_status.rstrip() or "(none)"))
    sections.append(("Diff Stat", diff_stat.rstrip() or "(none)"))
    sections.append(("Patch", clip_lines(patch.rstrip(), max_patch_lines) or "(none)"))

    return "\n\n".join(f"## {title}\n{body}" for title, body in sections)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Collect git change context for Conventional Commit drafting.")
    parser.add_argument("--repo", default=".", help="Path to the git repository")
    parser.add_argument(
        "--mode",
        choices=["auto", "staged", "worktree"],
        default="auto",
        help="Change source to inspect",
    )
    parser.add_argument(
        "--max-patch-lines",
        type=int,
        default=400,
        help="Maximum number of patch lines to emit",
    )
    parser.add_argument(
        "--include-untracked",
        action="store_true",
        help="Include untracked files when inspecting worktree changes",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    repo = Path(os.path.expanduser(args.repo)).resolve()
    ensure_git_repo(repo)
    sys.stdout.write(collect(repo, args.mode, args.max_patch_lines, args.include_untracked))
    if not sys.stdout.isatty():
        sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
