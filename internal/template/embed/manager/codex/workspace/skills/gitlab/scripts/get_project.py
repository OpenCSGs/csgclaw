#!/usr/bin/env python3
"""GitLab project info retrieval script.

Retrieves project information from GitLab API by project path.
Usage: python3 get_project.py "group/project"
"""

from __future__ import annotations

import json
import sys
from urllib.parse import quote

import httpx
from shared_gitlab_utils import load_credentials


def get_project(project_path: str) -> dict:
    """Get project info from GitLab API.

    Args:
        project_path: Project path in format "group/project" or "group/subgroup/project"

    Returns:
        Project info dict with id, name, default_branch, web_url, etc.

    Raises:
        httpx.HTTPStatusError: If API request fails.
    """
    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    access_token = credentials["access_token"]

    encoded_path = quote(project_path, safe="")
    url = f"{gitlab_url}/api/v4/projects/{encoded_path}"

    headers = {
        "PRIVATE-TOKEN": access_token,
        "Content-Type": "application/json",
    }

    with httpx.Client(timeout=30.0) as client:
        response = client.get(url, headers=headers)
        response.raise_for_status()
        return response.json()


def main():
    """Main entry point."""
    if len(sys.argv) < 2:
        print("Usage: python3 get_project.py <project_path>", file=sys.stderr)
        print("Example: python3 get_project.py 'mygroup/myproject'", file=sys.stderr)
        sys.exit(1)

    project_path = sys.argv[1]

    try:
        project = get_project(project_path)

        output = {
            "id": project["id"],
            "name": project["name"],
            "path_with_namespace": project["path_with_namespace"],
            "default_branch": project.get("default_branch", "main"),
            "web_url": project["web_url"],
            "http_url_to_repo": project.get("http_url_to_repo", ""),
        }
        print(json.dumps(output, indent=2))
        sys.exit(0)

    except FileNotFoundError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    except httpx.HTTPStatusError as e:
        if e.response.status_code == 401:
            print("Error: Authentication failed. Check your access token.", file=sys.stderr)
        elif e.response.status_code == 404:
            print(f"Error: Project not found: {project_path}", file=sys.stderr)
        else:
            print(f"Error: GitLab API error: {e.response.status_code}", file=sys.stderr)
        sys.exit(2)

    except Exception as e:
        print(f"Unexpected error: {e}", file=sys.stderr)
        sys.exit(3)


if __name__ == "__main__":
    main()
