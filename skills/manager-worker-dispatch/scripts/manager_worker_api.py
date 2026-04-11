#!/usr/bin/env python
"""Simple CLI for worker management and task dispatch in CSGClaw."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import signal
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any
from urllib import error, parse, request


DEFAULT_BASE_URL = "http://127.0.0.1:18080"
CSGCLAW_BASE_URL_ENV = "CSGCLAW_BASE_URL"
CSGCLAW_ACCESS_TOKEN_ENV = "CSGCLAW_ACCESS_TOKEN"
MANAGER_API_BASE_URL_ENV = "MANAGER_API_BASE_URL"
MANAGER_API_TOKEN_ENV = "MANAGER_API_TOKEN"
MANAGER_API_TIMEOUT_ENV = "MANAGER_API_TIMEOUT"
PICOCLAW_CONFIG_PATH = os.path.expanduser("~/.picoclaw/config.json")
TRACKING_STATE_ROOT = os.path.join(tempfile.gettempdir(), "manager-worker-dispatch")


def build_url(base_url: str, path: str) -> str:
    return f"{base_url.rstrip('/')}/{path.lstrip('/')}"


def load_local_config() -> dict[str, Any]:
    if not os.path.exists(PICOCLAW_CONFIG_PATH):
        return {}
    try:
        with open(PICOCLAW_CONFIG_PATH, "r", encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, json.JSONDecodeError):
        return {}
    return data if isinstance(data, dict) else {}


def get_local_csgclaw_settings() -> dict[str, str]:
    config = load_local_config()
    channels = config.get("channels")
    if not isinstance(channels, dict):
        return {}
    csgclaw = channels.get("csgclaw")
    if not isinstance(csgclaw, dict):
        return {}

    settings: dict[str, str] = {}
    base_url = csgclaw.get("base_url")
    access_token = csgclaw.get("access_token")
    if isinstance(base_url, str) and base_url.strip():
        settings["base_url"] = base_url.strip()
    if isinstance(access_token, str) and access_token.strip():
        settings["access_token"] = access_token.strip()
    return settings


def ensure_tracking_state_root() -> Path:
    root = Path(TRACKING_STATE_ROOT)
    root.mkdir(parents=True, exist_ok=True)
    return root


def make_task_id(task_id: Any, fallback: int) -> Any:
    if task_id is None:
        return fallback
    if isinstance(task_id, str):
        normalized_task_id = task_id.strip()
        return normalized_task_id or fallback
    return task_id


def build_todo_tracking_key(todo_path: str) -> str:
    resolved = os.path.abspath(os.path.expanduser(todo_path))
    return hashlib.sha256(resolved.encode("utf-8")).hexdigest()[:16]


def get_tracking_state_paths(todo_path: str) -> tuple[Path, Path]:
    key = build_todo_tracking_key(todo_path)
    root = ensure_tracking_state_root()
    return root / f"{key}.json", root / f"{key}.log"


def read_tracking_state(todo_path: str) -> dict[str, Any] | None:
    state_path, _ = get_tracking_state_paths(todo_path)
    if not state_path.exists():
        return None
    try:
        with state_path.open("r", encoding="utf-8") as handle:
            data = json.load(handle)
    except (OSError, json.JSONDecodeError):
        return None
    return data if isinstance(data, dict) else None


def write_tracking_state(todo_path: str, state: dict[str, Any]) -> Path:
    state_path, _ = get_tracking_state_paths(todo_path)
    with state_path.open("w", encoding="utf-8") as handle:
        json.dump(state, handle, ensure_ascii=False, indent=2)
        handle.write("\n")
    return state_path


def remove_tracking_state(todo_path: str) -> None:
    state_path, _ = get_tracking_state_paths(todo_path)
    try:
        state_path.unlink()
    except FileNotFoundError:
        return


def is_process_alive(pid: int) -> bool:
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    return True


class TrackingError(RuntimeError):
    """Raised when task tracking cannot make a safe sequencing decision."""


def normalize_handle(value: Any) -> str:
    return str(value or "").strip().lower().removeprefix("@")


def parse_created_at(value: Any) -> datetime | None:
    text = str(value or "").strip()
    if not text:
        return None
    if text.endswith("Z"):
        text = f"{text[:-1]}+00:00"
    try:
        created_at = datetime.fromisoformat(text)
    except ValueError:
        return None
    if created_at.tzinfo is None:
        created_at = created_at.replace(tzinfo=timezone.utc)
    return created_at.astimezone(timezone.utc)


def get_pending_task_index(tasks: list[dict[str, Any]]) -> int | None:
    for index, task in enumerate(tasks):
        if not get_task_passes(task):
            return index
    return None


def is_human_room_reply(message: dict[str, Any]) -> bool:
    content = str(message.get("content") or "").strip()
    if not content:
        return False
    return not content.startswith("🔧")


def find_task_dispatch_message(
    messages: list[dict[str, Any]],
    bot_id: str,
    dispatch_text: str,
) -> dict[str, Any] | None:
    expected_sender_id = str(bot_id).strip()
    expected_content = dispatch_text.strip()
    for message in messages:
        sender_id = str(message.get("sender_id") or "").strip()
        content = str(message.get("content") or "").strip()
        if sender_id != expected_sender_id or content != expected_content:
            continue
        return message
    return None


def has_assignee_reply_after(
    messages: list[dict[str, Any]],
    assignee_user_id: str,
    dispatched_at: datetime,
) -> bool:
    for message in messages:
        sender_id = str(message.get("sender_id") or "").strip()
        if sender_id != assignee_user_id:
            continue
        if not is_human_room_reply(message):
            continue
        created_at = parse_created_at(message.get("created_at"))
        if created_at is None or created_at <= dispatched_at:
            continue
        return True
    return False


def get_room_participant_index(bootstrap: dict[str, Any], room_id: str) -> dict[str, dict[str, Any]]:
    if not isinstance(bootstrap, dict):
        raise TrackingError("Unexpected bootstrap response while resolving room participants")

    rooms = bootstrap.get("rooms")
    users = bootstrap.get("users")
    if not isinstance(rooms, list) or not isinstance(users, list):
        raise TrackingError("Bootstrap response is missing rooms/users data required for task tracking")

    room: dict[str, Any] | None = None
    for candidate in rooms:
        if not isinstance(candidate, dict):
            continue
        if str(candidate.get("id") or "").strip() == room_id:
            room = candidate
            break
    if room is None:
        raise TrackingError(f'Room "{room_id}" was not found in IM bootstrap data')

    participants = room.get("participants")
    if not isinstance(participants, list):
        raise TrackingError(f'Room "{room_id}" is missing participant data in IM bootstrap response')

    participant_ids = {str(participant_id).strip() for participant_id in participants if str(participant_id).strip()}
    index: dict[str, dict[str, Any]] = {}
    for user in users:
        if not isinstance(user, dict):
            continue
        user_id = str(user.get("id") or "").strip()
        handle = normalize_handle(user.get("handle"))
        if user_id in participant_ids and handle:
            index[handle] = user
    return index


def resolve_room_assignee(bootstrap: dict[str, Any], room_id: str, assignee: Any) -> dict[str, Any]:
    handle = normalize_handle(assignee)
    if not handle:
        raise TrackingError(f'Room "{room_id}" contains a task with an empty assignee handle')

    participants_by_handle = get_room_participant_index(bootstrap, room_id)
    user = participants_by_handle.get(handle)
    if user is None:
        raise TrackingError(
            f'Task assignee "{assignee}" is not a known participant handle in room "{room_id}"'
        )
    return user


def build_wait_event(
    event: str,
    *,
    todo_path: str,
    room_id: str,
    retry_in_seconds: float,
    task_id: Any,
    assignee: Any,
    pending_task_id: Any | None = None,
    dispatched_at: Any | None = None,
) -> dict[str, Any]:
    output: dict[str, Any] = {
        "event": event,
        "todo_path": todo_path,
        "room_id": room_id,
        "task_id": task_id,
        "assignee": assignee,
        "retry_in_seconds": retry_in_seconds,
    }
    if pending_task_id is not None:
        output["pending_task_id"] = pending_task_id
    if dispatched_at is not None:
        output["dispatched_at"] = dispatched_at
    return output


def decide_tracking_action(
    tasks: list[dict[str, Any]],
    messages: list[dict[str, Any]],
    bootstrap: dict[str, Any],
    *,
    bot_id: str,
    room_id: str,
    mention: str | None,
    todo_path: str,
    retry_in_seconds: float,
) -> dict[str, Any]:
    pending_index = get_pending_task_index(tasks)
    if pending_index is None:
        return {"kind": "complete"}

    pending_task = tasks[pending_index]
    pending_task_id = pending_task["id"]
    pending_dispatch_text = build_tracking_message(pending_task, mention, todo_path)
    pending_dispatch_message = find_task_dispatch_message(messages, bot_id, pending_dispatch_text)
    if pending_dispatch_message is not None:
        return {
            "kind": "wait",
            "wait_key": f"waiting-for-task-passes:{pending_task_id}",
            "output": build_wait_event(
                "waiting-for-task-passes",
                todo_path=todo_path,
                room_id=room_id,
                retry_in_seconds=retry_in_seconds,
                task_id=pending_task_id,
                assignee=pending_task.get("assignee"),
                dispatched_at=pending_dispatch_message.get("created_at"),
            ),
        }

    if pending_index == 0:
        return {
            "kind": "dispatch",
            "task": pending_task,
            "text": pending_dispatch_text,
        }

    completed_task = tasks[pending_index - 1]
    completed_task_id = completed_task["id"]
    completed_dispatch_text = build_tracking_message(completed_task, mention, todo_path)
    completed_dispatch_message = find_task_dispatch_message(messages, bot_id, completed_dispatch_text)
    if completed_dispatch_message is None:
        raise TrackingError(
            f'Task {completed_task_id} is already marked passed, but no tracker dispatch message was found in room "{room_id}"'
        )

    completed_dispatch_at = parse_created_at(completed_dispatch_message.get("created_at"))
    if completed_dispatch_at is None:
        raise TrackingError(
            f'Task {completed_task_id} has a tracker dispatch message with an invalid created_at timestamp'
        )

    assignee_user = resolve_room_assignee(bootstrap, room_id, completed_task.get("assignee"))
    assignee_user_id = str(assignee_user.get("id") or "").strip()
    if not assignee_user_id:
        raise TrackingError(
            f'Task assignee "{completed_task.get("assignee")}" in room "{room_id}" is missing a user id'
        )

    if not has_assignee_reply_after(messages, assignee_user_id, completed_dispatch_at):
        return {
            "kind": "wait",
            "wait_key": f"waiting-for-assignee-reply:{completed_task_id}",
            "output": build_wait_event(
                "waiting-for-assignee-reply",
                todo_path=todo_path,
                room_id=room_id,
                retry_in_seconds=retry_in_seconds,
                task_id=completed_task_id,
                assignee=completed_task.get("assignee"),
                pending_task_id=pending_task_id,
                dispatched_at=completed_dispatch_message.get("created_at"),
            ),
        }

    return {
        "kind": "dispatch",
        "task": pending_task,
        "text": pending_dispatch_text,
    }


class CSGClawAPI:
    def __init__(self, base_url: str, token: str | None, timeout: float, dry_run: bool) -> None:
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self.dry_run = dry_run

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json", "Accept": "application/json"}
        if self.token:
            headers["Authorization"] = f"Bearer {self.token}"
        return headers

    def request_json(self, method: str, path: str, payload: dict[str, Any] | None = None) -> Any:
        url = build_url(self.base_url, path)
        body = None if payload is None else json.dumps(payload).encode("utf-8")

        if self.dry_run:
            return self._mock_response(method, path, url, payload)

        req = request.Request(url, data=body, method=method, headers=self._headers())
        try:
            with request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read().decode("utf-8")
        except error.HTTPError as exc:
            detail = exc.read().decode("utf-8", errors="replace")
            raise SystemExit(f"HTTP {exc.code} for {method} {url}: {detail}") from exc
        except error.URLError as exc:
            raise SystemExit(f"Request failed for {method} {url}: {exc.reason}") from exc

        if not raw.strip():
            return {}

        try:
            return json.loads(raw)
        except json.JSONDecodeError as exc:
            raise SystemExit(f"Invalid JSON response from {method} {url}: {raw}") from exc

    def _mock_response(
        self,
        method: str,
        path: str,
        url: str,
        payload: dict[str, Any] | None,
    ) -> dict[str, Any] | list[dict[str, Any]]:
        if method == "GET" and path == "/api/v1/workers":
            return []

        if method == "GET" and path == "/api/v1/im/bootstrap":
            return {"current_user_id": "u-admin", "users": [], "rooms": []}

        if method == "GET" and path.startswith("/api/v1/messages?"):
            return []

        result: dict[str, Any] = {
            "dry_run": True,
            "method": method,
            "url": url,
            "payload": payload,
        }

        if method == "POST" and path == "/api/v1/workers":
            worker_name = str((payload or {}).get("name") or "worker")
            worker_role = str((payload or {}).get("role") or "worker")
            worker_id = str((payload or {}).get("id") or f"u-{worker_name}")
            result.update(
                {
                    "id": worker_id,
                    "name": worker_name,
                    "role": "worker",
                    "description": (payload or {}).get("description", ""),
                    "status": "running",
                    "model_id": (payload or {}).get("model_id", ""),
                }
            )
            return result

        if method == "POST" and path == "/api/v1/im/agents/join":
            result["joined"] = True
            return result

        if method == "POST" and path.startswith("/api/bots/") and path.endswith("/messages/send"):
            result["message_id"] = "dry-run-message-id"
            return result

        return result

    def list_workers(self) -> list[dict[str, Any]]:
        data = self.request_json("GET", "/api/v1/workers")
        if isinstance(data, list):
            return [item for item in data if isinstance(item, dict)]
        raise SystemExit(f"Unexpected worker list response: {json.dumps(data, ensure_ascii=False)}")

    def create_worker(
        self,
        name: str,
        role: str,
        worker_id: str | None,
        description: str | None,
        model_id: str | None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {"name": name, "role": role}
        if worker_id:
            payload["id"] = worker_id
        if description:
            payload["description"] = description
        if model_id:
            payload["model_id"] = model_id
        return self.request_json("POST", "/api/v1/workers", payload)

    def join_agent_to_room(
        self,
        agent_id: str,
        room_id: str,
        inviter_id: str | None,
        locale: str | None,
    ) -> dict[str, Any]:
        payload: dict[str, Any] = {
            "agent_id": agent_id,
            "room_id": room_id,
        }
        if inviter_id:
            payload["inviter_id"] = inviter_id
        if locale:
            payload["locale"] = locale
        return self.request_json("POST", "/api/v1/im/agents/join", payload)

    def send_bot_message(self, bot_id: str, room_id: str, text: str) -> dict[str, Any]:
        return self.request_json(
            "POST",
            f"/api/bots/{bot_id}/messages/send",
            {"room_id": room_id, "text": text},
        )

    def get_bootstrap(self) -> dict[str, Any]:
        data = self.request_json("GET", "/api/v1/im/bootstrap")
        if isinstance(data, dict):
            return data
        raise SystemExit(f"Unexpected bootstrap response: {json.dumps(data, ensure_ascii=False)}")

    def list_messages(self, room_id: str) -> list[dict[str, Any]]:
        query = parse.urlencode({"room_id": room_id})
        data = self.request_json("GET", f"/api/v1/messages?{query}")
        if isinstance(data, list):
            return [item for item in data if isinstance(item, dict)]
        raise SystemExit(f"Unexpected message list response: {json.dumps(data, ensure_ascii=False)}")


def build_assignment_text(worker_name: str, task: str, mention: str | None) -> str:
    custom_mention = (mention or "").strip()
    if custom_mention:
        return f"{custom_mention} {task}".strip()
    return f"@{worker_name} {task}".strip()


def load_json_file(path: str) -> Any:
    try:
        with open(path, "r", encoding="utf-8") as handle:
            return json.load(handle)
    except FileNotFoundError as exc:
        raise SystemExit(f"todo.json not found: {path}") from exc
    except json.JSONDecodeError as exc:
        raise SystemExit(f"Invalid JSON in {path}: {exc}") from exc


def normalize_task_record(record: dict[str, Any], task_id: int) -> dict[str, Any]:
    normalized = dict(record)
    normalized["id"] = make_task_id(normalized.get("id"), task_id)
    return normalized


def load_tasks(todo_data: Any) -> list[dict[str, Any]]:
    if isinstance(todo_data, dict) and isinstance(todo_data.get("tasks"), list):
        tasks = [item for item in todo_data["tasks"] if isinstance(item, dict)]
        return [normalize_task_record(task, index + 1) for index, task in enumerate(tasks)]

    if isinstance(todo_data, dict):
        single_task = {key: value for key, value in todo_data.items() if key != "tasks"}
        return [normalize_task_record(single_task, 1)]

    raise SystemExit("todo.json must be an object or an object with a top-level 'tasks' array")


def get_task_passes(task: dict[str, Any]) -> bool:
    return bool(task.get("passes") is True)


def get_pending_task(tasks: list[dict[str, Any]]) -> dict[str, Any] | None:
    for task in tasks:
        if not get_task_passes(task):
            return task
    return None


def summarize_task(task: dict[str, Any]) -> str:
    description = str(task.get("description") or "").strip()
    if not description:
        raise SystemExit("Selected task is missing 'description'")

    category = str(task.get("category") or "").strip()
    steps = task.get("steps")
    progress_note = str(task.get("progress_note") or "").strip()
    passes = task.get("passes")

    lines = [description]
    if category:
        lines.append(f"Category: {category}")
    if isinstance(steps, list) and steps:
        lines.append("Steps:")
        for index, step in enumerate(steps, start=1):
            lines.append(f"{index}. {str(step).strip()}")
    if progress_note:
        lines.append(f"Progress note: {progress_note}")
    if isinstance(passes, bool):
        lines.append(f"Passes: {'true' if passes else 'false'}")
    lines.append("完成后请在房间内用 @manager 回复结果、阻塞或交接信息。")
    lines.append("不要手动通知下一位执行者，tracker 会在你回复且 todo.json 更新后自动继续。")
    return "\n".join(lines)


def build_tracking_message(task: dict[str, Any], mention: str | None, todo_path: str) -> str:
    assignee = str(task.get("assignee") or "").strip()
    if not assignee:
        raise SystemExit("Selected task is missing 'assignee'")
    task_id = task.get("id")
    task_message = (
        f"请根据{todo_path}处理任务{task_id}。"
        "完成后请将对应任务的passes更新为true，并将进展备注写到progress_note字段。"
        "同时请在当前房间回复 @manager 结果或阻塞说明。"
        "不要手动通知下一位执行者；tracker 会在你回复且 todo.json 更新后自动分派后续任务。"
    )
    return build_assignment_text(assignee, task_message, mention)


def load_api(args: argparse.Namespace) -> CSGClawAPI:
    local_settings = get_local_csgclaw_settings()
    base_url = (
        args.base_url
        or os.environ.get(CSGCLAW_BASE_URL_ENV)
        or os.environ.get(MANAGER_API_BASE_URL_ENV)
        or local_settings.get("base_url")
        or DEFAULT_BASE_URL
    )
    token = (
        args.token
        or os.environ.get(CSGCLAW_ACCESS_TOKEN_ENV)
        or os.environ.get(MANAGER_API_TOKEN_ENV)
        or local_settings.get("access_token")
    )
    timeout = float(os.environ.get(MANAGER_API_TIMEOUT_ENV, "30"))
    return CSGClawAPI(base_url=base_url, token=token, timeout=timeout, dry_run=args.dry_run)


def add_common_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--base-url",
        help=(
            "CSGClaw API base URL. "
            f"Falls back to {CSGCLAW_BASE_URL_ENV}, {MANAGER_API_BASE_URL_ENV}, default {DEFAULT_BASE_URL}."
        ),
    )
    parser.add_argument(
        "--token",
        help=f"Bearer token. Falls back to {CSGCLAW_ACCESS_TOKEN_ENV}, {MANAGER_API_TOKEN_ENV}.",
    )
    parser.add_argument("--dry-run", action="store_true", help="Print requests instead of sending them.")


def cmd_list_workers(args: argparse.Namespace) -> int:
    api = load_api(args)
    print(json.dumps(api.list_workers(), ensure_ascii=False, indent=2))
    return 0


def cmd_create_worker(args: argparse.Namespace) -> int:
    api = load_api(args)
    worker = api.create_worker(args.name, args.role, args.id, args.description, args.model_id)
    print(json.dumps(worker, ensure_ascii=False, indent=2))
    return 0


def cmd_join_worker(args: argparse.Namespace) -> int:
    api = load_api(args)
    result = api.join_agent_to_room(args.worker_id, args.room_id, args.inviter_id, args.locale)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


def cmd_start_tracking(args: argparse.Namespace) -> int:
    existing_state = read_tracking_state(args.todo_path)
    if existing_state:
        pid = existing_state.get("pid")
        if isinstance(pid, int) and is_process_alive(pid):
            print(json.dumps(existing_state, ensure_ascii=False, indent=2))
            return 0
        remove_tracking_state(args.todo_path)

    state_path, log_path = get_tracking_state_paths(args.todo_path)
    command = [sys.executable, os.path.abspath(__file__), "run-tracking"]
    if args.base_url:
        command.extend(["--base-url", args.base_url])
    if args.token:
        command.extend(["--token", args.token])
    if args.dry_run:
        command.append("--dry-run")
    command.extend(
        [
            "--room-id",
            args.room_id,
            "--bot-id",
            args.bot_id,
            "--todo-path",
            args.todo_path,
            "--interval",
            str(args.interval),
        ]
    )
    if args.mention:
        command.extend(["--mention", args.mention])
    if args.once:
        command.append("--once")

    with log_path.open("a", encoding="utf-8") as log_handle:
        process = subprocess.Popen(
            command,
            stdin=subprocess.DEVNULL,
            stdout=log_handle,
            stderr=log_handle,
            start_new_session=True,
        )

    state = {
        "event": "tracking-started",
        "pid": process.pid,
        "todo_path": os.path.abspath(os.path.expanduser(args.todo_path)),
        "room_id": args.room_id,
        "bot_id": args.bot_id,
        "interval": args.interval,
        "mention": args.mention or "",
        "dry_run": bool(args.dry_run),
        "once": bool(args.once),
        "log_path": str(log_path),
        "state_path": str(state_path),
    }
    write_tracking_state(args.todo_path, state)
    print(json.dumps(state, ensure_ascii=False, indent=2))
    return 0


def cmd_run_tracking(args: argparse.Namespace) -> int:
    api = load_api(args)
    read_error_streak = 0
    last_read_error: str | None = None
    last_wait_key: str | None = None
    terminated = False

    def handle_termination(signum: int, _frame: Any) -> None:
        nonlocal terminated
        terminated = True
        print(
            json.dumps(
                {"event": "tracking-stopping", "signal": signum, "todo_path": args.todo_path},
                ensure_ascii=False,
                indent=2,
            ),
            flush=True,
        )

    signal.signal(signal.SIGTERM, handle_termination)
    signal.signal(signal.SIGINT, handle_termination)

    try:
        while not terminated:
            try:
                todo_data = load_json_file(args.todo_path)
                tasks = load_tasks(todo_data)
                if not tasks:
                    raise SystemExit("todo.json does not contain any tasks")
            except SystemExit as exc:
                error_message = str(exc)
                recoverable = error_message.startswith("todo.json not found:") or error_message.startswith(
                    "Invalid JSON in "
                )
                if not recoverable:
                    raise

                read_error_streak += 1
                output = {
                    "event": "tracking-read-retry",
                    "todo_path": args.todo_path,
                    "error": error_message,
                    "retry_in_seconds": args.interval,
                    "streak": read_error_streak,
                }
                if error_message != last_read_error:
                    print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
                    last_read_error = error_message
                time.sleep(args.interval)
                continue

            if read_error_streak:
                output = {
                    "event": "tracking-read-recovered",
                    "todo_path": args.todo_path,
                    "streak": read_error_streak,
                }
                print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
                read_error_streak = 0
                last_read_error = None

            decision = decide_tracking_action(
                tasks,
                api.list_messages(args.room_id),
                api.get_bootstrap(),
                bot_id=args.bot_id,
                room_id=args.room_id,
                mention=args.mention,
                todo_path=args.todo_path,
                retry_in_seconds=args.interval,
            )

            if decision["kind"] == "complete":
                output = {
                    "event": "all-complete",
                    "todo_path": args.todo_path,
                    "tasks": [task["id"] for task in tasks],
                }
                print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
                output = {
                    "event": "tracking-auto-stopped",
                    "reason": "all-complete",
                    "todo_path": args.todo_path,
                }
                print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
                return 0

            if decision["kind"] == "wait":
                wait_key = str(decision["wait_key"])
                if wait_key != last_wait_key:
                    print(json.dumps(decision["output"], ensure_ascii=False, indent=2), flush=True)
                    last_wait_key = wait_key
                if args.once:
                    return 0
                time.sleep(args.interval)
                continue

            if decision["kind"] != "dispatch":
                raise SystemExit(f'Unexpected tracking decision: {decision["kind"]}')

            pending_task = decision["task"]
            text = str(decision["text"])
            result = api.send_bot_message(args.bot_id, args.room_id, text)
            output = {
                "event": "dispatched",
                "task_id": pending_task["id"],
                "assignee": pending_task["assignee"],
                "todo_path": args.todo_path,
                "message": result,
                "text": text,
            }
            print(json.dumps(output, ensure_ascii=False, indent=2), flush=True)
            last_wait_key = None
            if args.once:
                return 0

            time.sleep(args.interval)
    except TrackingError as exc:
        raise SystemExit(str(exc)) from exc
    finally:
        remove_tracking_state(args.todo_path)


def cmd_stop_tracking(args: argparse.Namespace) -> int:
    state = read_tracking_state(args.todo_path)
    if not state:
        print(
            json.dumps(
                {"event": "tracking-not-running", "todo_path": os.path.abspath(os.path.expanduser(args.todo_path))},
                ensure_ascii=False,
                indent=2,
            )
        )
        return 0

    pid = state.get("pid")
    if not isinstance(pid, int):
        remove_tracking_state(args.todo_path)
        raise SystemExit(f"Invalid tracking state for {args.todo_path}: missing pid")

    if not is_process_alive(pid):
        remove_tracking_state(args.todo_path)
        state["event"] = "tracking-already-stopped"
        print(json.dumps(state, ensure_ascii=False, indent=2))
        return 0

    os.kill(pid, signal.SIGTERM)
    state["event"] = "tracking-stop-requested"
    print(json.dumps(state, ensure_ascii=False, indent=2))
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Call CSGClaw worker and bot APIs.")
    add_common_args(parser)

    subparsers = parser.add_subparsers(dest="command", required=True)

    list_workers = subparsers.add_parser("list-workers", help="Fetch all workers.")
    add_common_args(list_workers)
    list_workers.set_defaults(func=cmd_list_workers)

    create_worker = subparsers.add_parser("create-worker", help="Create a worker.")
    add_common_args(create_worker)
    create_worker.add_argument("--name", required=True, help="Worker name.")
    create_worker.add_argument("--role", default="worker", help="Worker role. Default: worker.")
    create_worker.add_argument("--id", help="Optional worker id.")
    create_worker.add_argument("--description", help="Optional worker description.")
    create_worker.add_argument("--model-id", help="Optional model id.")
    create_worker.set_defaults(func=cmd_create_worker)

    join_worker = subparsers.add_parser("join-worker", help="Join a worker to a room.")
    add_common_args(join_worker)
    join_worker.add_argument("--room-id", required=True, help="Room id.")
    join_worker.add_argument("--worker-id", required=True, help="Worker agent id.")
    join_worker.add_argument("--inviter-id", default="u-manager", help="Inviter id. Default: u-manager.")
    join_worker.add_argument("--locale", help="Optional locale, for example zh-CN.")
    join_worker.set_defaults(func=cmd_join_worker)

    start_tracking = subparsers.add_parser(
        "start-tracking",
        help="Start a background process that watches todo.json and dispatches the selected task.",
    )
    add_common_args(start_tracking)
    start_tracking.add_argument("--room-id", required=True, help="Room id.")
    start_tracking.add_argument("--bot-id", default="u-manager", help="Bot id used as message sender.")
    start_tracking.add_argument("--todo-path", required=True, help="Path to todo.json.")
    start_tracking.add_argument("--mention", help="Optional custom mention prefix. Default: use @<assignee>.")
    start_tracking.add_argument(
        "--interval",
        type=float,
        default=2.0,
        help="Polling interval in seconds. Default: 2.0.",
    )
    start_tracking.add_argument(
        "--once",
        action="store_true",
        help="Run the background tracker once for the current task content, then exit.",
    )
    start_tracking.set_defaults(func=cmd_start_tracking)

    run_tracking = subparsers.add_parser("run-tracking", help=argparse.SUPPRESS)
    add_common_args(run_tracking)
    run_tracking.add_argument("--room-id", required=True, help="Room id.")
    run_tracking.add_argument("--bot-id", default="u-manager", help="Bot id used as message sender.")
    run_tracking.add_argument("--todo-path", required=True, help="Path to todo.json.")
    run_tracking.add_argument("--mention", help="Optional custom mention prefix. Default: use @<assignee>.")
    run_tracking.add_argument(
        "--interval",
        type=float,
        default=2.0,
        help="Polling interval in seconds. Default: 2.0.",
    )
    run_tracking.add_argument(
        "--once",
        action="store_true",
        help="Send once for the current task content, then exit.",
    )
    run_tracking.set_defaults(func=cmd_run_tracking)

    stop_tracking = subparsers.add_parser(
        "stop-tracking",
        help="Stop the background tracker for the specified todo.json.",
    )
    stop_tracking.add_argument("--todo-path", required=True, help="Path to todo.json.")
    stop_tracking.set_defaults(func=cmd_stop_tracking)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
