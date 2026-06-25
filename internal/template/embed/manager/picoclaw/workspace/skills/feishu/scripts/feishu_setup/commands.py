"""Command-line interface for CSGClaw Feishu registration."""

from __future__ import annotations

import argparse
import json
import sys
import time
import uuid
from typing import Any, Optional
from urllib.parse import quote

from .config import API_REQUEST_TIMEOUT, DEFAULT_EXPIRE_SECONDS, MANAGER_GROUP_SCOPES, ONBOARD_OPEN_URLS
from .csgclaw import (
    api_json,
    configure_csgclaw,
    csgclaw_cli_json,
    manager_recreate_action_card,
    path_id,
    public_result,
    resolve_role,
)
from .registration import (
    begin_registration,
    init_registration,
    poll_until_success,
    render_ascii_qr,
    validate_agent_id,
)
from .state import delete_state, load_state, save_state, state_path


def eprint(*args: Any) -> None:
    print(*args, file=sys.stderr)


def manager_group_permission_url(domain: str, app_id: str) -> str:
    base = ONBOARD_OPEN_URLS.get(domain or "feishu", ONBOARD_OPEN_URLS["feishu"])
    quoted_app_id = quote(app_id, safe="")
    quoted_scopes = quote(",".join(MANAGER_GROUP_SCOPES), safe=",:")
    return f"{base}/app/{quoted_app_id}/auth?q={quoted_scopes}&op_from=openapi&token_type=tenant"


def resolve_manager_app_id(args: argparse.Namespace, state: dict, result: dict) -> str:
    if state.get("agent_id") == "u-manager":
        return str(result.get("app_id") or "").strip()
    try:
        participants = csgclaw_cli_json(args, ["participant", "list", "--channel", "feishu"])
    except RuntimeError:
        return ""
    if not isinstance(participants, list):
        return ""
    for participant in participants:
        if not isinstance(participant, dict):
            continue
        is_manager_participant = str(participant.get("id") or "").strip() == "manager"
        is_manager_agent = str(participant.get("agent_id") or "").strip() == "u-manager"
        if not (is_manager_participant or is_manager_agent):
            continue
        config = participant.get("channel_app_config")
        if not isinstance(config, dict):
            continue
        app_id = str(config.get("app_id") or "").strip()
        if app_id:
            return app_id
    return ""


def add_manager_group_permission_info(args: argparse.Namespace, state: dict, result: dict, output: dict) -> None:
    manager_app_id = resolve_manager_app_id(args, state, result)
    domain = str(result.get("domain") or state.get("domain") or "feishu")
    output["manager_group_scopes"] = MANAGER_GROUP_SCOPES
    output["manager_group_permission_note"] = (
        "Approve these scopes on the manager Feishu app when the manager needs to "
        "create Feishu groups, inspect group members, or add worker bots to existing Feishu groups."
    )
    if manager_app_id:
        output["manager_group_permission_app_id"] = manager_app_id
        output["manager_group_permission_url"] = manager_group_permission_url(domain, manager_app_id)
    else:
        output["manager_group_permission_app_id"] = ""
        output["manager_group_permission_url"] = ""


def manager_secret_cli_args(args: argparse.Namespace) -> tuple[list[str], Optional[str]]:
    secret_file = str(args.app_secret_file or "").strip()
    secret_env = str(args.app_secret_env or "").strip()
    secret_stdin = bool(args.app_secret_stdin)
    source_count = sum(1 for enabled in (bool(secret_file), bool(secret_env), secret_stdin) if enabled)
    if source_count != 1:
        raise RuntimeError("provide exactly one of --app-secret-file, --app-secret-env, or --app-secret-stdin")
    if secret_file:
        return ["--app-secret-file", secret_file], None
    if secret_env:
        return ["--app-secret-env", secret_env], None
    return ["--app-secret-stdin"], sys.stdin.read()


def ensure_worker_agent_exists(args: argparse.Namespace, agent_id: str, role: str) -> None:
    if role != "worker":
        return
    try:
        api_json(args, "GET", f"/api/v1/agents/{path_id(agent_id)}", None)
    except RuntimeError as exc:
        if "HTTP 404" not in str(exc):
            raise
        raise RuntimeError(
            f"target worker agent {agent_id!r} does not exist yet. "
            "For a request like 'create dev worker and connect Feishu', run the agent-creator skill first "
            "to create the worker with `csgclaw-cli participant create --type agent --bind create --from-template ...`, "
            "then return to the Feishu skill and start registration for this existing agent."
        ) from None


