"""CSGClaw API helpers used by the Feishu skill."""

from __future__ import annotations

import json
import os
import subprocess
from typing import Any, Optional
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import Request, urlopen

from .config import API_REQUEST_TIMEOUT

ACTION_CARD_TYPE = "csgclaw.action_card"
MANAGER_REBUILD_ACTION_ID = "rebuild-manager"


def api_base(args) -> str:
    return (getattr(args, "csgclaw_base_url", "") or os.environ.get("CSGCLAW_BASE_URL") or "http://127.0.0.1:18080").rstrip("/")


def api_token(args) -> str:
    return getattr(args, "csgclaw_access_token", "") or os.environ.get("CSGCLAW_ACCESS_TOKEN", "")


def api_request_timeout(args) -> int:
    value = getattr(args, "api_timeout", None)
    if value is None:
        raw = os.environ.get("CSGCLAW_API_TIMEOUT", "").strip()
        if raw:
            try:
                value = int(raw)
            except ValueError:
                value = API_REQUEST_TIMEOUT
        else:
            value = API_REQUEST_TIMEOUT
    return max(1, int(value))


def path_id(value: str) -> str:
    return quote(value, safe="")


def api_json(args, method: str, path: str, body: Optional[dict] = None) -> Any:
    data = None if body is None else json.dumps(body).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    token = api_token(args)
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = Request(f"{api_base(args)}{path}", data=data, headers=headers, method=method)
    try:
        with urlopen(req, timeout=api_request_timeout(args)) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else None
    except HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"CSGClaw API {method} {path} failed: HTTP {exc.code}: {raw.strip()}") from None


def csgclaw_cli_env(args) -> dict[str, str]:
    env = os.environ.copy()
    base_url = getattr(args, "csgclaw_base_url", "") or os.environ.get("CSGCLAW_BASE_URL", "")
    token = api_token(args)
    if base_url:
        env["CSGCLAW_BASE_URL"] = base_url
    if token:
        env["CSGCLAW_ACCESS_TOKEN"] = token
    return env


def csgclaw_cli_json(args, cli_args: list[str], input_text: Optional[str] = None) -> Any:
    command = ["csgclaw-cli", "--output", "json", *cli_args]
    try:
        completed = subprocess.run(
            command,
            input=input_text,
            text=True,
            capture_output=True,
            timeout=api_request_timeout(args),
            env=csgclaw_cli_env(args),
            check=False,
        )
    except FileNotFoundError:
        raise RuntimeError("csgclaw-cli was not found in PATH") from None
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"csgclaw-cli timed out after {api_request_timeout(args)} seconds") from exc
    if completed.returncode != 0:
        detail = (completed.stderr or completed.stdout or "").strip()
        raise RuntimeError(f"csgclaw-cli {' '.join(cli_args)} failed: {detail}") from None
    raw = completed.stdout.strip()
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"csgclaw-cli returned invalid JSON: {raw}") from exc


def configure_csgclaw(args, state: dict, result: dict) -> dict:
    agent_id = state["agent_id"]
    response: dict[str, Any] = {}
    role = resolve_role(args, state)
    candidate_admin_open_id = str(result.get("open_id") or "").strip()
    if role == "manager" and candidate_admin_open_id:
        admin_bind_args = [
            "participant",
            "bind",
            "--channel",
            "feishu",
            "--feishu-kind",
            "human",
            "--admin",
            "--open-id",
            candidate_admin_open_id,
        ]
        candidate_admin_name = str(result.get("name") or "").strip()
        if candidate_admin_name:
            admin_bind_args.extend(["--name", candidate_admin_name])
        response["admin_bind"] = csgclaw_cli_json(
            args,
            admin_bind_args,
        )
    bot_bind_args = [
        "participant",
        "bind",
        "--channel",
        "feishu",
        "--feishu-kind",
        "bot",
        "--agent",
        agent_id,
        "--app-id",
        result["app_id"],
        "--app-secret-stdin",
    ]
    if role == "worker" and args.recreate in ("auto", "worker"):
        bot_bind_args.append("--restart")
    response["bot_bind"] = csgclaw_cli_json(args, bot_bind_args, input_text=result["app_secret"])
    if role == "manager" and candidate_admin_open_id:
        response["admin_open_id"] = candidate_admin_open_id
        response["admin_open_id_source"] = "registration"
    else:
        response.pop("admin_open_id", None)
    return response


def resolve_role(args, state: dict) -> str:
    agent_id = state["agent_id"]
    return args.role or state.get("role") or ("manager" if agent_id == "u-manager" else "worker")


def public_result(data: dict) -> dict:
    clean = dict(data)
    for key in ("app_secret", "client_secret", "access_token", "tenant_access_token"):
        if key in clean:
            clean[key] = "present"
    return clean


def manager_recreate_action_card(agent_id: str) -> dict:
    return {
        "type": ACTION_CARD_TYPE,
        "status": "manager_recreate_pending",
        "agent_id": agent_id,
        "bot_id": agent_id,
        "title": "Manager Feishu 配置已完成",
        "subtitle": agent_id,
        "badge": "需在窗口点击",
        "summary": (
            "飞书配置已写入并重新加载。"
            "Manager 需要重建后才能把新配置注入运行环境。"
            "请直接点击下方按钮，由浏览器发起安全的 Manager bootstrap replace。"
        ),
        "actions": [
            {
                "id": MANAGER_REBUILD_ACTION_ID,
                "label": "重建 Manager",
                "style": "danger",
                "method": "manager-bootstrap-replace",
                "confirm": "重建 Manager 会中断当前 Manager，会话可能需要刷新。确认继续？",
            }
        ],
    }
