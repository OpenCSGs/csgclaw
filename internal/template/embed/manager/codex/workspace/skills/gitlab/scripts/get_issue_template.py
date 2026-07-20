#!/usr/bin/env python3
"""Fetch project default issue description template (issues_template).

Template filling/rendering is done by the agent (LLM) in conversation, not in this script.

Usage:
  python3 get_issue_template.py product/agentichub/requirements
  python3 get_issue_template.py --project-id 312
"""

from __future__ import annotations

import argparse
import json
import sys
from urllib.parse import quote

import httpx
from shared_gitlab_utils import load_credentials


def _project_ref(project_id: int | str) -> str:
    """Return API-safe project id or URL-encoded path."""
    if isinstance(project_id, int):
        return str(project_id)
    project_path = str(project_id)
    if "/" in project_path and "%" not in project_path:
        return quote(project_path, safe="")
    return project_path


def fetch_default_issue_description(project_id: int | str) -> str | None:
    """Fetch project default issue description template (issues_template)."""
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"].rstrip("/")
    access_token = credentials["access_token"]
    ref = _project_ref(project_id)

    url = f"{gitlab_url}/api/v4/projects/{ref}"
    headers = {"PRIVATE-TOKEN": access_token}

    with httpx.Client(timeout=30.0) as client:
        response = client.get(url, headers=headers)
        response.raise_for_status()
        template = response.json().get("issues_template")

    if not template or not str(template).strip():
        return None
    return str(template).replace("\r\n", "\n")


def main() -> None:
    parser = argparse.ArgumentParser(description="Fetch GitLab project default issue template")
    parser.add_argument(
        "project",
        nargs="?",
        help="Project path (e.g. product/agentichub/requirements) or numeric id",
    )
    parser.add_argument(
        "--project-id",
        dest="project_id",
        help="Alias for project path or id",
    )
    args = parser.parse_args()

    project_ref = args.project_id or args.project
    if not project_ref:
        parser.print_help()
        sys.exit(1)

    try:
        template = fetch_default_issue_description(project_ref)
        print(
            json.dumps(
                {
                    "project": project_ref,
                    "issues_template": template,
                    "has_template": bool(template and template.strip()),
                },
                ensure_ascii=False,
                indent=2,
            ),
        )
        sys.exit(0)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
