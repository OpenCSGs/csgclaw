"""Shared utilities for GitLab fullstack skill scripts.

Consolidates credentials, git operations, and error handling used by
all scripts in this skill.
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib import error, parse, request
from urllib.parse import urlsplit, urlunsplit

# Workspace paths
WORKSPACE = Path(
    os.getenv("CSGCLAW_WORKSPACE", "").strip()
    or os.getenv("CODEX_WORKSPACE", "").strip()
    or os.getcwd()
)
SCRIPT_DIR = Path(__file__).parent
SKILL_DIR = SCRIPT_DIR.parent
ENV_GITLAB_URL = "GITLAB_BASE_URL"
ENV_GITLAB_TOKEN = "GITLAB_TOKEN"
ENV_GITLAB_HOST = "GITLAB_HOST"
ENV_CSGCLAW_BASE_URL = "CSGCLAW_BASE_URL"
ENV_CSGCLAW_ACCESS_TOKEN = "CSGCLAW_ACCESS_TOKEN"
MANAGER_GITLAB_CREDENTIAL_PATH = "/api/v1/agents/agent-manager/connectors/gitlab/credential"

_CREDENTIAL_MISSING_MSG = (
    "CSGClaw GitLab Connector 未配置或无法获取 lease。请用户在 Connector 中检查 Base URL 与 Token；"
    "不要要求用户在对话中发送 Token。"
)
DEFAULT_GIT_AUTH_USERNAME = "oauth2"

_REDACT_PATTERNS = (
    (re.compile(r"(https?://[^:/@\s]+:)[^@\s]+(@)"), r"\1***\2"),
    (re.compile(r"(?i)(authorization:\s*bearer\s+)[^\s,;]+"), r"\1***"),
    (re.compile(r"(?i)(access[_-]?token[\"'=:\s]+)[^,\"'\s]+"), r"\1***"),
    (re.compile(r"(?i)(private-token[\"'=:\s]+)[^,\"'\s]+"), r"\1***"),
)


def _redact_secret(value: str) -> str:
    redacted = value
    for pattern, replacement in _REDACT_PATTERNS:
        redacted = pattern.sub(replacement, redacted)
    return redacted


def load_credentials() -> dict:
    """Load a fresh GitLab credential lease from the CSGClaw Manager connector."""
    connector_creds = _load_csgclaw_connector_credentials()
    if connector_creds:
        _sync_env_from_credentials(connector_creds)
        return connector_creds
    raise ValueError(_CREDENTIAL_MISSING_MSG)


def _load_csgclaw_connector_credentials() -> dict | None:
    base_url = os.getenv(ENV_CSGCLAW_BASE_URL, "").strip().rstrip("/")
    access_token = os.getenv(ENV_CSGCLAW_ACCESS_TOKEN, "").strip()
    if not base_url or not access_token:
        return None

    endpoint = f"{base_url}{MANAGER_GITLAB_CREDENTIAL_PATH}"
    req = request.Request(
        endpoint,
        method="POST",
        headers={"Authorization": f"Bearer {access_token}", "Accept": "application/json"},
    )
    try:
        with request.urlopen(req, timeout=15) as response:  # noqa: S310
            payload = json.loads(response.read(1024 * 1024).decode("utf-8"))
    except (error.HTTPError, error.URLError, TimeoutError, json.JSONDecodeError) as exc:
        raise ValueError(f"获取 CSGClaw GitLab connector lease 失败: {_redact_secret(str(exc))}") from exc

    gitlab_url = str(payload.get("base_url") or "").strip().rstrip("/")
    token = str(payload.get("access_token") or "").strip()
    if not gitlab_url or not token:
        raise ValueError("CSGClaw GitLab connector lease 缺少 base_url 或 access_token")
    return {"gitlab_url": gitlab_url, "access_token": token}


def _sync_env_from_credentials(creds: dict) -> None:
    os.environ[ENV_GITLAB_URL] = creds["gitlab_url"].strip().rstrip("/")
    os.environ[ENV_GITLAB_TOKEN] = creds["access_token"].strip()
    os.environ[ENV_GITLAB_HOST] = _url_to_host(creds["gitlab_url"])


def _url_to_host(url: str) -> str:
    parsed = parse.urlparse(url if "://" in url else f"https://{url}")
    return (parsed.hostname or parsed.path.split("/")[0] or "").strip()


def git_repo_url(project_path: str, credentials: dict | None = None) -> str:
    """Return a clean HTTPS Git URL with no embedded credential material."""
    resolved = credentials or load_credentials()
    base_url = str(resolved["gitlab_url"]).rstrip("/")
    return f"{base_url}/{project_path.strip('/')}.git"


def git_credential_helper() -> str:
    """Git credential.helper snippet using GITLAB_TOKEN from process env."""
    return (
        '!f() { if [ "$1" = get ]; then '
        f'echo "username={DEFAULT_GIT_AUTH_USERNAME}"; echo "password=${{GITLAB_TOKEN}}"; '
        "fi; }; f"
    )


def scrub_git_remote_url(url: str) -> str:
    """Remove username/password material from an HTTPS remote URL."""
    parts = urlsplit(url)
    if not parts.scheme or not parts.netloc or "@" not in parts.netloc:
        return url

    host = parts.hostname or ""
    if ":" in host and not host.startswith("["):
        host = f"[{host}]"
    netloc = f"{host}:{parts.port}" if parts.port else host
    return urlunsplit((parts.scheme, netloc, parts.path, parts.query, parts.fragment))


def configure_git_credentials(cwd: Path) -> None:
    """Configure the repo to resolve Git HTTPS credentials via env (after load_credentials)."""
    creds = load_credentials()
    _sync_env_from_credentials(creds)
    for args in (
        ["config", "credential.helper", git_credential_helper()],
        ["config", "credential.useHttpPath", "false"],
    ):
        result = subprocess.run(  # noqa: S603
            ["git", *args],  # noqa: S607
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=30,
            check=False,
        )
        if result.returncode != 0:
            msg = f"git {args[0]} failed: {_redact_secret(result.stderr.strip())}"
            raise RuntimeError(msg)


def set_clean_origin(cwd: Path, remote_url: str) -> None:
    """Set origin to a clean HTTPS URL and attach the env-backed credential helper."""
    clean_url = scrub_git_remote_url(remote_url)
    result = subprocess.run(  # noqa: S603
        ["git", "remote", "set-url", "origin", clean_url],  # noqa: S607
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    if result.returncode != 0:
        msg = f"git remote set-url failed: {_redact_secret(result.stderr.strip())}"
        raise RuntimeError(msg)
    configure_git_credentials(cwd)


def run_git_with_credentials(args: list[str], cwd: Path, *, timeout: int = 120) -> str:
    """Run a Git command with a one-shot env-backed credential helper."""
    creds = load_credentials()
    _sync_env_from_credentials(creds)
    result = subprocess.run(  # noqa: S603
        ["git", "-c", f"credential.helper={git_credential_helper()}", *args],  # noqa: S607
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=timeout,
        check=False,
    )
    if result.returncode != 0:
        msg = f"git {args[0]} failed: {_redact_secret(result.stderr.strip())}"
        raise RuntimeError(msg)
    return result.stdout.strip()


def run_git(args: list[str], cwd: Path) -> str:
    """Run a git command and return stdout. Raises RuntimeError on failure."""
    result = subprocess.run(  # noqa: S603
        ["git", *args],  # noqa: S607
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=60,
        check=False,
    )
    if result.returncode != 0:
        msg = f"git {args[0]} failed: {result.stderr.strip()}"
        raise RuntimeError(msg)
    return result.stdout.strip()


def run_git_optional(args: list[str], cwd: Path) -> str | None:
    """Run a git command and return stdout or None when it fails."""
    result = subprocess.run(  # noqa: S603
        ["git", *args],  # noqa: S607
        cwd=cwd,
        capture_output=True,
        text=True,
        timeout=30,
        check=False,
    )
    if result.returncode != 0:
        return None
    return result.stdout.strip()


def fail(step: str, error: str, partial: dict | None = None) -> None:
    """Print structured error JSON to stderr and exit."""
    output = {"error": error, "step": step}
    if partial:
        output.update(partial)
    print(json.dumps(output), file=sys.stderr)
    sys.exit(1)
