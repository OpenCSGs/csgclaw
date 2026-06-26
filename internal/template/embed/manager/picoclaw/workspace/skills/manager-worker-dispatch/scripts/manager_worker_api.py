#!/usr/bin/env python
"""Simple CLI for task tracking in CSGClaw."""

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
TRACKING_STATE_ROOT = os.path.join(tempfile.gettempdir(), "manager-worker-dispatch")
FEISHU_DISPATCH_DELAY_SECONDS = 5.0


def current_timestamp() -> str:
    return datetime.now(timezone.utc).isoformat()


def build_url(base_url: str, path: str) -> str:
    return f"{base_url.rstrip('/')}/{path.lstrip('/')}"


def channel_resource_path(channel: str, resource: str) -> str:
    normalized = channel.strip().lower()
    if normalized in ("", "csgclaw"):
        return f"/api/v1/{resource.lstrip('/')}"
    if normalized == "feishu":
        return f"/api/v1/channels/feishu/{resource.lstrip('/')}"
    raise SystemExit(f'Unsupported channel "{channel}"')


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


def sanitize_log_value(value: Any) -> Any:
    if value is None or isinstance(value, (bool, int, float, str)):
        return value
    if isinstance(value, Path):
        return str(value)
    if isinstance(value, datetime):
        return value.isoformat()
    if isinstance(value, dict):
        return {str(key): sanitize_log_value(item) for key, item in value.items()}
    if isinstance(value, (list, tuple)):
        return [sanitize_log_value(item) for item in value]
    return str(value)


def emit_log(log_path: Path, event: str, **fields: Any) -> None:
    payload = {"ts": current_timestamp(), "event": event}
    payload.update({key: sanitize_log_value(value) for key, value in fields.items()})
    line = json.dumps(payload, ensure_ascii=False)
    with log_path.open("a", encoding="utf-8") as handle:
        handle.write(line)
        handle.write("\n")
    if sys.stdout.isatty():
        print(line, flush=True)


def summarize_tasks_for_debug(tasks: list[dict[str, Any]]) -> list[dict[str, Any]]:
    summary: list[dict[str, Any]] = []
    for task in tasks:
        summary.append(
            {
                "id": task.get("id"),
                "assignee": task.get("assignee"),
                "passes": get_task_passes(task),
                "description": str(task.get("description") or "").strip(),
            }
        )
    return summary


def summarize_messages_for_debug(messages: list[dict[str, Any]], limit: int = 5) -> list[dict[str, Any]]:
    summary: list[dict[str, Any]] = []
    for message in messages[-limit:]:
        summary.append(
            {
                "id": message.get("id"),
                "sender_id": message.get("sender_id"),
                "created_at": message.get("created_at"),
                "content": str(message.get("content") or "").strip(),
            }
        )
    return summary


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


def normalize_name(value: Any) -> str:
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
    participant_id: str,
    dispatch_text: str,
) -> dict[str, Any] | None:
    expected_sender_id = str(participant_id).strip()
    expected_content = dispatch_text.strip()
    for message in messages:
        sender_id = str(message.get("sender_id") or "").strip()
        content = str(message.get("content") or "").strip()
        if sender_id != expected_sender_id:
            continue
        if content != expected_content:
            parts = content.split(" ", 1)
            if len(parts) != 2 or not parts[0].startswith("@") or parts[1].strip() != expected_content:
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


def get_room_member_index(bootstrap: dict[str, Any], room_id: str) -> dict[str, dict[str, Any]]:
    if not isinstance(bootstrap, dict):
        raise TrackingError("Unexpected bootstrap response while resolving room members")

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

    members = room.get("members")
    if not isinstance(members, list):
        raise TrackingError(f'Room "{room_id}" is missing member data in IM bootstrap response')

    member_ids = {str(member_id).strip() for member_id in members if str(member_id).strip()}
    index: dict[str, dict[str, Any]] = {}
    for user in users:
        if not isinstance(user, dict):
            continue
        user_id = str(user.get("id") or "").strip()
        name = normalize_name(user.get("name"))
        if user_id in member_ids and name:
            index[name] = user
    return index


