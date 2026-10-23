#!/usr/bin/env python3
"""
DeepSeek 搜索 / 网页抓取 / 多轮检索

基于 DeepSeek 官方 Web Search & Fetch 接口：
  - Search: POST https://api.deepseek.com/v1/tools/search
  - Fetch : POST https://api.deepseek.com/v1/tools/fetch

三个入口:
  search    单次搜索，输出打分后的结构化结果
  fetch     抓取指定 URL 的正文（Markdown）
  research  完整流程：搜索 -> 可用性判定 -> 抓重点正文 -> 不足则改写再搜

用法:
  python scripts/deepseek_research.py search "DeepSeek V4 发布"
  python scripts/deepseek_research.py fetch https://example.com/article
  python scripts/deepseek_research.py research "某主题" --top 3 --rounds 3

凭据:
  API Key 按 --api-key -> 环境变量 DEEPSEEK_API_KEY 的顺序读取。
  两者都没有时直接退出（退出码 2）并提示获取地址。
"""

import argparse
import json
import os
import random
import re
import sys
import time
from datetime import datetime
from pathlib import Path
from urllib.parse import urlparse

import requests

# -- 常量 --------------------------------------------------------------------
API_BASE = "https://api.deepseek.com"
SEARCH_URL = f"{API_BASE}/v1/tools/search"
FETCH_URL = f"{API_BASE}/v1/tools/fetch"

ENV_VAR = "DEEPSEEK_API_KEY"
KEY_FILE_ENV = "DEEPSEEK_KEY_FILE"     # 可覆盖 Key 文件路径
KEY_HELP_URL = "https://platform.deepseek.com/api_keys"


def default_key_file():
    """跨平台统一的 Key 文件位置：~/.config/deepseek/api_key"""
    override = (os.environ.get(KEY_FILE_ENV) or "").strip()
    if override:
        return Path(override).expanduser()
    return Path.home() / ".config" / "deepseek" / "api_key"

DEFAULT_COUNT = 50        # 单次搜索返回条数：请求按次计费，多要不加钱（官方上限 50）
DEFAULT_MAX_FETCH = 1     # 每轮 Fetch 上限：实测 Search 正文覆盖率 100%，仅作兜底
DEFAULT_MAX_PAGES = 20    # 每轮最多落盘的正文篇数（Search 自带正文不额外计费）
DEFAULT_ENOUGH = 5        # 收集到多少篇有效正文就停止迭代
DEFAULT_ROUNDS = 3
GOOD_SCORE = 1.5          # rerank_score 阈值，官方定义为优质结果
GOOD_AUTHORITY = 3        # authority 阈值，官方定义为极权威
MIN_CONTENT_CHARS = 200   # 正文短于此长度视为未取得有效内容
REQUEST_TIMEOUT = 60
RETRY_STATUS = {408, 429, 500, 502, 503, 504}
MAX_RETRY = 3
FETCH_INTERVAL = 0.2      # Fetch 账号级 QPS 25，串行间隔取 0.2s

BLOCK_HINTS = (
    "请开启 JavaScript",
    "enable JavaScript",
    "Just a moment",
    "Attention Required",
    "Access Denied",
    "验证码",
    "cf-browser-verification",
)

KEY_MISSING_MESSAGE = """[错误] 未找到 DeepSeek API Key。

本 Skill 不内置任何 Key，需要你自己提供：
  1. 打开 {help_url} 创建 / 复制 API Key
  2. 首选：写入 Key 文件（终端、GUI 应用、macOS / Linux / Windows 都能读到）
       python {script} set-key --api-key sk-...
     文件位置：{key_file}
  3. 或配置环境变量 {env_var}（只对真正读取该变量的进程有效，GUI 应用通常读不到）
       export {env_var}="sk-..."
  4. 或仅在本次运行临时传入
       python {script} research "查询内容" --api-key sk-...

费用提示: Search 与 Fetch 各按 5 元 / 1000 次成功请求计费；失败、无结果不计费。
"""



# -- 凭据 --------------------------------------------------------------------
def read_key_file(path):
    """读取 Key 文件，兼容 `sk-...` 单行与 `DEEPSEEK_API_KEY=sk-...` 两种写法。"""
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None, None
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" in line:
            name, _, value = line.partition("=")
            if name.strip() == ENV_VAR:
                return value.strip().strip("'\""), "too_open" if _perm_too_open(path) else None
            continue
        return line.strip("'\""), "too_open" if _perm_too_open(path) else None
    return None, None


