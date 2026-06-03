"""Feishu/Lark accounts registration API helpers."""

from __future__ import annotations

import json
import time
from typing import Dict, Optional
from urllib.error import HTTPError, URLError
from urllib.parse import parse_qsl, urlencode, urlparse, urlunparse
from urllib.request import Request, urlopen

from .config import (
    DEFAULT_EXPIRE_SECONDS,
    ONBOARD_ACCOUNTS_URLS,
    REGISTRATION_PATH,
    REQUEST_TIMEOUT,
)


def accounts_base_url(domain: str) -> str:
    return ONBOARD_ACCOUNTS_URLS.get(domain, ONBOARD_ACCOUNTS_URLS["feishu"])


def validate_bot_id(bot_id: str) -> str:
    bot_id = (bot_id or "").strip()
    if not bot_id:
        raise RuntimeError("--bot-id is required")
    for ch in bot_id:
        if not (ch.isalnum() or ch in "-_"):
            raise RuntimeError(f"invalid bot id {bot_id!r}: only letters, digits, '-' and '_' are allowed")
    return bot_id


def append_launcher_params(url: str, source: str = "csgclaw") -> str:
    parsed = urlparse(url)
    query = dict(parse_qsl(parsed.query, keep_blank_values=True))
    query.setdefault("from", source)
    query.setdefault("tp", source)
    return urlunparse(parsed._replace(query=urlencode(query)))


def post_form(url: str, body: Dict[str, str]) -> dict:
    data = urlencode(body).encode("utf-8")
    req = Request(url, data=data, headers={"Content-Type": "application/x-www-form-urlencoded"})
    try:
        with urlopen(req, timeout=REQUEST_TIMEOUT) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except HTTPError as exc:
        body_bytes = exc.read()
        if body_bytes:
            try:
                return json.loads(body_bytes.decode("utf-8"))
            except (ValueError, json.JSONDecodeError):
                raise exc from None
        raise


def post_registration(domain: str, body: Dict[str, str]) -> dict:
    return post_form(f"{accounts_base_url(domain)}{REGISTRATION_PATH}", body)


def init_registration(domain: str) -> None:
    res = post_registration(domain, {"action": "init"})
    methods = res.get("supported_auth_methods") or []
    if "client_secret" not in methods:
        raise RuntimeError(f"Feishu/Lark registration does not support client_secret auth; supported={methods}")


def begin_registration(domain: str) -> dict:
    res = post_registration(
        domain,
        {
            "action": "begin",
            "archetype": "PersonalAgent",
            "auth_method": "client_secret",
            "request_user_info": "open_id",
        },
    )
    device_code = res.get("device_code")
    if not device_code:
        raise RuntimeError("registration begin did not return device_code")
    qr_url = append_launcher_params(res.get("verification_uri_complete", ""), "csgclaw")
    if not qr_url:
        raise RuntimeError("registration begin did not return verification_uri_complete")
    return {
        "device_code": device_code,
        "qr_url": qr_url,
        "user_code": res.get("user_code", ""),
        "interval": int(res.get("interval") or 5),
        "expire_in": int(res.get("expire_in") or DEFAULT_EXPIRE_SECONDS),
    }


def poll_registration_once(domain: str, device_code: str) -> dict:
    return post_registration(
        domain,
        {
            "action": "poll",
            "device_code": device_code,
            "tp": "ob_app",
        },
    )


def render_ascii_qr(url: str) -> bool:
    try:
        import qrcode  # type: ignore
    except Exception:
        return False
    try:
        qr = qrcode.QRCode()
        qr.add_data(url)
        qr.make(fit=True)
        qr.print_ascii(invert=True)
        return True
    except Exception:
        return False


def extract_success(state: dict, res: dict) -> Optional[dict]:
    user_info = res.get("user_info") or {}
    domain = state.get("domain", "feishu")
    if user_info.get("tenant_brand") == "lark":
        domain = "lark"
    if res.get("client_id") and res.get("client_secret"):
        return {
            "app_id": res["client_id"],
            "app_secret": res["client_secret"],
            "domain": domain,
            "open_id": user_info.get("open_id"),
        }
    return None


def poll_until_success(args, state: dict, wait: bool) -> Optional[dict]:
    deadline = min(int(state.get("expires_at", 0)) or (int(time.time()) + args.timeout), int(time.time()) + args.timeout)
    interval = max(1, int(state.get("interval") or 5))
    domain = state.get("domain", "feishu")
    while True:
        try:
            res = poll_registration_once(domain, state["device_code"])
        except (URLError, OSError, json.JSONDecodeError) as exc:
            if not wait:
                raise RuntimeError(f"poll failed: {exc}") from exc
            res = {"error": "temporary_network_error"}
        success = extract_success(state, res)
        if success:
            return success
        error = res.get("error")
        if error in ("access_denied", "expired_token"):
            raise RuntimeError(f"registration failed: {error}")
        if not wait:
            return None
        if time.time() >= deadline:
            raise RuntimeError("registration timed out before user confirmation")
        time.sleep(interval)
