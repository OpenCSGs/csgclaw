#!/usr/bin/env python3
"""
PaddleOCR -> Markdown 转换脚本

将本地 PDF/图片文件或在线文件 URL 提交至 PaddleOCR API 进行 OCR 识别，
输出为 Markdown 文档（含嵌入图片下载）。

支持的模型:
  - PaddleOCR-VL-1.6 (默认)
  - PaddleOCR-VL-1.4
  - PaddleOCR-VL-2
  - PaddleOCR-VL-1.1

异步 API 限制:
  - URL 模式: 单次请求最大 1000 页 PDF，文件 <= 200 MB
  - 本地文件模式: 文件 <= 50 MB

用法:
  python scripts/paddleocr_to_md.py <本地路径或URL> [选项]

示例:
  python scripts/paddleocr_to_md.py ./扫描件.pdf
  python scripts/paddleocr_to_md.py ./合同照片.png --model PaddleOCR-VL-2
  python scripts/paddleocr_to_md.py https://example.com/doc.pdf --skip-images
"""

import argparse
from datetime import datetime
import json
import os
import re
import sys
import time

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
        description="PaddleOCR -> Markdown 转换工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument("file", help="本地文件路径或在线文件 URL")
    parser.add_argument("--token", default=None,
                        help=f"API Token（默认读取环境变量 {TOKEN_ENV_VAR}；缺失时会提示获取方式并退出）")
    parser.add_argument("--model", default=DEFAULT_MODEL,
                        choices=["PaddleOCR-VL-1.6", "PaddleOCR-VL-1.4", "PaddleOCR-VL-2", "PaddleOCR-VL-1.1"],
                        help="OCR 模型（默认: PaddleOCR-VL-1.6）")
    parser.add_argument("--output-dir", default=None,
                        help="输出目录（自动生成: {文件名}_{时间戳}，可手动覆盖）")
    parser.add_argument("--poll-interval", type=int, default=POLL_INTERVAL,
                        help="轮询间隔秒数（默认: 5）")
    parser.add_argument("--timeout", type=int, default=MAX_POLL_TIME,
                        help="最大等待秒数（默认: 600）")
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
    return parser.parse_args()


def build_optional_payload(args):
    return {
        "useDocOrientationClassify": not args.no_orient_classify,
        "useDocUnwarping": not args.no_unwarping,
        "useChartRecognition": not args.no_chart_reco,
    }


def submit_job(args):
    headers = {
        "Authorization": f"bearer {args.token}",
    }

    optional_payload = build_optional_payload(args)
    file_path = args.file

    print(f"[文件] {file_path}")
    print(f"[模型] {args.model}")
    print(f"[配置] 方向分类={optional_payload['useDocOrientationClassify']}, "
          f"展开矫正={optional_payload['useDocUnwarping']}, "
          f"图表识别={optional_payload['useChartRecognition']}")

    if file_path.startswith(("http://", "https://", "ftp://")):
        # URL 模式
        headers["Content-Type"] = "application/json"
        payload = {
            "fileUrl": file_path,
            "model": args.model,
            "optionalPayload": optional_payload,
        }
        print("[模式] 在线文件 URL")
        job_response = requests.post(JOB_URL, json=payload, headers=headers)
    else:
        # 本地文件模式
        if not os.path.exists(file_path):
            print(f"[错误] 文件不存在: {file_path}")
            sys.exit(1)
        file_size = os.path.getsize(file_path)
        print(f"[模式] 本地文件 ({file_size / 1024:.1f} KB)")
        if file_size > 50 * 1024 * 1024:
            print("[错误] 本地文件超过 50 MB 限制")
            sys.exit(1)

        data = {
            "model": args.model,
            "optionalPayload": json.dumps(optional_payload),
        }
        with open(file_path, "rb") as f:
            files = {"file": f}
            job_response = requests.post(JOB_URL, headers=headers, data=data, files=files)

    print(f"[状态] HTTP {job_response.status_code}")
    if job_response.status_code != 200:
        print(f"[错误] 提交失败: {job_response.text}")
        sys.exit(1)

    result = job_response.json()
    job_id = result["data"]["jobId"]
    print(f"[成功] 任务提交成功, Job ID: {job_id}")
    return job_id


def poll_until_done(job_id, args):
    headers = {
        "Authorization": f"bearer {args.token}",
    }
    url = f"{JOB_URL}/{job_id}"
    start_time = time.time()

    print("[等待] 开始轮询任务结果...")

    while True:
        elapsed = time.time() - start_time
        if elapsed > args.timeout:
            print(f"[错误] 超时: 等待 {args.timeout} 秒后任务仍未完成")
            sys.exit(1)

        response = requests.get(url, headers=headers)
        if response.status_code != 200:
            print(f"[错误] 查询失败: {response.status_code} {response.text}")
            sys.exit(1)

        data = response.json()["data"]
        state = data["state"]

        if state == "pending":
            print(f"  [排队中] (已等待 {int(elapsed)}s)")
        elif state == "running":
            try:
                progress = data["extractProgress"]
                total = progress["totalPages"]
                extracted = progress["extractedPages"]
                print(f"  [识别中] {extracted}/{total} 页 (已等待 {int(elapsed)}s)")
            except KeyError:
                print(f"  [识别中] (已等待 {int(elapsed)}s)")
        elif state == "done":
            progress = data["extractProgress"]
            extracted = progress["extractedPages"]
            print(f"  [完成] 成功识别 {extracted} 页")
            print(f"  [时间] 开始: {progress['startTime']}, 结束: {progress['endTime']}")
            return data["resultUrl"]["jsonUrl"]
        elif state == "failed":
            error_msg = data.get("errorMsg", "未知错误")
            print(f"[错误] 任务失败: {error_msg}")
            sys.exit(1)
        else:
            print(f"[未知] 状态: {state}")

        time.sleep(args.poll_interval)


