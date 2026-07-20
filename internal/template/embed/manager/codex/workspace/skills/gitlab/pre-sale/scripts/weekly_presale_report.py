r"""Weekly pre-sale customer follow-up report from GitLab issues.

Lists all **opened** issues in the pre-sale customers project, maps each issue
title to a customer, fetches notes in the reporting window, and flags issues
with no tracking notes in that window.

Default reporting window (Asia/Shanghai): from **last Tuesday 00:00** up to
**this Tuesday 00:00** (exclusive), implemented as half-open:
``[last Tuesday 00:00, this Tuesday 00:00)``.

Usage:
  python3 weekly_presale_report.py
  python3 weekly_presale_report.py --report-at "2026-04-21T09:30:00+08:00" --format both
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import sys
from datetime import datetime, time, timedelta
from pathlib import Path
from urllib.parse import quote
from zoneinfo import ZoneInfo

import httpx


SHARED_SCRIPTS_DIR = Path(__file__).resolve().parents[2] / "scripts"
if str(SHARED_SCRIPTS_DIR) not in sys.path:
    sys.path.insert(0, str(SHARED_SCRIPTS_DIR))

from shared_gitlab_utils import load_credentials  # noqa: E402


DEFAULT_PROJECT = "product/pre-sale/customers"
DEFAULT_TZ = "Asia/Shanghai"


def _project_ref(project_path: str) -> str:
    return quote(project_path.strip(), safe="")


def _parse_report_at(value: str | None, tz: ZoneInfo) -> datetime:
    if not value:
        return datetime.now(tz)
    text = value.strip()
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    dt = datetime.fromisoformat(text)
    if dt.tzinfo is None:
        return dt.replace(tzinfo=tz)
    return dt


def tuesday_midnight_of_this_week(t: datetime, tz: ZoneInfo) -> datetime:
    """Tuesday 00:00 of the current Monday-based week in ``tz``."""
    t_local = t.astimezone(tz)
    d = t_local.date()
    wd = d.weekday()
    monday = d - timedelta(days=wd)
    tuesday = monday + timedelta(days=1)
    return datetime.combine(tuesday, time.min, tzinfo=tz)


def reporting_window(report_at: datetime, tz: ZoneInfo) -> tuple[datetime, datetime]:
    """Return ``(start, end)`` as ``[last_tue_00, this_tue_00)``."""
    end = tuesday_midnight_of_this_week(report_at, tz)
    start = end - timedelta(days=7)
    return start, end


def _parse_gitlab_datetime(value: str) -> datetime:
    return datetime.fromisoformat(value)


def _in_window(created_at: str, start: datetime, end: datetime, tz: ZoneInfo) -> bool:
    dt = _parse_gitlab_datetime(created_at).astimezone(tz)
    return start <= dt < end


def _format_ts_local(value: str, tz: ZoneInfo) -> str:
    """Format ISO timestamp string to local readable datetime."""
    dt = _parse_gitlab_datetime(value).astimezone(tz)
    return dt.strftime("%Y-%m-%d %H:%M:%S %z")


def _format_date_cn(dt: datetime) -> str:
    """Format date as Chinese style without zero padding."""
    return f"{dt.year}年{dt.month}月{dt.day}日"


def _list_open_issues(
    *,
    gitlab_url: str,
    token: str,
    project_path: str,
    per_page: int,
    max_pages: int,
) -> list[dict]:
    ref = _project_ref(project_path)
    url = f"{gitlab_url}/api/v4/projects/{ref}/issues"
    headers = {"PRIVATE-TOKEN": token}
    items: list[dict] = []
    with httpx.Client(timeout=60.0) as client:
        for page in range(1, max_pages + 1):
            resp = client.get(
                url,
                headers=headers,
                params={
                    "state": "opened",
                    "order_by": "updated_at",
                    "sort": "desc",
                    "page": page,
                    "per_page": per_page,
                },
            )
            resp.raise_for_status()
            batch = resp.json()
            if not batch:
                break
            items.extend(batch)
            if len(batch) < per_page:
                break
    return items


def _list_issue_notes(
    *,
    gitlab_url: str,
    token: str,
    project_path: str,
    issue_iid: int,
    per_page: int,
    max_pages: int,
) -> list[dict]:
    ref = _project_ref(project_path)
    url = f"{gitlab_url}/api/v4/projects/{ref}/issues/{issue_iid}/notes"
    headers = {"PRIVATE-TOKEN": token}
    notes: list[dict] = []
    with httpx.Client(timeout=60.0) as client:
        for page in range(1, max_pages + 1):
            resp = client.get(
                url,
                headers=headers,
                params={"sort": "asc", "page": page, "per_page": per_page},
            )
            resp.raise_for_status()
            batch = resp.json()
            if not batch:
                break
            notes.extend(batch)
            if len(batch) < per_page:
                break
    return notes


def _fetch_issue_notes_safe(
    *,
    gitlab_url: str,
    token: str,
    project_path: str,
    issue_iid: int,
    per_page: int,
    max_pages: int,
) -> tuple[int, list[dict], str]:
    """Fetch one issue's notes safely for concurrent execution."""
    try:
        notes = _list_issue_notes(
            gitlab_url=gitlab_url,
            token=token,
            project_path=project_path,
            issue_iid=issue_iid,
            per_page=per_page,
            max_pages=max_pages,
        )
        return issue_iid, notes, ""
    except httpx.HTTPStatusError as e:
        return issue_iid, [], f"notes API {e.response.status_code}"