def _perm_too_open(path):
    try:
        mode = path.stat().st_mode & 0o777
    except OSError:
        return False
    return mode & 0o077 != 0   # 组或其他用户可读即可视为过宽


def find_key(cli_key=None):
    """查找 Key，返回 (key, 来源描述)；找不到返回 (None, None)。"""
    key = (cli_key or "").strip()
    if key:
        return key, "命令行参数"
    key = (os.environ.get(ENV_VAR) or "").strip()
    if key:
        return key, f"环境变量 {ENV_VAR}"
    path = default_key_file()
    if path.is_file():
        key, warn = read_key_file(path)
        if warn:
            sys.stderr.write(f"[提示] {path} 权限过宽，建议执行: chmod 600 {path}\n")
        if key:
            return key, f"Key 文件 {path}"
    return None, None


def resolve_api_key(cli_key=None):
    """按 CLI 参数 -> 环境变量 -> Key 文件 解析 API Key，缺失则报错退出。"""
    key, _ = find_key(cli_key)
    if not key:
        script = os.path.basename(sys.argv[0]) or "deepseek_research.py"
        sys.stderr.write(KEY_MISSING_MESSAGE.format(
            script=script, key_file=default_key_file(),
            help_url=KEY_HELP_URL, env_var=ENV_VAR,
        ))
        sys.exit(2)
    return key


def mask(key):
    if not key:
        return "<未设置>"
    if len(key) <= 8:
        return key[:2] + "****"
    return f"{key[:6]}****{key[-4:]}"


# -- HTTP 层 -----------------------------------------------------------------
class DeepSeekClient:
    def __init__(self, api_key, timeout=REQUEST_TIMEOUT, verbose=True):
        self.api_key = api_key
        self.timeout = timeout
        self.verbose = verbose
        self.session = requests.Session()
        self.session.headers.update({
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        })
        self.request_count = 0   # 成功计费请求数（HTTP 200 且有内容）
        self.call_count = 0      # 全部 HTTP 调用次数（含失败与重试）
        self.fetch_calls = 0     # 实际发出的 Fetch 次数，用于预算控制
        self.search_ok = 0       # 成功且有结果的 Search 次数（计费）
        self.fetch_ok = 0        # 成功且拿到正文的 Fetch 次数（计费）

    def _log(self, msg):
        if self.verbose:
            print(msg, file=sys.stderr, flush=True)

    def post(self, url, payload):
        """带退避重试的 POST，返回 (ok, data, error_dict)。"""
        last_err = None
        for attempt in range(1, MAX_RETRY + 1):
            try:
                self.call_count += 1
                resp = self.session.post(url, json=payload, timeout=self.timeout)
            except requests.RequestException as exc:
                last_err = {"status": None, "type": "network_error", "message": str(exc)}
                if attempt < MAX_RETRY:
                    self._sleep_backoff(attempt, "网络异常")
                    continue
                return False, None, last_err

            if resp.status_code == 200:
                try:
                    return True, resp.json(), None
                except ValueError:
                    return False, None, {"status": 200, "type": "invalid_json", "message": resp.text[:200]}

            err = self._parse_error(resp)
            last_err = err
            if resp.status_code in RETRY_STATUS and attempt < MAX_RETRY:
                self._sleep_backoff(attempt, f"HTTP {resp.status_code}")
                continue
            return False, None, err
        return False, None, last_err

    @staticmethod
    def _parse_error(resp):
        try:
            body = resp.json()
        except ValueError:
            body = {}
        e = body.get("error") or {}
        return {
            "status": resp.status_code,
            "type": e.get("type") or "",
            "message": e.get("message") or (resp.text or "")[:200],
            "hint": error_hint(resp.status_code),
        }

    def _sleep_backoff(self, attempt, reason):
        delay = min(2 ** (attempt - 1), 8) + random.uniform(0, 0.5)
        self._log(f"    [重试] {reason}，{delay:.1f}s 后重试（第 {attempt + 1} 次）")
        time.sleep(delay)

    # -- 业务接口 ------------------------------------------------------------
    def search(self, query, count=DEFAULT_COUNT, ori_question=None,
               localization=None, need_content=True):
        payload = {"query": query, "count": count, "need_content": need_content}
        if ori_question:
            payload["ori_question"] = ori_question
        if localization:
            payload["localization"] = localization
        ok, data, err = self.post(SEARCH_URL, payload)
        if not ok:
            return False, [], err
        results = data.get("results") or []
        if results:
            self.request_count += 1
            self.search_ok += 1
        return True, results, None

    def fetch(self, url):
        self.fetch_calls += 1
        ok, data, err = self.post(FETCH_URL, {"url": url})
        if not ok:
            return False, None, err
        content = data.get("content") or ""
        if content.strip():
            self.request_count += 1
            self.fetch_ok += 1
        return True, {"title": data.get("title") or "", "content": content}, None


