#!/usr/bin/env python3
"""Run one command with an in-memory CSGClaw GitLab connector lease."""

from __future__ import annotations

import os
import subprocess
import sys

from gitlab_utils import ENV_GITLAB_HOST, ENV_GITLAB_TOKEN, ENV_GITLAB_URL, _redact_secret, _url_to_host, load_credentials


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: run_with_gitlab_auth.py <command> [args...]", file=sys.stderr)
        return 2

    try:
        credentials = load_credentials()
    except ValueError as exc:
        print(_redact_secret(str(exc)), file=sys.stderr)
        return 1

    env = os.environ.copy()
    env[ENV_GITLAB_URL] = credentials["gitlab_url"]
    env[ENV_GITLAB_TOKEN] = credentials["access_token"]
    env[ENV_GITLAB_HOST] = _url_to_host(credentials["gitlab_url"])
    result = subprocess.run(sys.argv[1:], env=env, capture_output=True, text=True, check=False)  # noqa: S603
    if result.stdout:
        print(_redact_secret(result.stdout), end="")
    if result.stderr:
        print(_redact_secret(result.stderr), end="", file=sys.stderr)
    return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
