#!/usr/bin/env python3
"""Rewrite GitHub release download URLs in install.sh for a mirror base URL."""
import pathlib
import re
import sys


def main() -> None:
    if len(sys.argv) != 5:
        print(
            "usage: patch-install-for-mirror.py <install.sh path> <owner> <repo> <base_url>",
            file=sys.stderr,
        )
        raise SystemExit(2)
    script_path = pathlib.Path(sys.argv[1])
    owner = sys.argv[2]
    repo = sys.argv[3]
    base = sys.argv[4].rstrip("/")
    text = script_path.read_text()
    old_expr = "https://github.com/${REPO}/releases/download"
    new_expr = f"{base}/releases"
    patched = text.replace(old_expr, new_expr)
    if patched == text:
        pattern = rf"https://github\.com/{re.escape(owner)}/{re.escape(repo)}/releases/download"
        patched = re.sub(pattern, new_expr, text)
    script_path.write_text(patched)


if __name__ == "__main__":
    main()
