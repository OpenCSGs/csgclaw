#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def run_gh(args: list[str], *, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["gh", *args],
        check=check,
        capture_output=True,
        text=True,
    )


def load_json_file(path: Path) -> Any:
    with path.open("r", encoding="utf-8") as fh:
        return json.load(fh)


def dump_json(data: Any, path: Path | None) -> None:
    text = json.dumps(data, indent=2, ensure_ascii=False) + "\n"
    if path is None:
        sys.stdout.write(text)
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8")


def looks_like_credential_store_access_issue(message: str) -> bool:
    text = message.lower()
    markers = [
        "keychain",
        "credential",
        "credentials",
        "secretservice",
        "sandbox",
        "operation not permitted",
        "not allowed",
        "non-interactive",
        "interaction is not allowed",
        "user interaction is not allowed",
    ]
    return any(marker in text for marker in markers)


def require_gh_auth() -> None:
    result = run_gh(["auth", "status"], check=False)
    if result.returncode != 0:
        message = result.stderr.strip() or result.stdout.strip() or "gh auth status failed"
        if looks_like_credential_store_access_issue(message):
            raise SystemExit(
                "GitHub CLI authentication is configured but unavailable in the current "
                "non-interactive session. The sandbox likely cannot access the system "
                f"credential store or keychain. Retry the `gh` command with escalated "
                f"permissions before falling back. Original error: {message}"
            )
        raise SystemExit(f"GitHub CLI authentication is unavailable: {message}")


def normalize_repo_arg(repo: str | None) -> list[str]:
    return ["--repo", repo] if repo else []


def resolve_repo(repo: str | None) -> str:
    if repo:
        return repo
    result = run_gh(["repo", "view", "--json", "nameWithOwner"])
    payload = json.loads(result.stdout)
    return payload["nameWithOwner"]


def collect_pr_payload(repo: str, pr: int) -> dict[str, Any]:
    view_args = [
        "pr",
        "view",
        str(pr),
        *normalize_repo_arg(repo),
        "--json",
        "number,title,body,files,commits,headRefOid,baseRefName,headRefName,url",
    ]
    diff_args = ["pr", "diff", str(pr), *normalize_repo_arg(repo), "--patch"]

    view = json.loads(run_gh(view_args).stdout)
    diff = run_gh(diff_args).stdout

    return {
        "repo": repo,
        "pr": pr,
        "head_sha": view["headRefOid"],
        "title": view["title"],
        "body": view["body"],
        "base_ref": view["baseRefName"],
        "head_ref": view["headRefName"],
        "url": view["url"],
        "files": view["files"],
        "commits": view["commits"],
        "diff": diff,
        "collected_at": datetime.now(timezone.utc).isoformat(),
    }


@dataclass
class CommentDraft:
    repo: str
    pr: int
    head_sha: str
    path: str
    line: int
    body: str
    side: str = "RIGHT"
    severity: str | None = None
    selected: bool = True

    @classmethod
    def from_dict(cls, item: dict[str, Any], default_repo: str, default_pr: int, default_head_sha: str) -> "CommentDraft":
        missing = [
            key
            for key in ("path", "line", "body")
            if key not in item or item[key] in ("", None)
        ]
        if missing:
            raise ValueError(f"missing required comment fields: {', '.join(missing)}")

        line = item["line"]
        if not isinstance(line, int) or line <= 0:
            raise ValueError("line must be a positive integer")

        side = item.get("side", "RIGHT")
        if side not in {"LEFT", "RIGHT"}:
            raise ValueError("side must be LEFT or RIGHT")

        return cls(
            repo=item.get("repo", default_repo),
            pr=int(item.get("pr", default_pr)),
            head_sha=item.get("head_sha", default_head_sha),
            path=str(item["path"]),
            line=line,
            body=str(item["body"]).strip(),
            side=side,
            severity=item.get("severity"),
            selected=bool(item.get("selected", True)),
        )