def error_hint(status):
    return {
        400: "请求不合法，检查 query 长度（1-128）或 count 范围（1-50）",
        401: f"API Key 缺失或无效，请到 {KEY_HELP_URL} 重新获取并覆盖旧值",
        402: "账号余额不足，请先在 DeepSeek 平台充值",
        403: "Fetch 被目标站点安全策略拒绝，换其他来源或改用搜索引擎摘要",
        404: "无法获取可用网页内容（URL 失效 / 站点拦截 / 无正文），换其他结果",
        408: "读取请求体超时，已自动重试",
        413: "请求体超过大小限制",
        429: "触发 QPS 限速，已自动退避重试",
        500: "服务端内部错误，已自动重试",
        504: "下游请求超时，已自动重试",
    }.get(status, "")


# -- 打分与判定 ---------------------------------------------------------------
def whole_score(result):
    try:
        return float(result.get("rerank_score") or 0)
    except (TypeError, ValueError):
        return 0.0


def whole_authority(result):
    auth = result.get("authority")
    if auth is None:
        return -1
    try:
        return int(auth)
    except (TypeError, ValueError):
        return -1


def grade(result, min_score=GOOD_SCORE):
    """给单条搜索结果打可用性等级：good / usable / weak。"""
    score = whole_score(result)
    auth = whole_authority(result)
    has_text = bool((result.get("summary") or "").strip() or (result.get("snippet") or "").strip())
    if (score >= min_score or auth >= GOOD_AUTHORITY) and result.get("url"):
        tier = "good"
    elif result.get("url") and (has_text or score > 0):
        tier = "usable"
    else:
        tier = "weak"
    return {"tier": tier, "score": score, "authority": auth, "has_text": has_text}


def grade_all(results, min_score=GOOD_SCORE):
    graded = []
    for r in results:
        g = grade(r, min_score)
        row = dict(r)
        row.update(g)
        graded.append(row)
    order = {"good": 0, "usable": 1, "weak": 2}
    graded.sort(key=lambda x: (order[x["tier"]], -x["score"], -x["authority"]))
    return graded


def looks_blocked(text):
    low = (text or "")[:2000].lower()
    return any(h.lower() in low for h in BLOCK_HINTS)


def content_verdict(text):
    text = text or ""
    chars = len(text.strip())
    if chars == 0:
        return "empty", chars
    if chars < MIN_CONTENT_CHARS:
        return "too_short", chars
    if looks_blocked(text):
        return "blocked", chars
    return "ok", chars


# -- 查询改写 -----------------------------------------------------------------
OPERATOR_RE = re.compile(
    r'\s*(?:-?site:\S+|\blang:[^\s]+|\b(?:after|before):\S+|-"[^"]*"|-\S+|"[^"]*")\s*',
    re.IGNORECASE,
)
STOPWORDS = {
    "的", "了", "如何", "如何用", "怎么", "怎么用", "怎样", "怎样用", "什么", "哪些",
    "请问", "帮我", "请帮", "一下", "想知道", "有没有",
    "the", "a", "an", "how", "what", "why", "is", "are", "to", "of", "and", "or",
}


def normalize_query(q):
    return re.sub(r"\s+", " ", (q or "").strip())[:128]


def simplify_query(q):
    """去掉搜索操作符，只留语义部分。"""
    stripped = OPERATOR_RE.sub(" ", q or "")
    stripped = re.sub(r"\s+", " ", stripped).strip()
    return stripped[:128] or normalize_query(q)


def core_terms(q, limit=4):
    """抽取核心词：按空白与常见标点切分，去停用词，取前 limit 个。"""
    tokens = [t for t in re.split(r"[\s,，。;；、/|()（）]+", q or "") if t]
    kept = [t for t in tokens if t.lower() not in STOPWORDS]
    if not kept:
        kept = tokens
    if len(kept) <= 1:
        return normalize_query((kept[0] if kept else q)[:20])
    return normalize_query(" ".join(kept[:limit]))


