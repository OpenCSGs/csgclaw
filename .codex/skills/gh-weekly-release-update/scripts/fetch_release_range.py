#!/usr/bin/env python3
"""Fetch an inclusive GitHub release range with gh and emit JSON."""

from __future__ import annotations

import argparse
import json
import subprocess
import sys


def run_gh(args: list[str]) -> str:
    try:
        result = subprocess.run(
            ["gh", *args],
            check=True,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError as exc:
        raise RuntimeError("gh CLI is not installed or not on PATH.") from exc
    except subprocess.CalledProcessError as exc:
        stderr = exc.stderr.strip() or exc.stdout.strip() or "gh command failed"
        raise RuntimeError(stderr) from exc
    return result.stdout


def fetch_page(repo: str, page: int, per_page: int) -> list[dict[str, object]]:
    output = run_gh(
        [
            "api",
            f"repos/{repo}/releases",
            "--method",
            "GET",
            "--field",
            f"per_page={per_page}",
            "--field",
            f"page={page}",
        ]
    )
    data = json.loads(output)
    if not isinstance(data, list):
        raise RuntimeError("Unexpected GitHub API response for releases.")
    return data


def normalize_release(release: dict[str, object]) -> dict[str, object]:
    return {
        "tag_name": release.get("tag_name"),
        "name": release.get("name"),
        "published_at": release.get("published_at"),
        "prerelease": bool(release.get("prerelease")),
        "draft": bool(release.get("draft")),
        "url": release.get("html_url"),
        "body": release.get("body") or "",
    }


def collect_range(
    repo: str,
    from_tag: str,
    to_tag: str,
    per_page: int,
    max_pages: int,
) -> list[dict[str, object]]:
    releases: list[dict[str, object]] = []
    seen_tags: set[str] = set()

    for page in range(1, max_pages + 1):
        batch = fetch_page(repo, page=page, per_page=per_page)
        if not batch:
            break

        for raw in batch:
            release = normalize_release(raw)
            tag = str(release["tag_name"] or "")
            if not tag or release["draft"]:
                continue
            releases.append(release)
            seen_tags.add(tag)

        if from_tag in seen_tags and to_tag in seen_tags:
            break

    tag_to_index = {str(item["tag_name"]): idx for idx, item in enumerate(releases)}
    missing = [tag for tag in (from_tag, to_tag) if tag not in tag_to_index]
    if missing:
        joined = ", ".join(missing)
        raise RuntimeError(f"Release tag not found: {joined}")

    start = tag_to_index[from_tag]
    end = tag_to_index[to_tag]
    low = min(start, end)
    high = max(start, end)

    # GitHub returns releases newest first; reverse to make summaries chronological.
    return list(reversed(releases[low : high + 1]))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Fetch a closed GitHub release range and emit JSON."
    )
    parser.add_argument("--repo", required=True, help="Repository in owner/repo form.")
    parser.add_argument("--from-tag", required=True, help="Older or first tag.")
    parser.add_argument("--to-tag", required=True, help="Newer or second tag.")
    parser.add_argument(
        "--per-page",
        type=int,
        default=100,
        help="Releases to fetch per page. Defaults to 100.",
    )
    parser.add_argument(
        "--max-pages",
        type=int,
        default=10,
        help="Maximum number of API pages to fetch. Defaults to 10.",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        releases = collect_range(
            repo=args.repo,
            from_tag=args.from_tag,
            to_tag=args.to_tag,
            per_page=args.per_page,
            max_pages=args.max_pages,
        )
    except RuntimeError as exc:
        sys.stderr.write(f"{exc}\n")
        return 1

    payload = {
        "repo": args.repo,
        "from_tag": args.from_tag,
        "to_tag": args.to_tag,
        "count": len(releases),
        "releases": releases,
    }
    json.dump(payload, sys.stdout, indent=2, ensure_ascii=False)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