def _slim_note(n: dict) -> dict:
    author = n.get("author") or {}
    return {
        "id": n.get("id"),
        "body": n.get("body"),
        "created_at": n.get("created_at"),
        "author_username": author.get("username"),
        "author_name": author.get("name"),
        "system": n.get("system", False),
    }


def _markdown_table(rows: list[dict]) -> str:
    header = [
        "| 客户名称 | 跟进状态 | 更新时间 | Issue IID | 周期内跟进评论数 |",
        "|---|---|---|---:|---:|",
    ]
    body = [
        "| {title} | {status} | {updated} | {iid_link} | {cnt} |".format(
            title=(r.get("customer_title") or "").replace("|", "\\|"),
            status="已跟进" if r.get("has_tracking_in_window") else "本周未跟进",
            updated=(r.get("latest_note_time") or "-").replace("|", "\\|"),
            iid_link=(
                f"[{r.get('iid')}]({r.get('web_url')})"
                if r.get("web_url")
                else str(r.get("iid"))
            ),
            cnt=r.get("tracking_note_count_in_window", 0),
        )
        for r in rows
    ]
    return "\n".join([*header, *body])


def main() -> None:
    parser = argparse.ArgumentParser(description="Pre-sale weekly GitLab issue / notes report")
    parser.add_argument(
        "--project-path",
        default=DEFAULT_PROJECT,
        help=f"GitLab project path (default: {DEFAULT_PROJECT})",
    )
    parser.add_argument(
        "--timezone",
        default=DEFAULT_TZ,
        help=f"IANA timezone for window boundaries (default: {DEFAULT_TZ})",
    )
    parser.add_argument(
        "--report-at",
        default=None,
        help="Anchor time for the window (ISO-8601). Naive values use --timezone. Default: now.",
    )
    parser.add_argument(
        "--per-page",
        type=int,
        default=100,
        help="GitLab API pagination size",
    )
    parser.add_argument(
        "--max-pages",
        type=int,
        default=50,
        help="Max API pages per list (issues and notes)",
    )
    parser.add_argument(
        "--max-workers",
        type=int,
        default=8,
        help="Max concurrent workers for notes fetching",
    )
    parser.add_argument(
        "--format",
        choices=("json", "markdown", "both"),
        default="both",
        help="stdout format",
    )
    args = parser.parse_args()

    tz = ZoneInfo(args.timezone)
    report_at = _parse_report_at(args.report_at, tz)
    start, end = reporting_window(report_at, tz)

    credentials = load_credentials()
    gitlab_url = credentials["gitlab_url"]
    token = credentials["access_token"]

    try:
        issues = _list_open_issues(
            gitlab_url=gitlab_url,
            token=token,
            project_path=args.project_path,
            per_page=args.per_page,
            max_pages=args.max_pages,
        )
    except httpx.HTTPStatusError as e:
        print(f"Error: GitLab API {e.response.status_code}", file=sys.stderr)
        sys.exit(2)

    notes_by_iid: dict[int, list[dict]] = {}
    remark_by_iid: dict[int, str] = {}
    issues_to_fetch: list[int] = []

    for issue in issues:
        iid = issue.get("iid")
        issue_updated_at = (issue.get("updated_at") or "").strip()
        if iid is None:
            continue

        # Performance optimization:
        # if issue.updated_at is earlier than window start, this issue cannot have
        # new notes inside the window, so we can skip notes API calls.
        should_fetch_notes = False
        if issue_updated_at:
            try:
                issue_updated_dt = _parse_gitlab_datetime(issue_updated_at).astimezone(tz)
                should_fetch_notes = issue_updated_dt >= start
            except ValueError:
                should_fetch_notes = True
        else:
            should_fetch_notes = True

        if should_fetch_notes:
            issues_to_fetch.append(int(iid))
        else:
            notes_by_iid[int(iid)] = []
            remark_by_iid[int(iid)] = "issue.updated_at 早于窗口起点，跳过 notes 拉取"

    if issues_to_fetch:
        max_workers = max(1, min(args.max_workers, len(issues_to_fetch)))
        with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
            future_to_iid = {
                executor.submit(
                    _fetch_issue_notes_safe,
                    gitlab_url=gitlab_url,
                    token=token,
                    project_path=args.project_path,
                    issue_iid=iid,
                    per_page=args.per_page,
                    max_pages=args.max_pages,
                ): iid
                for iid in issues_to_fetch
            }
            for future in concurrent.futures.as_completed(future_to_iid):
                iid, notes, err = future.result()
                notes_by_iid[iid] = notes
                if err:
                    remark_by_iid[iid] = err

    rows_out: list[dict] = []
    for issue in issues:
        iid = issue.get("iid")
        if iid is None:
            continue
        iid_int = int(iid)
        title = issue.get("title") or ""
        web_url = issue.get("web_url") or ""
        issue_updated_at = (issue.get("updated_at") or "").strip()
        issue_updated_local = _format_ts_local(issue_updated_at, tz) if issue_updated_at else ""
        remark = remark_by_iid.get(iid_int, "")
        raw_notes = notes_by_iid.get(iid_int, [])

        all_non_system_notes: list[dict] = []
        matched: list[dict] = []
        for n in raw_notes:
            if n.get("system"):
                continue
            slim_note = _slim_note(n)
            all_non_system_notes.append(slim_note)
            if not _in_window(n.get("created_at") or "", start, end, tz):
                continue
            matched.append(slim_note)

        has_tracking = len(matched) > 0
        latest_note_time = ""
        if all_non_system_notes:
            latest_note = max(all_non_system_notes, key=lambda x: _parse_gitlab_datetime(x["created_at"]))
            latest_note_time = _format_ts_local(latest_note["created_at"], tz)
        elif issue_updated_local:
            # Fallback when notes are skipped or issue has no notes.
            latest_note_time = issue_updated_local

        latest_note_time_in_window = ""
        if matched:
            latest_note = max(matched, key=lambda x: _parse_gitlab_datetime(x["created_at"]))
            latest_note_time_in_window = _format_ts_local(latest_note["created_at"], tz)

        rows_out.append(
            {
                "iid": iid,
                "customer_title": title,
                "web_url": web_url,
                "issue_updated_at": issue_updated_at,
                "has_tracking_in_window": has_tracking,
                "tracking_note_count_in_window": len(matched),
                "tracking_notes_in_window": matched,
                "latest_note_time": latest_note_time,
                "latest_note_time_in_window": latest_note_time_in_window,
                "remark": remark,
            },
        )

    no_tracking = sum(1 for r in rows_out if not r["has_tracking_in_window"])
    rows_out.sort(
        key=lambda r: (
            0 if r["has_tracking_in_window"] else 1,
            -(r["tracking_note_count_in_window"] or 0),
            r["customer_title"] or "",
        ),
    )

    payload = {
        "project_path": args.project_path,
        "timezone": args.timezone,
        "report_at": report_at.astimezone(tz).isoformat(),
        "period": {
            "start_date": _format_date_cn(start),
            "end_date": _format_date_cn(end - timedelta(days=1)),
            "display": f"{_format_date_cn(start)}-{_format_date_cn(end - timedelta(days=1))}",
        },
        "window": {
            "start": start.isoformat(),
            "end": end.isoformat(),
            "description": "含起点、不含终点：[start, end)；对应「上周二 00:00」至「本周二 00:00」",
        },
        "open_issues_count": len(rows_out),
        "no_tracking_in_window_count": no_tracking,
        "issues": rows_out,
    }

    if args.format in ("json", "both"):
        print(json.dumps(payload, indent=2, ensure_ascii=False))
    if args.format in ("markdown", "both"):
        if args.format == "both":
            print("\n---\n")
        print(_markdown_table(rows_out))


if __name__ == "__main__":
    try:
        main()
    except (FileNotFoundError, ValueError) as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        sys.exit(130)
