#!/usr/bin/env python3
"""
PaddleOCR 批量转换脚本

批量将本地 PDF/图片文件提交至 PaddleOCR API 进行 OCR 识别，
支持可配置的并发数、进度追踪、中断恢复和结果汇总。

用法:
  python scripts/paddleocr_batch.py ./pdfs/*.pdf
  python scripts/paddleocr_batch.py ./合同/ --max-concurrent 5
  python scripts/paddleocr_batch.py ./pdfs/**/*.pdf --output-dir ./ocr_batch_output
  python scripts/paddleocr_batch.py ./files/ --resume              # 从上次中断处继续
"""

import argparse
from datetime import datetime
import glob
import json
import os
import re
import sys
import time
import threading

import requests

# -- 运行常量 ----------------------------------------------------------------
JOB_URL = "https://paddleocr.aistudio-app.com/api/v2/ocr/jobs"
DEFAULT_MODEL = "PaddleOCR-VL-1.6"
POLL_INTERVAL = 5
MAX_POLL_TIME = 600

# -- 凭据 --------------------------------------------------------------------
# Token 解析顺序：--token -> 环境变量 PADDLE_OCR_TOKEN -> ~/.config/paddleocr/api_key
# 具体实现见同目录的 paddleocr_key.py（可用 `python scripts/paddleocr_key.py check` 查来源）
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from paddleocr_key import TOKEN_ENV_VAR, mask as mask_token, resolve_token  # noqa: E402