def extract_basename(file_path):
    """从文件路径或 URL 中提取基础文件名（不含扩展名）。"""
    if file_path.startswith(("http://", "https://", "ftp://")):
        # 从 URL 末尾提取文件名
        url_path = file_path.rstrip("/").split("/")[-1]
        if url_path and "." in url_path:
            return os.path.splitext(url_path)[0]
        return "document"
    else:
        return os.path.splitext(os.path.basename(file_path))[0]


def sanitize_filename(name):
    """清理文件名中的非法字符。"""
    return re.sub(r'[\\/*?:"<>|]', "_", name)


def download_results(jsonl_url, args):
    print(f"\n[下载] 获取结果: {jsonl_url}")
    jsonl_response = requests.get(jsonl_url)
    jsonl_response.raise_for_status()

    lines = jsonl_response.text.strip().split("\n")

    # 自动生成输出目录: {文件名}_{时间戳}
    basename = extract_basename(args.file)
    basename = sanitize_filename(basename)
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    if args.output_dir:
        output_dir = args.output_dir
    else:
        output_dir = f"{basename}_{timestamp}"
    os.makedirs(output_dir, exist_ok=True)

    all_pages_text = []
    page_num = 0
    any_image_downloaded = False

    for line in lines:
        line = line.strip()
        if not line:
            continue

        result = json.loads(line)["result"]

        for res in result["layoutParsingResults"]:
            md_text = res["markdown"]["text"]

            # 提取 markdown 文本中实际引用的图片路径
            # 匹配 Markdown 语法 ![...](...) 和 HTML 标签 <img src="...">
            referenced_paths = set()
            for m in re.finditer(r'!\[.*?\]\((.*?)\)', md_text):
                referenced_paths.add(m.group(1))
            for m in re.finditer(r'<img[^>]+src=[\'"]\s*([^\'"\s]+)\s*[\'"]', md_text):
                referenced_paths.add(m.group(1))

            # 只下载 markdown 文本中实际引用的图片
            if not args.skip_images and referenced_paths:
                # 在 API 返回的 images 字典中查找匹配的图片 URL
                for img_rel_path, img_url in res["markdown"]["images"].items():
                    if img_rel_path in referenced_paths:
                        img_full_path = os.path.join(output_dir, img_rel_path)
                        os.makedirs(os.path.dirname(img_full_path), exist_ok=True)
                        try:
                            img_data = requests.get(img_url).content
                            with open(img_full_path, "wb") as f:
                                f.write(img_data)
                            print(f"  [图片] {img_full_path}")
                            any_image_downloaded = True
                        except Exception as e:
                            print(f"  [警告] 图片下载失败 ({img_rel_path}): {e}")

            all_pages_text.append(md_text)

            # 下载版面检测结果图片
            if not args.skip_layout_images:
                for img_name, img_url in res.get("outputImages", {}).items():
                    try:
                        img_resp = requests.get(img_url)
                        if img_resp.status_code == 200:
                            filename = os.path.join(output_dir, f"{img_name}_{page_num}.jpg")
                            with open(filename, "wb") as f:
                                f.write(img_resp.content)
                            print(f"  [图片] {filename}")
                        else:
                            print(f"  [警告] 版面图片下载失败 ({img_name}): HTTP {img_resp.status_code}")
                    except Exception as e:
                        print(f"  [警告] 版面图片下载失败 ({img_name}): {e}")

            page_num += 1

    # 合并所有页面为一个 Markdown 文件
    merged_text = "\n\n".join(all_pages_text)
    md_filename = os.path.join(output_dir, f"{basename}.md")
    with open(md_filename, "w", encoding="utf-8") as f:
        f.write(merged_text)
    print(f"\n  [合并] {md_filename} ({len(all_pages_text)} 页合并)")

    return page_num, output_dir


def main():
    args = parse_args()
    args.token = resolve_token(args.token)
    print(f"  [凭据] 使用 Token {mask_token(args.token)}", file=sys.stderr)
    job_id = submit_job(args)
    jsonl_url = poll_until_done(job_id, args)
    page_count, output_dir = download_results(jsonl_url, args)
    print(f"\n全部完成! 共处理 {page_count} 页, 输出目录: {os.path.abspath(output_dir)}")


if __name__ == "__main__":
    main()
