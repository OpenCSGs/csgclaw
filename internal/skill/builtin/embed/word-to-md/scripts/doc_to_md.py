#!/usr/bin/env python3
"""
doc_to_md.py — 将 Word 文档（.doc / .docx）转换为 Markdown，并提取内嵌图片。

用法：
    python doc_to_md.py 文档.docx
    python doc_to_md.py 文档.doc --output-dir ./my_output

输出目录结构（自动生成）：
    {原文件名}_{时间戳}/
        {原文件名}.md
        imgs/               # 仅当文档包含图片时生成
            image_1.png
            image_2.jpg
            ...

依赖：
    pip install mammoth olefile pywin32 pillow python-docx
"""

import os
import sys
import re
import argparse
import datetime
import tempfile
import shutil
from pathlib import Path


# ── 工具函数 ──────────────────────────────────────────────────────────


def timestamp():
    return datetime.datetime.now().strftime("%Y%m%d_%H%M%S")


def safe_filename(path: str) -> str:
    """提取文件名（不含扩展名），过滤非法路径字符。"""
    name = os.path.splitext(os.path.basename(path))[0]
    # 替换 Windows 非法字符
    return re.sub(r'[<>:"/\\|?*]', '_', name)


def ensure_output_dir(doc_path: str, output_dir: str = None) -> str:
    """确定输出目录：若未指定则自动生成 {文件名}_{时间戳}。"""
    base = safe_filename(doc_path)
    if output_dir:
        out = os.path.join(output_dir, f"{base}_{timestamp()}")
    else:
        out = f"{base}_{timestamp()}"
    os.makedirs(out, exist_ok=True)
    return out


# ── .doc → .docx 转换（via Word COM） ──────────────────────────────


def has_com_word() -> bool:
    """检测系统是否安装了 Microsoft Word（通过 win32com）。"""
    try:
        import win32com.client
        return True
    except ImportError:
        return False


def convert_doc_to_docx(doc_path: str) -> tuple:
    """将 .doc 转换为 .docx，返回 (docx_path, temp_dir)。"""
    import win32com.client

    word = win32com.client.Dispatch("Word.Application")
    word.Visible = False
    word.DisplayAlerts = False

    tmp_dir = tempfile.mkdtemp(prefix="doc2md_")
    tmp_docx = os.path.join(tmp_dir, "converted.docx")

    try:
        abs_path = os.path.abspath(doc_path)
        doc = word.Documents.Open(abs_path)
        doc.SaveAs(tmp_docx, FileFormat=16)  # 16 = wdFormatXMLDocument
        doc.Close()
        return tmp_docx, tmp_dir
    except Exception:
        shutil.rmtree(tmp_dir, ignore_errors=True)
        raise
    finally:
        try:
            word.Quit()
        except Exception:
            pass


# ── 图片提取（mammoth 自定义处理器） ─────────────────────────────


def make_image_handler(img_dir: str):
    """
    返回一个 mammoth 可用的 convert_image 函数。
    将图片保存到 img_dir，并在 Markdown 中引用为 `imgs/xxx`。
    返回 (handler_func, counter_list)，counter_list[0] 记录图片数。
    """
    from mammoth.images import img_element

    counter = [0]  # 闭包可变计数

    def _save(image):
        counter[0] += 1
        n = counter[0]

        # 根据 content_type 确定后缀
        ext_map = {
            "image/png": ".png",
            "image/jpeg": ".jpg",
            "image/gif": ".gif",
            "image/bmp": ".bmp",
            "image/tiff": ".tif",
            "image/svg+xml": ".svg",
            "image/x-wmf": ".wmf",
            "image/x-emf": ".emf",
            "image/webp": ".webp",
        }
        ext = ext_map.get(image.content_type, ".png")

        img_name = f"image_{n}{ext}"
        img_path = os.path.join(img_dir, img_name)

        os.makedirs(img_dir, exist_ok=True)
        with image.open() as src:
            with open(img_path, "wb") as dst:
                dst.write(src.read())

        return {"src": f"imgs/{img_name}"}

    return img_element(_save), counter


# ── Word 内部书签清理 ────────────────────────────────────────


def strip_word_bookmarks(text: str) -> str:
    """
    移除 Word 内部书签标签（_Hlk, _Toc, _Ref, _GoBack 等），
    这类标签是 Word 自动或误插入的不可见锚点，在 Markdown 中无意义。
    """
    # 匹配 <a id="_XXX..."></a> 格式的空锚点
    text = re.sub(
        r'<a\s+id="_(' + '|'.join([
            'Hlk\\d+',
            'Toc\\d+',
            'Ref\\d+',
            'GoBack',
            'Picture\\d+',
            'DV\\d+',
            'mso\\w+',
            'x[\\da-fA-F]+\\d+',
            'cm[dns]_\\d+',
        ]) + r')"[^>]*></a>',
        '',
        text,
    )
    # 还用一种更保守的策略：移除所有 id 以 _ 开头的空 <a></a>
    # 但避免误删用户编写的带 href 的链接
    text = re.sub(r'<a\s+id="_[^"]*"[^>]*></a>', '', text)
    return text