def build_query_ladder(query, rounds, variants=None):
    """确定性的查询改写阶梯：原查询 -> 去操作符 -> 核心词 -> 外部传入变体。

    去重后只保留有意义的步数，不重复提交完全相同的查询（重复查询不会再拿到新结果，只会白计费）。
    """
    ladder = [normalize_query(query)]
    simp = simplify_query(query)
    if simp:
        ladder.append(simp)
    core = core_terms(simp or query)
    if core:
        ladder.append(core)
    for v in variants or []:
        v = normalize_query(v)
        if v:
            ladder.append(v)

    distinct = []
    for q in ladder:
        if q and q not in distinct:
            distinct.append(q)
    return distinct[:max(1, rounds)]


# -- 输出 ---------------------------------------------------------------------
def slugify(text, limit=40):
    text = re.sub(r"https?://", "", text or "")
    text = re.sub(r"[^\w\u4e00-\u9fff]+", "-", text, flags=re.UNICODE).strip("-")
    return (text[:limit] or "result").lower()


def make_output_dir(base, label):
    stamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    prefix = Path(base) if base else Path.cwd()
    out = prefix / f"{slugify(label)}_{stamp}"
    suffix = 1
    while out.exists():
        suffix += 1
        out = prefix / f"{slugify(label)}_{stamp}_{suffix}"
    out.mkdir(parents=True, exist_ok=True)
    (out / "pages").mkdir(exist_ok=True)
    return out


