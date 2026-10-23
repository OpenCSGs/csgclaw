#!/usr/bin/env python3
"""
PaddleOCR 凭据解析（供 paddleocr_to_md.py / paddleocr_batch.py 共用）

解析顺序:
  1. 命令行参数 --token
  2. 环境变量 PADDLE_OCR_TOKEN
  3. Token 文件 ~/.config/paddleocr/api_key（可用 PADDLE_OCR_KEY_FILE 改路径）

为什么需要 Token 文件：shell 配置（~/.zshrc、~/.bashrc）在不同系统、不同启动方式下
差异很大，GUI 应用（如 Proma、各种桌面客户端）通常根本不读 shell rc，只有真正由 shell
拉起的进程才继承那些 export。Token 文件与环境无关，终端和 GUI 都能读到。

单独使用:
  python scripts/paddleocr_key.py set --token <Token>    # 写入 Token 文件（600 权限）
  python scripts/paddleocr_key.py check                  # 查看当前 Token 来源（掩码）
  python scripts/paddleocr_key.py delete                 # 删除已保存的 Token
"""

import argparse
import os
import sys
from pathlib import Path

TOKEN_ENV_VAR = "PADDLE_OCR_TOKEN"
KEY_FILE_ENV = "PADDLE_OCR_KEY_FILE"
TOKEN_HELP_URL = "https://aistudio.baidu.com/account/accessToken"


def default_key_file():
    """跨平台统一的 Token 文件位置：~/.config/paddleocr/api_key"""
    override = (os.environ.get(KEY_FILE_ENV) or "").strip()
    if override:
        return Path(override).expanduser()
    return Path.home() / ".config" / "paddleocr" / "api_key"


def _perm_too_open(path):
    try:
        mode = path.stat().st_mode & 0o777
    except OSError:
        return False
    return mode & 0o077 != 0   # 组或其他用户可读即可视为过宽


def read_key_file(path):
    """读取 Token 文件，兼容纯 Token 单行与 `PADDLE_OCR_TOKEN=...` 两种写法。

    返回 (token, warn)；warn 为 'too_open' 表示文件权限过宽。
    """
    try:
        text = path.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None, None
    warn = "too_open" if _perm_too_open(path) else None
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if "=" in line:
            name, _, value = line.partition("=")
            if name.strip() == TOKEN_ENV_VAR:
                return value.strip().strip("'\""), warn
            continue
        return line.strip("'\""), warn
    return None, None


def find_token(cli_token=None):
    """查找 Token，返回 (token, 来源描述)；找不到返回 (None, None)。"""
    token = (cli_token or "").strip()
    if token:
        return token, "命令行参数"
    token = (os.environ.get(TOKEN_ENV_VAR) or "").strip()
    if token:
        return token, f"环境变量 {TOKEN_ENV_VAR}"
    path = default_key_file()
    if path.is_file():
        token, warn = read_key_file(path)
        if warn:
            sys.stderr.write(f"[提示] {path} 权限过宽，建议执行: chmod 600 {path}\n")
        if token:
            return token, f"Token 文件 {path}"
    return None, None


def resolve_token(cli_token=None, script=None):
    """按 CLI 参数 -> 环境变量 -> Token 文件 解析 Token，缺失则报错退出（退出码 2）。"""
    token, _ = find_token(cli_token)
    if token:
        return token
    script = script or os.path.basename(sys.argv[0]) or "paddleocr_to_md.py"
    sys.stderr.write(
        f"""[错误] 未找到 PaddleOCR API Token。

本 Skill 不内置任何 Token，需要你自己提供：
  1. 打开 {TOKEN_HELP_URL} 获取 Access Token
  2. 首选：写入 Token 文件（终端、GUI 应用、macOS / Linux / Windows 都能读到）
       python scripts/paddleocr_key.py set --token <你的 Token>
     文件位置：{default_key_file()}
  3. 或配置环境变量 {TOKEN_ENV_VAR}（只对真正读取该变量的进程有效，GUI 应用通常读不到）
       export {TOKEN_ENV_VAR}="<你的 Token>"
  4. 或仅在本次运行临时传入
       python {script} <文件> --token <你的 Token>
"""
    )
    sys.exit(2)


def mask(token):
    if not token:
        return "<未设置>"
    if len(token) <= 8:
        return token[:2] + "****"
    return f"{token[:6]}****{token[-4:]}"


def main():
    parser = argparse.ArgumentParser(
        description="PaddleOCR Token 管理（写入 / 检查 / 删除 ~/.config/paddleocr/api_key）",
    )
    parser.add_argument("action", choices=["set", "check", "delete"], help="set 写入 / check 检查 / delete 删除")
    parser.add_argument("--token", default=None, help="要保存的 Token；缺省时读取环境变量 PADDLE_OCR_TOKEN")
    args = parser.parse_args()
    path = default_key_file()

    if args.action == "check":
        token, source = find_token(args.token)
        if token:
            print(f"已找到 Token：{mask(token)}")
            print(f"来源：{source}")
            return 0
        print("未找到可用 Token（命令行参数 / 环境变量 / Token 文件均无）")
        print(f"Token 文件预期位置：{path}")
        return 1

    if args.action == "delete":
        if path.is_file():
            path.unlink()
            print(f"已删除 {path}")
        else:
            print(f"文件不存在：{path}")
        return 0

    token = (args.token or os.environ.get(TOKEN_ENV_VAR) or "").strip()
    if not token:
        sys.stderr.write(f"请通过 --token 传入 Token，或先设好环境变量 {TOKEN_ENV_VAR}。"
                         f"获取地址：{TOKEN_HELP_URL}\n")
        return 2
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(token + "\n", encoding="utf-8")
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass
    print(f"已写入 {path}（权限 600，仅当前用户可读）")
    print(f"Token：{mask(token)}")
    print("该文件对终端和 GUI 应用都生效，无需改 shell 配置；可用 `check` 复核。")
    return 0


if __name__ == "__main__":
    sys.exit(main())