# ── .docx → Markdown（mammoth） ───────────────────────────────────


def docx_to_markdown(docx_path: str, output_dir: str) -> tuple:
    """
    将 .docx 转为 Markdown 并提取图片。
    返回 (md_content, has_images, messages)。
    """
    import mammoth

    img_dir = os.path.join(output_dir, "imgs")
    handler, counter = make_image_handler(img_dir)

    with open(docx_path, "rb") as f:
        result = mammoth.convert_to_markdown(f, convert_image=handler)

    # 清理 Word 内部书签标签
    content = strip_word_bookmarks(result.value)

    has_images = counter[0] > 0

    if not has_images and os.path.isdir(img_dir):
        try:
            os.rmdir(img_dir)  # rmdir only works if empty
        except OSError:
            shutil.rmtree(img_dir, ignore_errors=True)

    return content, has_images, result.messages


# ── 纯文本回退（olefile，仅当 Word 不可用时） ─────────────────────


def extract_text_from_doc_ole(doc_path: str) -> str:
    """
    使用 olefile 从 .doc 中提取纯文本（会丢失格式/表格/图片）。
    仅作为 win32com 不可用时的回退。
    """
    import olefile

    ole = olefile.OleFileIO(doc_path)
    # 尝试读取 WordDocument stream 中的文本
    text = ""
    try:
        stream = ole.openstream("WordDocument")
        data = stream.read()
        # 尝试以 UTF-16LE 解码（Word 的默认编码）
        text = data.decode("utf-16-le", errors="replace")
        # 清理不可见控制字符
        text = re.sub(r"[\x00-\x08\x0b\x0c\x0e-\x1f]", "", text)
    except Exception:
        pass
    finally:
        ole.close()

    if text.strip():
        return f"# Word 文档（纯文本提取）\n\n> ⚠️ 由于 Microsoft Word 不可用，仅提取了纯文本内容，格式、表格、图片已丢失。\n\n{text.strip()}"
    else:
        return "# Word 文档\n\n> ⚠️ 无法提取文档内容（Microsoft Word 不可用，olefile 回退也失败）。"


# ── 主流程 ────────────────────────────────────────────────────────────


def convert_word_to_md(doc_path: str, output_dir: str = None) -> str:
    """
    主入口：将 Word 文档转为 Markdown。
    返回输出目录路径。
    """
    doc_path = os.path.abspath(doc_path)
    if not os.path.isfile(doc_path):
        raise FileNotFoundError(f"文件不存在: {doc_path}")

    ext = os.path.splitext(doc_path)[1].lower()
    if ext not in (".doc", ".docx"):
        raise ValueError(f"不支持的文件格式: {ext}（仅支持 .doc / .docx）")

    out_dir = ensure_output_dir(doc_path, output_dir)
    md_filename = safe_filename(doc_path) + ".md"
    md_path = os.path.join(out_dir, md_filename)

    is_doc = ext == ".doc"
    cleanup_dirs = []

    try:
        # ── 转换 ────────────────────────────────────────────
        if is_doc:
            # .doc 先转 .docx
            if has_com_word():
                tmp_docx, tmp_dir = convert_doc_to_docx(doc_path)
                cleanup_dirs.append(tmp_dir)
                md_content, has_images, messages = docx_to_markdown(tmp_docx, out_dir)
            else:
                # 回退：纯文本提取
                md_content = extract_text_from_doc_ole(doc_path)
                has_images = False
                messages = []
        else:
            # .docx 直接转换
            md_content, has_images, messages = docx_to_markdown(doc_path, out_dir)

        # ── 写入 Markdown ─────────────────────────────────
        with open(md_path, "w", encoding="utf-8") as f:
            f.write(md_content)

        # ── 打印告警信息 ──────────────────────────────────
        for msg in messages:
            msg_type = msg.type if hasattr(msg, "type") else "warning"
            print(f"[{msg_type}] {msg}", file=sys.stderr)

        # ── 汇报 ──────────────────────────────────────────
        img_info = ""
        if has_images:
            img_count = len(os.listdir(os.path.join(out_dir, "imgs")))
            img_info = f", extracted {img_count} images -> imgs/"
        print(f"[OK] Conversion done: {md_path}{img_info}")

        return out_dir

    except Exception as e:
        # 清理不完整的输出
        if os.path.isdir(out_dir):
            shutil.rmtree(out_dir, ignore_errors=True)
        raise e
    finally:
        for d in cleanup_dirs:
            shutil.rmtree(d, ignore_errors=True)


# ── CLI ────────────────────────────────────────────────────────────────


def main():
    parser = argparse.ArgumentParser(
        description="将 Word 文档（.doc/.docx）转换为 Markdown，并提取内嵌图片。"
    )
    parser.add_argument("input", help="输入的 Word 文件路径")
    parser.add_argument(
        "--output-dir",
        "-o",
        default=None,
        help="输出目录（默认自动生成: {文件名}_{时间戳}）",
    )
    args = parser.parse_args()

    try:
        convert_word_to_md(args.input, args.output_dir)
    except Exception as e:
        print(f"[ERROR] Conversion failed: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