def cmd_bind_manager(args: argparse.Namespace) -> int:
    agent_id = validate_agent_id(args.agent)
    if agent_id != "u-manager":
        raise RuntimeError("bind-manager currently supports only u-manager")
    app_id = str(args.app_id or "").strip()
    if not app_id:
        raise RuntimeError("--app-id is required")
    open_id = str(args.open_id or "").strip()
    config: dict[str, Any] = {}
    if open_id:
        admin_args = [
            "participant",
            "bind",
            "--channel",
            "feishu",
            "--feishu-kind",
            "human",
            "--admin",
            "--open-id",
            open_id,
        ]
        admin_name = str(args.name or "").strip()
        if admin_name:
            admin_args.extend(["--name", admin_name])
        config["admin_bind"] = csgclaw_cli_json(args, admin_args)
    secret_args, input_text = manager_secret_cli_args(args)
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
        app_id,
        *secret_args,
    ]
    config["bot_bind"] = csgclaw_cli_json(args, bot_bind_args, input_text=input_text)
    output = {
        "status": "configured",
        "agent_id": agent_id,
        "bot_id": agent_id,
        "role": "manager",
        "app_id": app_id,
        "app_secret": "present",
        "domain": args.domain,
        "admin_open_id": open_id,
        "config": public_result(config),
        "bot_ensured": True,
    }
    add_manager_group_permission_info(
        args,
        {"agent_id": agent_id, "domain": args.domain},
        {"app_id": app_id, "domain": args.domain, "open_id": open_id},
        output,
    )
    setup_status = output["status"]
    recreated = manager_recreate_action_card(agent_id)
    output.update(recreated)
    output["setup_status"] = setup_status
    output["recreate"] = public_result(recreated)
    print(json.dumps(output, ensure_ascii=False, indent=2))
    return 0


def cmd_start(args: argparse.Namespace) -> int:
    agent_id = validate_agent_id(args.agent)
    domain = args.domain
    role = args.role or ("manager" if agent_id == "u-manager" else "worker")
    ensure_worker_agent_exists(args, agent_id, role)
    init_registration(domain)
    begin = begin_registration(domain)
    registration_id = str(uuid.uuid4())
    now = int(time.time())
    state = {
        "registration_id": registration_id,
        "agent_id": agent_id,
        "role": role,
        "bot_name": args.bot_name or agent_id.removeprefix("u-") or agent_id,
        "description": args.description or "",
        "domain": domain,
        "device_code": begin["device_code"],
        "qr_url": begin["qr_url"],
        "user_code": begin.get("user_code", ""),
        "interval": begin["interval"],
        "expire_in": begin["expire_in"],
        "created_at": now,
        "expires_at": now + min(begin["expire_in"], args.timeout),
    }
    save_state(args, state)
    output = {
        "registration_id": registration_id,
        "agent_id": agent_id,
        "role": role,
        "qr_url": begin["qr_url"],
        "user_code": begin.get("user_code", ""),
        "interval": begin["interval"],
        "expires_in": min(begin["expire_in"], args.timeout),
        "state_path": str(state_path(args, registration_id)),
        "next": f"python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id {registration_id}",
        "next_tool_timeout_seconds": API_REQUEST_TIMEOUT,
    }
    if args.json:
        print(json.dumps(output, ensure_ascii=False, indent=2))
    else:
        print(f"Feishu registration started for {agent_id}.")
        print(f"Registration ID: {registration_id}")
        print()
        if args.qr:
            rendered = render_ascii_qr(begin["qr_url"])
            if rendered:
                print()
        print("Open this URL in Feishu/Lark and confirm bot creation:")
        print(begin["qr_url"])
        print()
        print("After the user confirms, run:")
        print(output["next"])
        print(f"Use a tool timeout of at least {API_REQUEST_TIMEOUT} seconds for finalize when creating worker boxes.")
    return 0