def load_comment_drafts(path: Path) -> tuple[dict[str, Any], list[CommentDraft]]:
    payload = load_json_file(path)
    if not isinstance(payload, dict):
        raise SystemExit("comments payload must be a JSON object")

    missing = [
        key
        for key in ("repo", "pr", "head_sha", "comments")
        if key not in payload or payload[key] in ("", None)
    ]
    if missing:
        raise SystemExit(f"comments payload missing top-level fields: {', '.join(missing)}")

    comments = payload["comments"]
    if not isinstance(comments, list):
        raise SystemExit("comments must be a JSON array")

    drafts = [
        CommentDraft.from_dict(item, payload["repo"], int(payload["pr"]), payload["head_sha"])
        for item in comments
    ]
    return payload, drafts


def refresh_head_sha(repo: str, pr: int) -> str:
    args = [
        "pr",
        "view",
        str(pr),
        *normalize_repo_arg(repo),
        "--json",
        "headRefOid",
    ]
    view = json.loads(run_gh(args).stdout)
    return view["headRefOid"]


def post_inline_comment(draft: CommentDraft) -> dict[str, Any]:
    args = [
        "api",
        "--method",
        "POST",
        f"repos/{draft.repo}/pulls/{draft.pr}/comments",
        "-f",
        f"body={draft.body}",
        "-f",
        f"commit_id={draft.head_sha}",
        "-f",
        f"path={draft.path}",
        "-F",
        f"line={draft.line}",
        "-f",
        f"side={draft.side}",
    ]
    result = json.loads(run_gh(args).stdout)
    return {
        "id": result.get("id"),
        "url": result.get("html_url") or result.get("url"),
        "path": draft.path,
        "line": draft.line,
        "severity": draft.severity,
        "body": draft.body,
    }


def cmd_collect(args: argparse.Namespace) -> int:
    require_gh_auth()
    repo = resolve_repo(args.repo)
    payload = collect_pr_payload(repo, args.pr)
    dump_json(payload, Path(args.output) if args.output else None)
    return 0


def cmd_post(args: argparse.Namespace) -> int:
    require_gh_auth()
    payload, drafts = load_comment_drafts(Path(args.input))
    selected = [draft for draft in drafts if draft.selected]
    if not selected:
        raise SystemExit("no selected comments to post")

    current_head_sha = refresh_head_sha(payload["repo"], int(payload["pr"]))
    if current_head_sha != payload["head_sha"]:
        raise SystemExit(
            "PR head SHA changed since preview; refresh the PR data and rebuild the posting plan"
        )

    report = {
        "repo": payload["repo"],
        "pr": int(payload["pr"]),
        "head_sha": current_head_sha,
        "selected_count": len(selected),
        "dry_run": not args.confirm,
        "comments": [],
    }

    for draft in selected:
        entry = {
            "path": draft.path,
            "line": draft.line,
            "severity": draft.severity,
            "body": draft.body,
            "side": draft.side,
        }
        if args.confirm:
            entry["posted"] = post_inline_comment(draft)
        report["comments"].append(entry)

    dump_json(report, Path(args.output) if args.output else None)
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Collect PR review inputs with gh and post confirmed inline review comments."
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    collect = subparsers.add_parser(
        "collect",
        help="Fetch PR metadata and patch text with gh and emit a structured JSON payload.",
    )
    collect.add_argument("--repo", help="GitHub repo in owner/repo form. Defaults to the current checkout.")
    collect.add_argument("--pr", required=True, type=int, help="Pull request number.")
    collect.add_argument("--output", help="Write JSON output to this file instead of stdout.")
    collect.set_defaults(func=cmd_collect)

    post = subparsers.add_parser(
        "post",
        help="Post selected inline review comments from a prepared JSON payload.",
    )
    post.add_argument("--input", required=True, help="Path to a JSON file with repo, pr, head_sha, and comments.")
    post.add_argument("--output", help="Write the posting report to this file instead of stdout.")
    post.add_argument(
        "--confirm",
        action="store_true",
        help="Actually publish comments. Without this flag, only emit a dry-run report.",
    )
    post.set_defaults(func=cmd_post)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    raise SystemExit(main())
