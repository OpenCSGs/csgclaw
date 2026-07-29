#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

WEB_APP_DIR="$ROOT_DIR/desktop" exec "$SCRIPT_DIR/web-pnpm.sh" "$@"