def write_json(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8")


def write_page(out_dir, index, item, source):
    name = f"{index:02d}_{slugify(item.get('title') or urlparse(item['url']).netloc, 40)}.md"
    path = out_dir / "pages" / name
    date = item.get("date") or ""
    header = (
        "---\n"
        f"url: {item['url']}\n"
        f"title: {json.dumps(item.get('title') or '', ensure_ascii=False)}\n"
        f"site: {json.dumps(item.get('site_name') or '', ensure_ascii=False)}\n"
        f"date: {date}\n"
        f"rerank_score: {item.get('score', 0)}\n"
        f"authority: {item.get('authority', -1)}\n"
        f"content_source: {source}\n"
        f"round: {item.get('round', 1)}\n"
        "---\n\n"
    )
    path.write_text(header + (item.get("content") or ""), encoding="utf-8")
    return path


# -- research 主流程 ----------------------------------------------------------
def obtain_content(client, item, force_fetch=False):
    """优先复用 Search 返回的正文，不足再调 Fetch。返回 (source, verdict, chars, err)。"""
    inline = item.get("content") or ""
    verdict, chars = content_verdict(inline)
    if verdict == "ok" and not force_fetch:
        return "search", verdict, chars, None

    if client.verbose:
        print(f"    [fetch] {item['url']}", file=sys.stderr, flush=True)
    ok, page, err = client.fetch(item["url"])
    time.sleep(FETCH_INTERVAL)
    if not ok:
        if verdict in ("ok", "too_short") and inline.strip():
            return "search", verdict, chars, err
        return "none", "fetch_failed", 0, err

    f_verdict, f_chars = content_verdict(page["content"])
    if f_verdict == "ok":
        item["title"] = page["title"] or item.get("title") or ""
        item["content"] = page["content"]
        return "fetch", f_verdict, f_chars, None
    if verdict == "ok":
        # 原本 inline 正文已可用（force_fetch 场景），保留 inline
        return "search", verdict, chars, None
    # 两边都不可用：采用 Fetch 的判定，对失败原因的描述比 too_short 更具体
    item["content"] = page["content"] or inline
    return "fetch", f_verdict, f_chars, None


def record_for(item, round_no, source, verdict, chars, ferr):
    return {
        "round": round_no,
        "url": item["url"],
        "title": item.get("title") or "",
        "site": item.get("site_name"),
        "date": item.get("date"),
        "rerank_score": item.get("score"),
        "authority": item.get("authority"),
        "tier": item.get("tier"),
        "content_source": source,
        "content_verdict": verdict,
        "chars": chars,
        "error": ferr,
        "path": None,
        "summary": item.get("summary") or item.get("snippet") or "",
    }


def research(client, args):
    """按次计费策略下的检索：一次搜索尽量多拿正文，Fetch 只补缺且受预算封顶。"""
    variants = [v.strip() for v in re.split(r"[|;\n]", getattr(args, "query_variants", None) or "") if v.strip()]
    ladder = build_query_ladder(args.query, args.rounds, variants)
    out_dir = make_output_dir(args.output_dir, args.query)
    rounds_log = []
    accepted = []
    seen = set()
    page_seq = 0

    if len(ladder) < args.rounds:
        print(f"[提示] 自动改写只得到 {len(ladder)} 阶段查询（少于 --rounds {args.rounds}），"
              f"将不重复提交相同 query。如需更多轮可传 --query-variants 自行指定改写。",
              file=sys.stderr, flush=True)

    for round_no, q in enumerate(ladder, 1):
        fetch_before = client.fetch_calls
        print(f"\n[第 {round_no} 轮] 搜索: {q}", file=sys.stderr, flush=True)
        ok, results, err = client.search(
            q, count=args.count, ori_question=args.ori_question,
            localization=args.localization, need_content=True,
        )
        if not ok:
            rounds_log.append({
                "round": round_no, "query": q, "ok": False, "error": err,
                "total": 0, "good": 0, "picked": [], "accepted": 0, "entries": [],
            })
            print(f"    搜索失败: {err.get('message')} ({err.get('hint')})", file=sys.stderr)
            continue

        graded = grade_all(results, args.min_score)
        good = [g for g in graded if g["tier"] == "good"]
        entries = []
        adopted = 0
        free_count = 0

        # 1) 免费正文：Search 已经随结果返回 content，落盘不再产生费用
        free_items = [g for g in graded
                      if g.get("url") and g["url"] not in seen
                      and content_verdict(g.get("content"))[0] == "ok"]
        for item in free_items:
            if adopted >= args.max_pages:
                break
            seen.add(item["url"])
            item["round"] = round_no
            verdict, chars = content_verdict(item.get("content"))
            rec = record_for(item, round_no, "search", verdict, chars, None)
            page_seq += 1
            path = write_page(out_dir, page_seq, item, "search")
            rec["path"] = str(path.relative_to(out_dir))
            accepted.append(rec)
            entries.append(rec)
            adopted += 1
            free_count += 1

        # 2) 补缺：只对缺正文的高价值结果调 Fetch，受 --max-fetch 封顶
        need = [g for g in graded
                if g.get("url") and g["url"] not in seen
                and g["tier"] in ("good", "usable")
                and content_verdict(g.get("content"))[0] != "ok"]
        need.sort(key=lambda x: (0 if x["tier"] == "good" else 1, -x["score"], -x["authority"]))
        room = max(0, args.max_fetch - (client.fetch_calls - fetch_before))
        fetched_try = 0
        for item in need[:room]:
            if adopted >= args.max_pages:
                break
            seen.add(item["url"])
            item["round"] = round_no
            source, verdict, chars, ferr = obtain_content(client, item, args.force_fetch)
            fetched_try += 1
            rec = record_for(item, round_no, source, verdict, chars, ferr)
            if verdict == "ok":
                page_seq += 1
                path = write_page(out_dir, page_seq, item, source)
                rec["path"] = str(path.relative_to(out_dir))
                accepted.append(rec)
                adopted += 1
            elif args.keep_partial:
                page_seq += 1
                path = write_page(out_dir, page_seq, item, source)
                rec["path"] = str(path.relative_to(out_dir))
            entries.append(rec)
        rounds_log.append({
            "round": round_no, "query": q, "ok": True, "error": None,
            "total": len(graded), "good": len(good),
            "picked": [e["url"] for e in entries],
            "accepted": adopted,
            "free_pages": free_count, "fetches": fetched_try,
            "entries": entries,
        })
        print(f"    结果 {len(graded)} 条（优质 {len(good)}）；免费正文 {free_count} 篇，"
              f"Fetch {fetched_try} 次，采纳 {adopted} 篇", file=sys.stderr, flush=True)

        if len(accepted) >= args.enough:
            print(f"    已达到 --enough {args.enough} 篇，停止迭代", file=sys.stderr)
            break
        if adopted == 0:
            print("    本轮无有效正文，尝试改写查询", file=sys.stderr)

    short_hits = [e for r in rounds_log for e in r.get("entries", [])
                  if e.get("content_verdict") in ("too_short", "blocked")]
    if accepted:
        status = "success" if len(accepted) >= args.enough else "partial"
    elif short_hits:
        status = "partial"
    else:
        status = "failed"
    report = build_report(args, out_dir, status, rounds_log, accepted, client)
    write_json(out_dir / "results.json", {
        "query": args.query,
        "status": status,
        "generated_at": datetime.now().isoformat(timespec="seconds"),
        "api_key": mask(client.api_key),
        "billing": {
            "successful_requests": client.request_count,
            "total_http_calls": client.call_count,
            "fetch_calls": client.fetch_calls,
            "note": "Search/Fetch 均按请求次数计费，与返回条数和正文长度无关",
        },
        "rounds": rounds_log,
        "accepted": accepted,
    })
    (out_dir / "report.md").write_text(report, encoding="utf-8")
    return out_dir, status, accepted, client


def build_report(args, out_dir, status, rounds_log, accepted, client):
    status_cn = {"success": "成功", "partial": "部分成功", "failed": "失败"}[status]
    lines = [
        f"# 搜索报告：{args.query}",
        "",
        f"- 生成时间：{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
        f"- 判定状态：**{status_cn}**（采纳 {len(accepted)} 篇有效正文）",
        f"- 请求消耗：Search 成功 {client.search_ok} 次、Fetch 成功 {client.fetch_ok} 次"
        f"（合计计费 {client.request_count} 次，发出 Fetch {client.fetch_calls} 次）",
        f"- 预估费用：约 {client.request_count * 0.005:.3f} 元"
        f"（按返回内容的请求数估算，5 元/1000 次，仅供参考）",
        f"- 检索轮次：{len(rounds_log)}",
        "",
        "## 轮次轨迹",
        "",
        "| 轮次 | 查询 | 结果数 | 优质数 | 免费正文 | Fetch | 采纳 | 判定 |",
        "|---|---|---|---|---|---|---|---|",
    ]
    for r in rounds_log:
        verdict = "成功" if r.get("accepted") else ("搜索失败" if not r.get("ok") else "无有效正文")
        lines.append(
            f"| {r['round']} | `{r['query']}` | {r.get('total', 0)} | {r.get('good', 0)} | "
            f"{r.get('free_pages', 0)} | {r.get('fetches', 0)} | {r.get('accepted', 0)} | {verdict} |"
        )

    lines += ["", "## 采纳的页面", ""]
    if not accepted:
        lines += ["未取得任何有效正文。", ""]
    for i, a in enumerate(accepted, 1):
        lines += [
            f"### {i}. {a['title'] or a['url']}",
            "",
            f"- 链接：{a['url']}",
            f"- 站点：{a['site'] or '未知'} ｜ 发布：{a['date'] or '未知'}",
            f"- 质量：rerank_score={a['rerank_score']} / authority={a['authority']}（{a['tier']}）",
            f"- 正文：{a['content_source']} 取得，{a['chars']} 字符 → `{a['path']}`",
            f"- 摘要：{(a['summary'] or '（无）')[:300]}",
            "",
        ]

    weak = [r for r in rounds_log if r.get("ok") and not r.get("accepted")]
    if weak:
        lines += ["## 失败与建议", ""]
        for r in weak:
            lines.append(f"- 第 {r['round']} 轮 `{r['query']}`：结果 {r.get('total', 0)} 条，"
                         f"优质 {r.get('good', 0)} 条，无一取得有效正文。")
        lines += [
            "- 建议：换更精确的实体名/版本号重搜；用 `site:` 限定高权威站点；"
            "用 `after:`/`before:` 收窄时效；或直接给出具体 URL 走 `fetch`。",
            "",
        ]
    return "\n".join(lines)


# -- 子命令 -------------------------------------------------------------------
def cmd_search(client, args):
    ok, results, err = client.search(
        args.query, count=args.count, ori_question=args.ori_question,
        localization=args.localization, need_content=args.need_content,
    )
    if not ok:
        print(json.dumps({"ok": False, "error": err}, ensure_ascii=False, indent=2))
        return 1
    graded = grade_all(results, args.min_score)
    if not args.json:
        print(f"搜索: {args.query}")
        print(f"结果: {len(graded)} 条（优质 {sum(1 for g in graded if g['tier'] == 'good')} 条）\n")
        for i, g in enumerate(graded, 1):
            print(f"{i}. [{g['tier']}] score={g['score']} auth={g['authority']} {g.get('title') or '(无标题)'}")
            print(f"   {g['url']}")
            if g.get("date"):
                print(f"   发布: {g['date']}")
            text = (g.get("summary") or g.get("snippet") or "").strip()
            if text:
                print(f"   {text[:240]}")
            print()
    payload = {"ok": True, "query": args.query, "results": graded}
    if args.output:
        path = Path(args.output)
        write_json(path, payload)
        print(f"结果已写入 {path}", file=sys.stderr)
    if args.json:
        print(json.dumps(payload, ensure_ascii=False, indent=2))
    return 0


def cmd_fetch(client, args):
    out_dir = make_output_dir(args.output_dir, urlparse(args.url).netloc or "page")
    ok, page, err = client.fetch(args.url)
    if not ok:
        print(json.dumps({"ok": False, "url": args.url, "error": err}, ensure_ascii=False, indent=2))
        return 1
    verdict, chars = content_verdict(page["content"])
    item = {
        "url": args.url, "title": page["title"], "content": page["content"],
        "score": None, "authority": None, "round": 1,
    }
    path = write_page(out_dir, 1, item, "fetch")
    write_json(out_dir / "results.json", {
        "url": args.url, "title": page["title"], "verdict": verdict,
        "chars": chars, "path": str(path), "generated_at": datetime.now().isoformat(timespec="seconds"),
    })
    if not args.json:
        print(f"标题: {page['title'] or '(无)'}")
        print(f"正文: {chars} 字符，判定 {verdict}")
        print(f"落地: {path}")
        print("\n--- 正文前 1200 字符 ---\n")
        print(page["content"][:1200])
    else:
        print(json.dumps({
            "ok": True, "url": args.url, "title": page["title"],
            "verdict": verdict, "chars": chars, "path": str(path),
        }, ensure_ascii=False, indent=2))
    return 0 if verdict == "ok" else 1


def cmd_research(client, args):
    out_dir, status, accepted, client = research(client, args)
    print(f"\n状态: {status}")
    print(f"采纳正文: {len(accepted)} 篇")
    print(f"输出目录: {out_dir.resolve()}")
    print(f"报告: {out_dir / 'report.md'}")
    if accepted:
        print("\n采纳页面:")
        for a in accepted:
            print(f"  - {a['title'] or a['url']}  ({a['chars']} 字符, {a['content_source']})")
    if args.json:
        print(json.dumps({
            "ok": status == "success", "status": status,
            "output_dir": str(out_dir.resolve()),
            "accepted": [{k: a[k] for k in ("url", "title", "chars", "content_source", "path")} for a in accepted],
        }, ensure_ascii=False, indent=2))
    return 0 if accepted else 1


# -- 凭据维护 ----------------------------------------------------------------
def cmd_set_key(args):
    """把 Key 写入跨平台统一的 Key 文件，绕开 shell 配置在各系统上的差异。"""
    path = default_key_file()

    if args.check:
        key, source = find_key(getattr(args, "api_key", None))
        if key:
            print(f"已找到 Key：{mask(key)}")
            print(f"来源：{source}")
            return 0
        print("未找到可用 Key（命令行参数 / 环境变量 / Key 文件均无）")
        print(f"Key 文件预期位置：{path}")
        return 1

    if args.delete:
        if path.is_file():
            path.unlink()
            print(f"已删除 {path}")
        else:
            print(f"文件不存在：{path}")
        return 0

    key = (getattr(args, "api_key", None) or os.environ.get(ENV_VAR) or "").strip()
    if not key:
        sys.stderr.write("请通过 --api-key 传入 Key，或先设好环境变量 "
                         f"{ENV_VAR}。获取地址：{KEY_HELP_URL}\n")
        return 2

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(key + "\n", encoding="utf-8")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass
    print(f"已写入 {path}（权限 600，仅当前用户可读）")
    print(f"Key：{mask(key)}")
    print("该文件对终端和 GUI 应用都生效，无需改 shell 配置；可用 `set-key --check` 复核。")
    return 0


# -- CLI ----------------------------------------------------------------------
def build_parser():
    p = argparse.ArgumentParser(
        description="DeepSeek 搜索 / 网页抓取 / 多轮检索",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    p.add_argument("--api-key", default=None, help=f"DeepSeek API Key（默认读环境变量 {ENV_VAR}）")
    p.add_argument("--timeout", type=int, default=REQUEST_TIMEOUT, help="单次请求超时秒数（默认 60）")
    p.add_argument("--quiet", action="store_true", help="不打印过程日志到 stderr")
    sub = p.add_subparsers(dest="cmd", required=True)

    def add_global_opts(sp):
        # 默认值用 SUPPRESS，避免子命令未传时把父级已解析的值覆盖成 None
        sp.add_argument("--api-key", default=argparse.SUPPRESS, help=f"DeepSeek API Key（默认读环境变量 {ENV_VAR}）")
        sp.add_argument("--timeout", type=int, default=argparse.SUPPRESS, help="单次请求超时秒数（默认 60）")
        sp.add_argument("--quiet", action="store_true", default=argparse.SUPPRESS, help="不打印过程日志到 stderr")

    def add_search_opts(sp, with_query=True):
        add_global_opts(sp)
        if with_query:
            sp.add_argument("query", help="搜索词，1～128 字符，支持 site:/lang:/after:/before: 等语法")
        sp.add_argument("--count", type=int, default=DEFAULT_COUNT,
                        help="单次搜索返回条数 1-50（默认 20；按次计费，调大不加钱）")
        sp.add_argument("--ori-question", default=None, help="触发搜索的用户原始问题，帮助理解意图")
        sp.add_argument("--localization", default=None, help="用于本地化的 IP 地址")
        sp.add_argument("--min-score", type=float, default=GOOD_SCORE, help="rerank_score 优质阈值（默认 1.5）")

    sp = sub.add_parser("search", help="单次搜索并打分")
    add_search_opts(sp)
    sp.add_argument("--need-content", action="store_true", help="同时返回网页正文（content 字段）")
    sp.add_argument("--output", default=None, help="把完整结果写入指定 JSON 文件")
    sp.add_argument("--json", action="store_true", help="以 JSON 输出")

    sp = sub.add_parser("fetch", help="抓取单个网页正文")
    add_global_opts(sp)
    sp.add_argument("url", help="要抓取的网页地址")
    sp.add_argument("--output-dir", default=None, help="输出根目录（默认当前目录）")
    sp.add_argument("--json", action="store_true", help="以 JSON 输出")

    sp = sub.add_parser("set-key", help="把 API Key 写入 Key 文件（跨平台统一，600 权限）")
    add_global_opts(sp)
    sp.add_argument("--check", action="store_true", help="只检查当前 Key 来源，不写入")
    sp.add_argument("--delete", action="store_true", help="删除已保存的 Key 文件")

    sp = sub.add_parser("research", help="搜索 -> 判定 -> 抓重点 -> 不足再搜")
    add_search_opts(sp)
    sp.add_argument("--max-fetch", type=int, default=DEFAULT_MAX_FETCH,
                    help="每轮 Fetch 次数上限，只用于补缺正文（默认 3，每次单独计费）")
    sp.add_argument("--max-pages", type=int, default=DEFAULT_MAX_PAGES,
                    help="每轮最多落盘的正文篇数（默认 12；Search 自带正文不额外计费）")
    sp.add_argument("--enough", type=int, default=DEFAULT_ENOUGH,
                    help="收集到多少篇有效正文即停止迭代（默认 3）")
    sp.add_argument("--rounds", type=int, default=DEFAULT_ROUNDS, help="最多检索轮次（默认 3）")
    sp.add_argument("--query-variants", default=None,
                    help="追加的改写查询，用 | 或 ; 分隔，例如 'query B|query C'；脚本自动阶梯不足时使用")
    sp.add_argument("--output-dir", default=None, help="输出根目录（默认当前目录）")
    sp.add_argument("--force-fetch", action="store_true", help="忽略 Search 返回的正文，强制调用 Fetch")
    sp.add_argument("--keep-partial", action="store_true", help="正文过短也落盘，便于人工核查")
    sp.add_argument("--json", action="store_true", help="额外输出 JSON 摘要")
    return p


def main():
    args = build_parser().parse_args()
    if args.cmd == "set-key":
        return cmd_set_key(args)
    api_key = resolve_api_key(getattr(args, "api_key", None))
    timeout = getattr(args, "timeout", REQUEST_TIMEOUT)
    verbose = not getattr(args, "quiet", False)
    client = DeepSeekClient(api_key, timeout=timeout, verbose=verbose)
    handlers = {"search": cmd_search, "fetch": cmd_fetch, "research": cmd_research}
    return handlers[args.cmd](client, args)


if __name__ == "__main__":
    sys.exit(main())