def cmd_poll(args: argparse.Namespace) -> int:
    state = load_state(args)
    result = poll_until_success(args, state, wait=False)
    if result:
        print(
            json.dumps(
                {
                    "status": "confirmed",
                    "agent_id": state["agent_id"],
                    "credentials": "available",
                    "next": f"python /home/picoclaw/.picoclaw/workspace/skills/feishu/scripts/feishu_register.py finalize --registration-id {state['registration_id']}",
                    "next_tool_timeout_seconds": API_REQUEST_TIMEOUT,
                },
                ensure_ascii=False,
                indent=2,
            )
        )
    else:
        print(json.dumps({"status": "pending", "agent_id": state["agent_id"]}, ensure_ascii=False, indent=2))
    return 0


def cmd_finalize(args: argparse.Namespace) -> int:
    state = load_state(args)
    result = poll_until_success(args, state, wait=True)
    if not result:
        raise RuntimeError("registration has not completed")
    configured = configure_csgclaw(args, state, result) if not args.no_configure else None
    role = resolve_role(args, state)
    worker_existed_before_ensure = None
    ensured = (configured or {}).get("bot_bind") if isinstance(configured, dict) else None
    recreated = None
    if role == "manager" and configured is not None and args.recreate != "none":
        recreated = manager_recreate_action_card(state["agent_id"])
    if not args.keep_state:
        delete_state(args, state["registration_id"])
    if configured is not None:
        admin_open_id = str((configured or {}).get("admin_open_id") or "").strip() if state["agent_id"] == "u-manager" else ""
    else:
        admin_open_id = str(result.get("open_id") or "").strip() if state["agent_id"] == "u-manager" else ""
    worker_recreate_policy = None
    if role == "worker":
        restart_status = str((ensured or {}).get("restart_status") or "") if isinstance(ensured, dict) else ""
        if restart_status:
            worker_recreate_policy = restart_status
        elif args.recreate == "none":
            worker_recreate_policy = "recreate_disabled"
        elif args.recreate == "manager":
            worker_recreate_policy = "worker_recreate_skipped_manager_mode"
        else:
            worker_recreate_policy = "not_checked"
    output = {
        "status": "configured" if configured else "credentials_received",
        "agent_id": state["agent_id"],
        "role": state.get("role"),
        "app_id": result["app_id"],
        "app_secret": "present",
        "domain": result.get("domain"),
        "admin_open_id": admin_open_id,
        "config": public_result(configured or {}),
        "bot_ensured": ensured is not None,
        "worker_existed_before_ensure": worker_existed_before_ensure,
        "worker_recreate_policy": worker_recreate_policy,
        "recreate": public_result(recreated or {}),
    }
    add_manager_group_permission_info(args, state, result, output)
    if isinstance(recreated, dict) and recreated.get("type") == "csgclaw.action_card":
        setup_status = output["status"]
        output.update(recreated)
        output["setup_status"] = setup_status
        output["recreate"] = public_result(recreated)
    print(json.dumps(output, ensure_ascii=False, indent=2))
    return 0


def cmd_status(args: argparse.Namespace) -> int:
    state = load_state(args)
    safe = {k: v for k, v in state.items() if k not in {"device_code"}}
    safe["device_code"] = "present"
    print(json.dumps(safe, ensure_ascii=False, indent=2))
    return 0


def cmd_recreate_agent(args: argparse.Namespace) -> int:
    agent_id = validate_agent_id(args.agent)
    if agent_id == "u-manager":
        output = manager_recreate_action_card(agent_id)
    else:
        result = api_json(args, "POST", f"/api/v1/agents/{path_id(agent_id)}/recreate", None)
        output = {"status": "recreate_requested", "agent_id": agent_id, "result": public_result(result or {})}
    print(json.dumps(output, ensure_ascii=False, indent=2))
    return 0


def add_common(p: argparse.ArgumentParser) -> None:
    p.add_argument("--state-dir", default="", help="State directory; default is ~/.picoclaw/workspace/.feishu or ~/.cache/csgclaw-feishu")


