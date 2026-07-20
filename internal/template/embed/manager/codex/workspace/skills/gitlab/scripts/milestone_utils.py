"""Shared milestone title normalization and group milestone resolution."""

from __future__ import annotations

from urllib.parse import quote

import httpx


def normalize_milestone_version(title: str) -> str:
    """Normalize milestone version for comparison (0.6.1 == v0.6.1)."""
    value = title.strip().lower().removeprefix("v")
    return value


def group_path_from_project(project_path: str) -> str:
    """Return group path from a project path like group/sub/project."""
    parts = project_path.strip("/").split("/")
    if len(parts) < 2:
        msg = f"Invalid project path for group extraction: {project_path}"
        raise ValueError(msg)
    return "/".join(parts[:-1])


def fetch_group_milestones(
    *,
    gitlab_url: str,
    access_token: str,
    group_path: str,
    state: str | None = None,
    per_page: int = 50,
    max_pages: int = 5,
) -> list[dict]:
    """List group milestones (active + closed when state is None)."""
    encoded = quote(group_path.strip(), safe="")
    url = f"{gitlab_url.rstrip('/')}/api/v4/groups/{encoded}/milestones"
    headers = {"PRIVATE-TOKEN": access_token, "Content-Type": "application/json"}
    all_items: list[dict] = []
    with httpx.Client(timeout=60.0) as client:
        for page in range(1, max_pages + 1):
            params: dict[str, str | int] = {"page": page, "per_page": per_page}
            if state:
                params["state"] = state
            response = client.get(url, headers=headers, params=params)
            response.raise_for_status()
            batch = response.json()
            if not batch:
                break
            all_items.extend(batch)
            if len(batch) < per_page:
                break
    return all_items


def resolve_milestone_title(
    milestones: list[dict],
    user_input: str,
) -> str | None:
    """Match user milestone text to the canonical GitLab milestone title."""
    target = normalize_milestone_version(user_input)
    if not target:
        return None
    matches = [
        milestone
        for milestone in milestones
        if normalize_milestone_version(str(milestone.get("title", ""))) == target
    ]
    if not matches:
        return None
    if len(matches) == 1:
        return str(matches[0]["title"])
    # Prefer active milestones, then the highest iid.
    active = [milestone for milestone in matches if milestone.get("state") == "active"]
    pool = active or matches
    pool.sort(key=lambda milestone: int(milestone.get("iid", 0)), reverse=True)
    return str(pool[0]["title"])


def resolve_group_milestone_title(
    *,
    gitlab_url: str,
    access_token: str,
    group_path: str,
    user_input: str,
) -> str | None:
    """Resolve milestone title from group milestones by version-like input."""
    milestones = fetch_group_milestones(
        gitlab_url=gitlab_url,
        access_token=access_token,
        group_path=group_path,
        state=None,
    )
    return resolve_milestone_title(milestones, user_input)


def milestone_matches(issue: dict, user_input: str) -> bool:
    """Return True when issue milestone matches the user version input."""
    milestone = issue.get("milestone") or {}
    title = milestone.get("title")
    if not title:
        return False
    return normalize_milestone_version(str(title)) == normalize_milestone_version(user_input)