def parse_args():
    parser = argparse.ArgumentParser(
        description="PaddleOCR 批量转换工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("input", nargs="+", help="文件路径/glob 模式或目录（支持多个）")
    parser.add_argument("--token", default=None,
                        help=f"API Token（默认读取环境变量 {TOKEN_ENV_VAR}；缺失时会提示获取方式并退出）")
    parser.add_argument("--model", default=DEFAULT_MODEL,
                        choices=["PaddleOCR-VL-1.6", "PaddleOCR-VL-1.4", "PaddleOCR-VL-2", "PaddleOCR-VL-1.1"],
                        help="OCR 模型（默认: PaddleOCR-VL-1.6）")
    parser.add_argument("--output-dir", default="./paddleocr_batch_output",
                        help="批量输出根目录（默认: ./paddleocr_batch_output）")
    parser.add_argument("--max-concurrent", type=int, default=3,
                        help="最大并发任务数（默认: 3，推荐 3-5）")
    parser.add_argument("--poll-interval", type=int, default=POLL_INTERVAL,
                        help="轮询间隔秒数（默认: 5）")
    parser.add_argument("--timeout", type=int, default=MAX_POLL_TIME,
                        help="单个任务最大等待秒数（默认: 600）")
    parser.add_argument("--no-orient-classify", action="store_true",
                        help="禁用文档方向分类")
    parser.add_argument("--no-unwarping", action="store_true",
                        help="禁用文档展开矫正")
    parser.add_argument("--no-chart-reco", action="store_true",
                        help="禁用图表识别")
    parser.add_argument("--skip-images", action="store_true",
                        help="不下载 markdown 中的图片")
    parser.add_argument("--skip-layout-images", action="store_true",
                        help="不下载版面检测结果图片")
    parser.add_argument("--resume", action="store_true",
                        help="从上次中断处继续（仅适用于同一输出目录）")
    parser.add_argument("--extensions", default=".pdf,.png,.jpg,.jpeg,.bmp,.tiff,.tif",
                        help="要处理的文件扩展名，逗号分隔（默认: .pdf,.png,.jpg,.jpeg,.bmp,.tiff,.tif）")
    return parser.parse_args()


def build_optional_payload(args):
    return {
        "useDocOrientationClassify": not args.no_orient_classify,
        "useDocUnwarping": not args.no_unwarping,
        "useChartRecognition": not args.no_chart_reco,
    }


def collect_files(args):
    """收集所有待处理的文件列表。"""
    extensions = {ext.strip().lower() if ext.strip().startswith(".") else "." + ext.strip().lower()
                  for ext in args.extensions.split(",")}
    files = []

    for pattern in args.input:
        # 如果是目录，扫描其下的所有文件
        if os.path.isdir(pattern):
            for root, _, filenames in os.walk(pattern):
                for fn in filenames:
                    ext = os.path.splitext(fn)[1].lower()
                    if ext in extensions:
                        files.append(os.path.join(root, fn))
        else:
            # glob 模式展开
            matched = glob.glob(pattern, recursive=True)
            if matched:
                for fp in matched:
                    if os.path.isfile(fp):
                        ext = os.path.splitext(fp)[1].lower()
                        if ext in extensions:
                            files.append(os.path.abspath(fp))
            else:
                # 尝试直接作为路径
                if os.path.isfile(pattern):
                    ext = os.path.splitext(pattern)[1].lower()
                    if ext in extensions:
                        files.append(os.path.abspath(pattern))

    # 去重并保持顺序
    seen = set()
    unique = []
    for f in files:
        norm = os.path.normpath(os.path.abspath(f))
        if norm not in seen:
            seen.add(norm)
            unique.append(norm)

    # 过滤超过 50MB 的文件
    valid = []
    for f in unique:
        size = os.path.getsize(f)
        if size > 50 * 1024 * 1024:
            print(f"  [跳过] {f} ({size/1024/1024:.1f} MB) - 超过 50 MB 限制")
        else:
            valid.append(f)

    return sorted(valid)


def get_state_file(args):
    """获取状态文件路径。"""
    return os.path.join(args.output_dir, "_batch_state.json")


def load_state(state_file):
    """加载上次中断时的状态。"""
    if os.path.exists(state_file):
        with open(state_file, "r", encoding="utf-8") as f:
            return json.load(f)
    return {"completed": [], "failed": [], "in_progress": {}}


def save_state(state_file, state):
    """保存当前状态（原子写入）。"""
    tmp = state_file + ".tmp"
    with open(tmp, "w", encoding="utf-8") as f:
        json.dump(state, f, ensure_ascii=False, indent=2)
    os.replace(tmp, state_file)


def submit_single_file(file_path, args, headers, optional_payload):
    """提交单个文件到 PaddleOCR API，返回 jobId。"""
    with open(file_path, "rb") as f:
        data = {
            "model": args.model,
            "optionalPayload": json.dumps(optional_payload),
        }
        files = {"file": f}
        resp = requests.post(JOB_URL, headers=headers, data=data, files=files)

    if resp.status_code != 200:
        raise RuntimeError(f"HTTP {resp.status_code}: {resp.text}")
    return resp.json()["data"]["jobId"]


def poll_job(job_id, file_path, args, headers):
    """轮询单个任务直到完成，返回 jsonl_url 或 None（失败）。"""
    url = f"{JOB_URL}/{job_id}"
    start_time = time.time()

    while True:
        elapsed = time.time() - start_time
        if elapsed > args.timeout:
            raise TimeoutError(f"等待超时 ({args.timeout}秒)")

        resp = requests.get(url, headers=headers)
        if resp.status_code != 200:
            raise RuntimeError(f"查询失败: HTTP {resp.status_code}")

        data = resp.json()["data"]
        state = data["state"]

        if state == "done":
            return data["resultUrl"]["jsonUrl"]
        elif state == "failed":
            raise RuntimeError(data.get("errorMsg", "未知错误"))
        elif state == "pending" or state == "running":
            time.sleep(args.poll_interval)
        else:
            raise RuntimeError(f"未知状态: {state}")


def download_single_result(jsonl_url, file_path, args, basename):
    """下载单个任务的结果并保存。"""
    jsonl_resp = requests.get(jsonl_url)
    jsonl_resp.raise_for_status()
    lines = jsonl_resp.text.strip().split("\n")

    # 每个文件一个独立输出子目录
    output_dir = os.path.join(args.output_dir, basename)
    os.makedirs(output_dir, exist_ok=True)

    all_pages_text = []
    page_num = 0

    for line in lines:
        line = line.strip()
        if not line:
            continue

        result = json.loads(line)["result"]
        for res in result["layoutParsingResults"]:
            md_text = res["markdown"]["text"]

            # 提取实际引用的图片路径
            referenced_paths = set()
            for m in re.finditer(r'!\[.*?\]\((.*?)\)', md_text):
                referenced_paths.add(m.group(1))
            for m in re.finditer(r'<img[^>]+src=[\'"]\s*([^\'"\s]+)\s*[\'"]', md_text):
                referenced_paths.add(m.group(1))

            # 只下载实际引用的图片
            if not args.skip_images and referenced_paths:
                for img_rel_path, img_url in res["markdown"]["images"].items():
                    if img_rel_path in referenced_paths:
                        img_full_path = os.path.join(output_dir, img_rel_path)
                        os.makedirs(os.path.dirname(img_full_path), exist_ok=True)
                        try:
                            img_data = requests.get(img_url).content
                            with open(img_full_path, "wb") as f:
                                f.write(img_data)
                        except Exception:
                            pass  # 静默跳过图片下载失败

            all_pages_text.append(md_text)
            page_num += 1

    # 保存合并的 Markdown
    md_filename = os.path.join(output_dir, f"{basename}.md")
    with open(md_filename, "w", encoding="utf-8") as f:
        f.write("\n\n".join(all_pages_text))

    return page_num, md_filename


def sanitize_filename(name):
    """清理文件名中的非法字符。"""
    return re.sub(r'[\\/*?:"<>|]', "_", name)


def print_progress_bar(completed, failed, total, active, start_time):
    """打印进度条。"""
    elapsed = time.time() - start_time
    done = completed + failed
    pct = done / total * 100 if total > 0 else 0
    bar_len = 30
    filled = int(bar_len * done / total) if total > 0 else 0
    bar = "#" * filled + "." * (bar_len - filled)

    rate = done / elapsed if elapsed > 0 else 0
    eta_secs = (total - done) / rate if rate > 0 else 0
    eta_str = f"{int(eta_secs // 60)}m{int(eta_secs % 60)}s" if eta_secs > 0 else "--"

    elapsed_str = f"{int(elapsed // 60)}m{int(elapsed % 60)}s"

    line = f"\r  [{bar}] {done}/{total} ({pct:.0f}%) OK:{completed} FAIL:{failed} | conc:{active} time:{elapsed_str} ETA:{eta_str}  "
    try:
        sys.stdout.write(line)
        sys.stdout.flush()
    except UnicodeEncodeError:
        # fallback for terminals with limited charset
        safe = line.encode("ascii", errors="replace").decode("ascii")
        sys.stdout.write(safe)
        sys.stdout.flush()


def process_worker(file_queue, results, args, headers, optional_payload, lock):
    """工作线程：从队列取文件，提交并轮询，下载结果。"""
    while True:
        try:
            file_path = file_queue.pop(0)
        except IndexError:
            break

        basename = sanitize_filename(os.path.splitext(os.path.basename(file_path))[0])
        result_entry = {"file": file_path, "basename": basename, "status": "running", "pages": 0, "output": "", "error": ""}

        try:
            # 提交
            job_id = submit_single_file(file_path, args, headers, optional_payload)

            # 轮询
            jsonl_url = poll_job(job_id, file_path, args, headers)

            # 下载结果
            pages, md_path = download_single_result(jsonl_url, file_path, args, basename)

            result_entry["status"] = "completed"
            result_entry["pages"] = pages
            result_entry["output"] = md_path

            with lock:
                results["completed"].append(result_entry)

        except Exception as e:
            result_entry["status"] = "failed"
            result_entry["error"] = str(e)
            with lock:
                results["failed"].append(result_entry)


def main():
    args = parse_args()
    args.token = resolve_token(args.token)
    print(f"  [凭据] 使用 Token {mask_token(args.token)}", file=sys.stderr)
    start_time = time.time()

    # 收集文件
    print("正在扫描文件...")
    all_files = collect_files(args)
    total = len(all_files)
    if total == 0:
        print("未找到待处理的文件。")
        return

    print(f"找到 {total} 个文件待处理")

    # 创建输出目录
    os.makedirs(args.output_dir, exist_ok=True)

    # 加载状态（resume 模式）
    state_file = get_state_file(args)
    skip_files = set()
    if args.resume and os.path.exists(state_file):
        state = load_state(state_file)
        for entry in state.get("completed", []):
            skip_files.add(os.path.normpath(entry["file"]))
        for entry in state.get("failed", []):
            skip_files.add(os.path.normpath(entry["file"]))
        print(f"[恢复] 跳过 {len(skip_files)} 个已完成/失败的文件")
    else:
        state = {"completed": [], "failed": [], "in_progress": {}}

    # 过滤需要处理的文件
    pending = [f for f in all_files if os.path.normpath(f) not in skip_files]
    remaining = len(pending)
    print(f"本次需处理: {remaining} 个文件 (并发: {args.max_concurrent})")

    if remaining == 0:
        print("所有文件已完成!")
        return

    # 准备并发处理
    headers = {"Authorization": f"bearer {args.token}"}
    optional_payload = build_optional_payload(args)

    file_queue = pending[:]
    results = {"completed": state.get("completed", [])[:], "failed": state.get("failed", [])[:]}
    lock = threading.Lock()

    # 启动工作线程
    num_workers = min(args.max_concurrent, remaining)
    threads = []
    for _ in range(num_workers):
        t = threading.Thread(
            target=process_worker,
            args=(file_queue, results, args, headers, optional_payload, lock),
        )
        t.start()
        threads.append(t)

    # 进度显示线程
    stop_progress = threading.Event()

    def progress_updater():
        while not stop_progress.is_set():
            with lock:
                c = len(results["completed"])
                f = len(results["failed"])
            active = max(0, min(args.max_concurrent, remaining - c - f))
            print_progress_bar(c, f, remaining, active, start_time)

            # 定期保存状态
            if (c + f) % 5 == 0 and (c + f) > 0:
                state_snapshot = {"completed": results["completed"], "failed": results["failed"], "in_progress": {}}
                save_state(state_file, state_snapshot)

            stop_progress.wait(2)

    progress_thread = threading.Thread(target=progress_updater, daemon=True)
    progress_thread.start()

    # 等待所有线程完成
    for t in threads:
        t.join()

    stop_progress.set()

    # 最终进度
    with lock:
        final_completed = len(results["completed"])
        final_failed = len(results["failed"])
    print_progress_bar(final_completed, final_failed, remaining, 0, start_time)
    print()

    # 最终保存状态
    save_state(state_file, results)

    # 输出汇总报告
    elapsed_total = time.time() - start_time
    print(f"\n{'='*60}")
    print(f"批量处理完成!")
    print(f"  总文件: {total}")
    print(f"  成功:   {final_completed}")
    print(f"  失败:   {final_failed}")
    print(f"  耗时:   {int(elapsed_total // 60)}分 {int(elapsed_total % 60)}秒")
    if final_completed > 0:
        avg = elapsed_total / final_completed
        print(f"  平均:   {avg:.1f} 秒/文件")
    print(f"  输出:   {os.path.abspath(args.output_dir)}")

    if final_failed > 0:
        print(f"\n失败的 {final_failed} 个文件:")
        for entry in results["failed"]:
            print(f"  ✗ {os.path.basename(entry['file'])}: {entry['error'][:80]}")

    # 生成汇总报告文件
    report_path = os.path.join(args.output_dir, "_batch_report.json")
    report = {
        "start_time": datetime.fromtimestamp(start_time).isoformat(),
        "end_time": datetime.now().isoformat(),
        "total": total,
        "completed": final_completed,
        "failed": final_failed,
        "elapsed_seconds": elapsed_total,
        "model": args.model,
        "max_concurrent": args.max_concurrent,
        "completed_files": [
            {"file": e["file"], "pages": e["pages"], "output": e["output"]}
            for e in results["completed"]
        ],
        "failed_files": [
            {"file": e["file"], "error": e["error"]}
            for e in results["failed"]
        ],
    }
    with open(report_path, "w", encoding="utf-8") as f:
        json.dump(report, f, ensure_ascii=False, indent=2)
    print(f"\n报告: {report_path}")


if __name__ == "__main__":
    main()