def add_api_common(p: argparse.ArgumentParser) -> None:
    p.add_argument("--csgclaw-base-url", default="", help="CSGClaw base URL; default $CSGCLAW_BASE_URL or http://127.0.0.1:18080")
    p.add_argument("--api-timeout", type=int, default=None, help="CSGClaw API timeout in seconds; default $CSGCLAW_API_TIMEOUT or 600")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Feishu/Lark QR registration helper for CSGClaw Feishu channel setup")
    sub = parser.add_subparsers(dest="command", required=True)

    start = sub.add_parser("start", help="Start QR registration and print URL/QR")
    add_common(start)
    add_api_common(start)
    start.add_argument("--agent", required=True, help="CSGClaw agent id, e.g. u-dev or u-manager")
    start.add_argument("--role", choices=["worker", "manager"], default="", help="Agent role; inferred from agent id when omitted")
    start.add_argument("--bot-name", default="", help="CSGClaw bot display name")
    start.add_argument("--description", default="", help="CSGClaw bot description")
    start.add_argument("--domain", choices=["feishu", "lark"], default="feishu")
    start.add_argument("--timeout", type=int, default=DEFAULT_EXPIRE_SECONDS)
    start.add_argument("--json", action="store_true", help="Print machine-readable JSON")
    start.add_argument("--qr", action="store_true", help="Try to render an ASCII QR code if qrcode is installed")
    start.set_defaults(func=cmd_start)

    poll = sub.add_parser("poll", help="Check whether the user has completed registration; does not print secrets")
    add_common(poll)
    poll.add_argument("--registration-id", required=True)
    poll.add_argument("--timeout", type=int, default=30)
    poll.set_defaults(func=cmd_poll)

    finalize = sub.add_parser("finalize", help="Wait for registration, bind Feishu participants, and optionally recreate the worker")
    add_common(finalize)
    add_api_common(finalize)
    finalize.add_argument("--registration-id", required=True)
    finalize.add_argument("--timeout", type=int, default=DEFAULT_EXPIRE_SECONDS)
    finalize.add_argument("--no-configure", action="store_true", help="Do not write CSGClaw config; for debugging only, still never prints secret")
    finalize.add_argument("--role", choices=["worker", "manager"], default="", help="Override role for ensure/recreate logic")
    finalize.add_argument("--bot-name", default="", help="Override bot name for ensure")
    finalize.add_argument("--description", default="", help="Override bot description for ensure")
    finalize.add_argument("--recreate", choices=["none", "auto", "worker", "manager"], default="auto", help="auto recreates existing workers and returns an action card for manager")
    finalize.add_argument("--keep-state", action="store_true", help="Keep registration state file after successful finalize")
    finalize.set_defaults(func=cmd_finalize)

    status = sub.add_parser("status", help="Print saved registration state without secrets")
    add_common(status)
    status.add_argument("--registration-id", required=True)
    status.set_defaults(func=cmd_status)

    recreate = sub.add_parser("recreate-agent", help="Request worker agent recreate; manager returns a browser action card")
    add_api_common(recreate)
    recreate.add_argument("--agent", required=True, help="CSGClaw agent id to recreate")
    recreate.set_defaults(func=cmd_recreate_agent)

    bind_manager = sub.add_parser("bind-manager", help="Bind manager Feishu credentials and print a browser action card")
    add_common(bind_manager)
    add_api_common(bind_manager)
    bind_manager.add_argument("--agent", default="u-manager", help="Manager agent id; only u-manager is supported")
    bind_manager.add_argument("--app-id", required=True, help="Feishu app id for the manager bot app")
    bind_manager.add_argument("--open-id", default="", help="Optional Feishu admin open_id to bind before the bot app")
    bind_manager.add_argument("--name", default="", help="Optional admin participant display name")
    bind_manager.add_argument("--domain", choices=["feishu", "lark"], default="feishu")
    bind_manager.add_argument("--app-secret-file", default="", help="Read Feishu app secret from file")
    bind_manager.add_argument("--app-secret-env", default="", help="Read Feishu app secret from environment variable")
    bind_manager.add_argument("--app-secret-stdin", action="store_true", help="Read Feishu app secret from stdin")
    bind_manager.set_defaults(func=cmd_bind_manager)
    return parser


def main(argv: Optional[list[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    try:
        return args.func(args)
    except Exception as exc:
        eprint(f"error: {exc}")
        return 1
