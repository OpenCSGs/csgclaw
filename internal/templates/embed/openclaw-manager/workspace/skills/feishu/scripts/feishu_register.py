#!/usr/bin/env python3
"""Compatibility entrypoint for the CSGClaw Feishu registration helper."""

from __future__ import annotations

from feishu_setup.commands import main


if __name__ == "__main__":
    raise SystemExit(main())