def resolve_room_assignee(bootstrap: dict[str, Any], room_id: str, assignee: Any) -> dict[str, Any]:
    name = normalize_name(assignee)
    if not name:
        raise TrackingError(f'Room "{room_id}" contains a task with an empty assignee name')

    members_by_name = get_room_member_index(bootstrap, room_id)
    user = members_by_name.get(name)
    if user is None:
        raise TrackingError(
            f'Task assignee "{assignee}" is not a known member name in room "{room_id}"'
        )
    return user


def resolve_simple_dispatch_mention_id(task: dict[str, Any]) -> str:
    explicit = str(task.get("mention_id") or task.get("assignee_id") or "").strip()
    if explicit:
        return explicit

    assignee = str(task.get("assignee") or "").strip()
    if not assignee:
        raise SystemExit("Selected task is missing 'assignee'")
    if assignee.startswith("u-") or assignee.startswith("ou_"):
        return assignee
    return f"u-{normalize_name(assignee)}"


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
    participant_id: str,
    room_id: str,
    todo_path: str,
    retry_in_seconds: float,
) -> dict[str, Any]:
    pending_index = get_pending_task_index(tasks)
    if pending_index is None:
        return {"kind": "complete"}

    pending_task = tasks[pending_index]
    pending_task_id = pending_task["id"]
    pending_dispatch_text = build_tracking_message(pending_task, todo_path)
    pending_dispatch_message = find_task_dispatch_message(messages, participant_id, pending_dispatch_text)
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
        pending_assignee_user = resolve_room_assignee(bootstrap, room_id, pending_task.get("assignee"))
        return {
            "kind": "dispatch",
            "task": pending_task,
            "text": pending_dispatch_text,
            "mention_id": str(pending_assignee_user.get("id") or "").strip(),
        }

    completed_task = tasks[pending_index - 1]
    completed_task_id = completed_task["id"]
    completed_dispatch_text = build_tracking_message(completed_task, todo_path)
    completed_dispatch_message = find_task_dispatch_message(messages, participant_id, completed_dispatch_text)
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

    pending_assignee_user = resolve_room_assignee(bootstrap, room_id, pending_task.get("assignee"))
    return {
        "kind": "dispatch",
        "task": pending_task,
        "text": pending_dispatch_text,
        "mention_id": str(pending_assignee_user.get("id") or "").strip(),
    }


