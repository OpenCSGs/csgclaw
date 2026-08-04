#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  printf 'usage: %s <goos> <goarch>\n' "$0" >&2
  exit 1
fi

GOOS_TARGET="$1"
GOARCH_TARGET="$2"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
OUTPUT_ROOT="$ROOT_DIR/desktop/out/input/${GOOS_TARGET}-${GOARCH_TARGET}"
BACKEND_ROOT="$OUTPUT_ROOT/backend"
TEMP_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEMP_ROOT"' EXIT

case "$OUTPUT_ROOT" in
  "$ROOT_DIR"/desktop/out/input/*) ;;
  *)
    printf 'refuse to replace unexpected desktop output path: %s\n' "$OUTPUT_ROOT" >&2
    exit 1
    ;;
esac

rm -rf "$OUTPUT_ROOT"
mkdir -p "$BACKEND_ROOT" "$TEMP_ROOT/release"

(
  cd "$ROOT_DIR"
  APP=csgclaw \
    VERSION="${VERSION:-dev}" \
    COMMIT="${COMMIT:-unknown}" \
    BUILD_TIME="${BUILD_TIME:-unknown}" \
    DIST_DIR="$TEMP_ROOT/release" \
    GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}" \
    INCLUDE_BOXLITE="${INCLUDE_BOXLITE:-0}" \
    BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}" \
    BOXLITE_CLI_BASE_URL="${BOXLITE_CLI_BASE_URL:-https://github.com/boxlite-ai/boxlite/releases/download}" \
    "$SCRIPT_DIR/package-release.sh" "$GOOS_TARGET" "$GOARCH_TARGET" >/dev/null
)

if [ "$GOOS_TARGET" = "windows" ]; then
  archive="$(find "$TEMP_ROOT/release" -maxdepth 1 -type f -name 'csgclaw_*.zip' -print -quit)"
  if [ -z "$archive" ]; then
    printf 'desktop backend archive was not produced for windows/%s\n' "$GOARCH_TARGET" >&2
    exit 1
  fi
  unzip -q "$archive" -d "$BACKEND_ROOT"
else
  archive="$(find "$TEMP_ROOT/release" -maxdepth 1 -type f -name 'csgclaw_*.tar.gz' -print -quit)"
  if [ -z "$archive" ]; then
    printf 'desktop backend archive was not produced for %s/%s\n' "$GOOS_TARGET" "$GOARCH_TARGET" >&2
    exit 1
  fi
  tar -xzf "$archive" -C "$BACKEND_ROOT"
fi

test -f "$BACKEND_ROOT/csgclaw/.csgclaw-bundle.json"
if [ "$GOOS_TARGET" = "windows" ]; then
  test -f "$BACKEND_ROOT/csgclaw/bin/codex.exe"
else
  test -f "$BACKEND_ROOT/csgclaw/bin/codex"
fi
printf 'Desktop backend ready: %s\n' "$BACKEND_ROOT/csgclaw"
