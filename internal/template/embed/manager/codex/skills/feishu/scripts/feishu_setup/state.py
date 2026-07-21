"""Registration state file handling."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Iterable

from .config import (
    CACHE_STATE_DIR_NAME,
    LEGACY_CACHE_STATE_DIR_NAME,
    STATE_DIR_ENV,
    STATE_DIR_NAME,
)


def _safe_registration_id(registration_id: str) -> str:
    return "".join(ch for ch in registration_id if ch.isalnum() or ch in "-_")


def _cache_dir(name: str) -> Path:
    return Path("~/.cache").expanduser() / name


def _dedupe(paths: Iterable[Path]) -> list[Path]:
    unique = []
    seen = set()
    for path in paths:
        key = str(path)
        if key in seen:
            continue
        seen.add(key)
        unique.append(path)
    return unique


def default_state_dir() -> Path:
    override = os.environ.get(STATE_DIR_ENV)
    if override:
        return Path(override).expanduser()
    codex_home = os.environ.get("CODEX_HOME")
    if codex_home:
        return Path(codex_home).expanduser() / STATE_DIR_NAME
    return _cache_dir(CACHE_STATE_DIR_NAME)


def state_dir(args) -> Path:
    return Path(args.state_dir).expanduser() if args.state_dir else default_state_dir()


def state_path(args, registration_id: str) -> Path:
    return state_dir(args) / f"{_safe_registration_id(registration_id)}.json"


def state_paths(args, registration_id: str) -> Iterable[Path]:
    safe_name = f"{_safe_registration_id(registration_id)}.json"
    if args.state_dir or os.environ.get(STATE_DIR_ENV):
        yield state_path(args, registration_id)
        return

    for directory in _dedupe(
        [
            default_state_dir(),
            _cache_dir(CACHE_STATE_DIR_NAME),
            _cache_dir(LEGACY_CACHE_STATE_DIR_NAME),
        ]
    ):
        yield directory / safe_name


def save_state(args, state: dict) -> None:
    directory = state_dir(args)
    directory.mkdir(parents=True, exist_ok=True)
    os.chmod(directory, 0o700)
    path = state_path(args, state["registration_id"])
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(state, ensure_ascii=False, indent=2), encoding="utf-8")
    os.chmod(tmp, 0o600)
    tmp.replace(path)


def load_state(args) -> dict:
    if not args.registration_id:
        raise SystemExit("--registration-id is required")
    checked = []
    for path in state_paths(args, args.registration_id):
        checked.append(path)
        if path.exists():
            return json.loads(path.read_text(encoding="utf-8"))
    joined = ", ".join(str(path) for path in checked)
    raise SystemExit(f"registration state not found: {joined}")


def delete_state(args, registration_id: str) -> None:
    for path in state_paths(args, registration_id):
        try:
            path.unlink()
        except FileNotFoundError:
            pass