def decide_simple_tracking_action(
    tasks: list[dict[str, Any]],
    *,
    room_id: str,
    todo_path: str,
    retry_in_seconds: float,
    previous_pending_index: int | None,
) -> dict[str, Any]:
    pending_index = get_pending_task_index(tasks)
    if pending_index is None:
        return {"kind": "complete"}

    pending_task = tasks[pending_index]
    pending_task_id = pending_task["id"]
    pending_dispatch_text = build_tracking_message(pending_task, todo_path)
    if previous_pending_index == pending_index:
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
            ),
        }

    return {
        "kind": "dispatch",
        "task": pending_task,
        "text": pending_dispatch_text,
        "mention_id": resolve_simple_dispatch_mention_id(pending_task),
        "delay_seconds": 0.0 if pending_index == 0 else FEISHU_DISPATCH_DELAY_SECONDS,
        "pending_index": pending_index,
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
        if method == "GET" and path == "/api/v1/bootstrap":
            return {"current_user_id": "admin", "users": [], "rooms": []}

        if method == "GET" and (
            path.startswith("/api/v1/messages?") or path.startswith("/api/v1/channels/feishu/messages?")
        ):
            return []

        result: dict[str, Any] = {
            "dry_run": True,
            "method": method,
            "url": url,
            "payload": payload,
        }

        if (
            method == "POST"
            and path.startswith("/api/v1/channels/csgclaw/participants/")
            and path.endswith("/messages")
        ):
            result["message_id"] = "dry-run-message-id"
            return result

        return result

    def send_participant_message(self, channel: str, room_id: str, participant_id: str, mention_participant_id: str, content: str) -> Any:
        command = [
            "csgclaw-cli",
            "--endpoint",
            self.base_url,
            "--output",
            "json",
        ]
        if self.token:
            command.extend(["--token", self.token])
        command.extend(
            [
                "message",
                "create",
                "--channel",
                channel,
                "--room-id",
                room_id,
                "--sender-id",
                participant_id,
                "--mention-id",
                mention_participant_id,
                "--content",
                content,
            ]
        )

        if self.dry_run:
            return {"dry_run": True, "command": command}

        try:
            completed = subprocess.run(command, check=True, capture_output=True, text=True)
        except FileNotFoundError as exc:
            raise SystemExit("csgclaw-cli was not found in PATH") from exc
        except subprocess.CalledProcessError as exc:
            detail = (exc.stderr or exc.stdout or "").strip()
            raise SystemExit(f"csgclaw-cli message create failed: {detail}") from exc

        output = completed.stdout.strip()
        if not output:
            return {}
        try:
            return json.loads(output)
        except json.JSONDecodeError as exc:
            raise SystemExit(f"Invalid JSON response from csgclaw-cli message create: {output}") from exc

    def get_bootstrap(self) -> dict[str, Any]:
        data = self.request_json("GET", "/api/v1/bootstrap")
        if isinstance(data, dict):
            return data
        raise SystemExit(f"Unexpected bootstrap response: {json.dumps(data, ensure_ascii=False)}")

    def list_messages(self, channel: str, room_id: str) -> list[dict[str, Any]]:
        query = parse.urlencode({"room_id": room_id})
        data = self.request_json("GET", f"{channel_resource_path(channel, 'messages')}?{query}")
        if isinstance(data, list):
            return [item for item in data if isinstance(item, dict)]
        raise SystemExit(f"Unexpected message list response: {json.dumps(data, ensure_ascii=False)}")


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


def build_tracking_message(task: dict[str, Any], todo_path: str) -> str:
    assignee = str(task.get("assignee") or "").strip()
    if not assignee:
        raise SystemExit("Selected task is missing 'assignee'")
    task_id = task.get("id")
    task_message = (
        f"请根据{todo_path}处理任务{task_id}。"
        "完成后请将对应任务的passes更新为true，并将进展备注写到progress_note字段。"
        "不要手动通知下一位执行者；tracker 会在你回复且 todo.json 更新后自动分派后续任务。"
    )
    return task_message


def load_api(args: argparse.Namespace) -> CSGClawAPI:
    base_url = (
        args.base_url
        or os.environ.get(CSGCLAW_BASE_URL_ENV)
        or os.environ.get(MANAGER_API_BASE_URL_ENV)
        or DEFAULT_BASE_URL
    )
    token = (
        args.token
        or os.environ.get(CSGCLAW_ACCESS_TOKEN_ENV)
        or os.environ.get(MANAGER_API_TOKEN_ENV)
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
            "--channel",
            args.channel,
            "--room-id",
            args.room_id,
            "--participant-id",
            args.participant_id,
            "--todo-path",
            args.todo_path,
            "--interval",
            str(args.interval),
        ]
    )
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
        "channel": args.channel,
        "room_id": args.room_id,
        "participant_id": args.participant_id,
        "interval": args.interval,
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
    _, log_path = get_tracking_state_paths(args.todo_path)
    read_error_streak = 0
    last_read_error: str | None = None
    last_wait_key: str | None = None
    last_simple_pending_index: int | None = None
    last_dispatch_signature: str | None = None
    terminated = False
    iteration = 0

    emit_log(
        log_path,
        "tracking-run-started",
        pid=os.getpid(),
        todo_path=os.path.abspath(os.path.expanduser(args.todo_path)),
        channel=args.channel,
        room_id=args.room_id,
        participant_id=args.participant_id,
        interval=args.interval,
        once=bool(args.once),
        dry_run=bool(args.dry_run),
        tracker_log_path=str(log_path),
    )

    def handle_termination(signum: int, _frame: Any) -> None:
        nonlocal terminated
        terminated = True
        emit_log(
            log_path,
            "tracking-stopping",
            signal=signum,
            todo_path=args.todo_path,
            iteration=iteration,
        )

    signal.signal(signal.SIGTERM, handle_termination)
    signal.signal(signal.SIGINT, handle_termination)

    try:
        while not terminated:
            iteration += 1
            emit_log(
                log_path,
                "tracking-loop-start",
                iteration=iteration,
                channel=args.channel,
                room_id=args.room_id,
                todo_path=args.todo_path,
                last_wait_key=last_wait_key,
                last_simple_pending_index=last_simple_pending_index,
                last_dispatch_signature=last_dispatch_signature,
            )
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
                    "todo_path": args.todo_path,
                    "error": error_message,
                    "retry_in_seconds": args.interval,
                    "streak": read_error_streak,
                }
                if error_message != last_read_error:
                    emit_log(log_path, "tracking-read-retry", iteration=iteration, **output)
                    last_read_error = error_message
                time.sleep(args.interval)
                continue

            if read_error_streak:
                emit_log(
                    log_path,
                    "tracking-read-recovered",
                    iteration=iteration,
                    todo_path=args.todo_path,
                    streak=read_error_streak,
                )
                read_error_streak = 0
                last_read_error = None

            emit_log(
                log_path,
                "tracking-tasks-loaded",
                iteration=iteration,
                task_count=len(tasks),
                tasks=summarize_tasks_for_debug(tasks),
                pending_index=get_pending_task_index(tasks),
            )

            if args.channel.strip().lower() in ("feishu", "csgclaw"):
                # Temporary fallback: keep CSGClaw on the same simplified delayed-dispatch
                # path as Feishu until room message tracking is ready to be enabled again.
                # The original room-aware CSGClaw sequencing logic remains in
                # decide_tracking_action() above for future restoration.
                emit_log(
                    log_path,
                    "tracking-decision-mode",
                    iteration=iteration,
                    mode="simple",
                    channel=args.channel,
                    previous_pending_index=last_simple_pending_index,
                )
                decision = decide_simple_tracking_action(
                    tasks,
                    room_id=args.room_id,
                    todo_path=args.todo_path,
                    retry_in_seconds=args.interval,
                    previous_pending_index=last_simple_pending_index,
                )
            else:
                messages = api.list_messages(args.channel, args.room_id)
                bootstrap = api.get_bootstrap()
                emit_log(
                    log_path,
                    "tracking-decision-mode",
                    iteration=iteration,
                    mode="room-aware",
                    channel=args.channel,
                    message_count=len(messages),
                    recent_messages=summarize_messages_for_debug(messages),
                    bootstrap_room_count=len(bootstrap.get("rooms", [])) if isinstance(bootstrap, dict) else None,
                    bootstrap_user_count=len(bootstrap.get("users", [])) if isinstance(bootstrap, dict) else None,
                )
                decision = decide_tracking_action(
                    tasks,
                    messages,
                    bootstrap,
                    participant_id=args.participant_id,
                    room_id=args.room_id,
                    todo_path=args.todo_path,
                    retry_in_seconds=args.interval,
                )

            emit_log(
                log_path,
                "tracking-decision",
                iteration=iteration,
                decision=decision,
            )

            if decision["kind"] == "complete":
                emit_log(
                    log_path,
                    "all-complete",
                    iteration=iteration,
                    todo_path=args.todo_path,
                    tasks=[task["id"] for task in tasks],
                )
                emit_log(
                    log_path,
                    "tracking-auto-stopped",
                    iteration=iteration,
                    reason="all-complete",
                    todo_path=args.todo_path,
                )
                return 0

            if decision["kind"] == "wait":
                wait_key = str(decision["wait_key"])
                if wait_key != last_wait_key:
                    emit_log(
                        log_path,
                        "tracking-wait",
                        iteration=iteration,
                        wait_key=wait_key,
                        output=decision["output"],
                    )
                    last_wait_key = wait_key
                if args.once:
                    emit_log(log_path, "tracking-once-exit", iteration=iteration, reason="wait")
                    return 0
                emit_log(
                    log_path,
                    "tracking-sleep",
                    iteration=iteration,
                    reason="wait",
                    seconds=args.interval,
                )
                time.sleep(args.interval)
                continue

            if decision["kind"] != "dispatch":
                raise SystemExit(f'Unexpected tracking decision: {decision["kind"]}')

            pending_task = decision["task"]
            text = str(decision["text"])
            mention_id = str(decision["mention_id"])
            delay_seconds = float(decision.get("delay_seconds", 0) or 0)
            dispatch_signature = json.dumps(
                {
                    "task_id": pending_task.get("id"),
                    "mention_id": mention_id,
                    "text": text,
                    "channel": args.channel.strip().lower(),
                    "room_id": args.room_id,
                },
                ensure_ascii=False,
                sort_keys=True,
            )
            emit_log(
                log_path,
                "tracking-dispatch-prepared",
                iteration=iteration,
                task_id=pending_task.get("id"),
                assignee=pending_task.get("assignee"),
                mention_id=mention_id,
                delay_seconds=delay_seconds,
                pending_index=decision.get("pending_index"),
                previous_pending_index=last_simple_pending_index,
                dispatch_signature=dispatch_signature,
                duplicate_of_last_dispatch=(dispatch_signature == last_dispatch_signature),
            )
            if delay_seconds > 0:
                emit_log(
                    log_path,
                    "waiting-before-dispatch",
                    iteration=iteration,
                    task_id=pending_task["id"],
                    assignee=pending_task["assignee"],
                    todo_path=args.todo_path,
                    wait_seconds=delay_seconds,
                )
                time.sleep(delay_seconds)
                if terminated:
                    emit_log(
                        log_path,
                        "tracking-stopped-before-dispatch",
                        iteration=iteration,
                        task_id=pending_task["id"],
                    )
                    return 0

            emit_log(
                log_path,
                "tracking-send-attempt",
                iteration=iteration,
                task_id=pending_task.get("id"),
                mention_id=mention_id,
                text=text,
            )
            result = api.send_participant_message(args.channel, args.room_id, args.participant_id, mention_id, text)
            emit_log(
                log_path,
                "dispatched",
                iteration=iteration,
                task_id=pending_task["id"],
                assignee=pending_task["assignee"],
                mention_id=mention_id,
                todo_path=args.todo_path,
                message=result,
                text=text,
            )
            if args.channel.strip().lower() in ("feishu", "csgclaw"):
                last_simple_pending_index = int(decision["pending_index"])
                emit_log(
                    log_path,
                    "tracking-simple-state-updated",
                    iteration=iteration,
                    last_simple_pending_index=last_simple_pending_index,
                )
            last_dispatch_signature = dispatch_signature
            last_wait_key = None
            if args.once:
                emit_log(log_path, "tracking-once-exit", iteration=iteration, reason="dispatch")
                return 0

            emit_log(
                log_path,
                "tracking-sleep",
                iteration=iteration,
                reason="post-dispatch",
                seconds=args.interval,
            )
            time.sleep(args.interval)
    except TrackingError as exc:
        emit_log(log_path, "tracking-error", iteration=iteration, error=str(exc))
        raise SystemExit(str(exc)) from exc
    except Exception as exc:
        emit_log(log_path, "tracking-unhandled-error", iteration=iteration, error=repr(exc))
        raise
    finally:
        emit_log(log_path, "tracking-run-finished", iteration=iteration, terminated=terminated)
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
    parser = argparse.ArgumentParser(description="Track todo.json progress and dispatch CSGClaw tasks.")
    add_common_args(parser)

    subparsers = parser.add_subparsers(dest="command", required=True)

    start_tracking = subparsers.add_parser(
        "start-tracking",
        help="Start a background process that watches todo.json and dispatches the selected task.",
    )
    add_common_args(start_tracking)
    start_tracking.add_argument("--channel", default="csgclaw", help="Channel name: csgclaw or feishu. Default: csgclaw.")
    start_tracking.add_argument("--room-id", required=True, help="Room id.")
    start_tracking.add_argument("--participant-id", default="u-manager", help="Participant id used as message sender.")
    start_tracking.add_argument("--todo-path", required=True, help="Path to todo.json.")
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
    run_tracking.add_argument("--channel", default="csgclaw", help="Channel name: csgclaw or feishu. Default: csgclaw.")
    run_tracking.add_argument("--room-id", required=True, help="Room id.")
    run_tracking.add_argument("--participant-id", default="u-manager", help="Participant id used as message sender.")
    run_tracking.add_argument("--todo-path", required=True, help="Path to todo.json.")
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
