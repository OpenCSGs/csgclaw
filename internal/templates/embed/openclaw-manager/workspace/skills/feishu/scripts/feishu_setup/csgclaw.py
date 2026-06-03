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
    return (args.csgclaw_base_url or os.environ.get("CSGCLAW_BASE_URL") or "http://127.0.0.1:18080").rstrip("/")


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
    bot_id = state["bot_id"]
    cli_args = [
        "bot",
        "config",
        "--channel",
        "feishu",
        "--set",
        "--bot-id",
        bot_id,
        "--app-id",
        result["app_id"],
        "--app-secret-stdin",
    ]
    candidate_admin_open_id = str(result.get("open_id") or "").strip()
    if bot_id == "u-manager" and candidate_admin_open_id:
        cli_args.extend(["--admin-open-id", candidate_admin_open_id])
    response = csgclaw_cli_json(args, cli_args, input_text=result["app_secret"] + "\n") or {}
    if bot_id == "u-manager":
        if candidate_admin_open_id:
            response["admin_open_id"] = candidate_admin_open_id
            response["admin_open_id_source"] = "manager_registration"
        else:
            response.pop("admin_open_id", None)
    elif bot_id != "u-manager":
        response.pop("admin_open_id", None)
    return response


def resolve_role(args, state: dict) -> str:
    bot_id = state["bot_id"]
    return args.role or state.get("role") or ("manager" if bot_id == "u-manager" else "worker")


def ensure_bot(args, state: dict, result: dict) -> Optional[dict]:
    if args.no_ensure_bot:
        return None
    bot_id = state["bot_id"]
    name = args.bot_name or state.get("bot_name") or bot_id.removeprefix("u-") or bot_id
    role = resolve_role(args, state)
    description = args.description or state.get("description") or f"{name} Feishu {role} agent"
    payload = {
        "id": bot_id,
        "name": name,
        "description": description,
        "role": role,
        "channel": "feishu",
    }
    return api_json(args, "POST", f"/api/v1/channels/feishu/bots", payload)


def worker_box_conflict_message(bot_id: str, name: str) -> str:
    return (
        f"worker {bot_id!r} could not be created because a residual BoxLite box named {name!r} already exists, "
        "but CSGClaw has no matching agent record. Stop here and ask the host operator to clean the stale worker "
        f"runtime, for example: ./bin/boxlite --home ~/.csgclaw/agents/{name}/boxlite rm -f {name}"
    )


def is_box_name_conflict(exc: RuntimeError, name: str) -> bool:
    message = str(exc)
    return "box with name" in message and f"'{name}' already exists" in message


def is_same_bot_name_conflict(exc: RuntimeError, bot_id: str) -> bool:
    message = str(exc)
    return (
        'bot name "' in message
        and 'already exists in channel "feishu"' in message
        and f'with id "{bot_id}"' in message
    )


def bot_exists(args, bot_id: str) -> bool:
    bots = csgclaw_cli_json(args, ["bot", "list", "--channel", "feishu"])
    if not isinstance(bots, list):
        raise RuntimeError(f"csgclaw-cli bot list returned unexpected JSON: {bots!r}")
    return any(str(bot.get("id") or "").strip() == bot_id for bot in bots if isinstance(bot, dict))


def maybe_recreate(args, state: dict, worker_existed_before_ensure: Optional[bool] = None) -> Optional[dict]:
    mode = args.recreate
    bot_id = state["bot_id"]
    role = resolve_role(args, state)
    if mode == "none":
        return None
    if role == "manager":
        if mode == "worker":
            return {"skipped": True, "reason": "worker recreate requested for manager bot"}
        return manager_recreate_action_card(bot_id)
    if mode == "manager":
        return {"skipped": True, "reason": "manager recreate requested for a worker bot"}
    # Feishu credentials are materialized into runtime env/files only during provision/start.
    return api_json(args, "POST", f"/api/v1/agents/{path_id(bot_id)}/recreate", None)


def public_result(data: dict) -> dict:
    clean = dict(data)
    for key in ("app_secret", "client_secret", "access_token", "tenant_access_token"):
        if key in clean:
            clean[key] = "present"
    return clean


def manager_recreate_action_card(bot_id: str) -> dict:
    return {
        "type": ACTION_CARD_TYPE,
        "status": "manager_recreate_pending",
        "bot_id": bot_id,
        "title": "Manager Feishu 配置已完成",
        "subtitle": bot_id,
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
