#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <goos> <goarch> <output-dir>" >&2
  exit 1
fi

GOOS_TARGET="$1"
GOARCH_TARGET="$2"
OUTPUT_DIR="$3"
BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}"
BOXLITE_CLI_BASE_URL="${BOXLITE_CLI_BASE_URL:-https://github.com/boxlite-ai/boxlite/releases/download}"

resolve_target_suffix() {
  case "$1/$2" in
    darwin/arm64) echo "aarch64-apple-darwin" ;;
    linux/amd64) echo "x86_64-unknown-linux-gnu" ;;
    linux/arm64) echo "aarch64-unknown-linux-gnu" ;;
    *)
      echo "unsupported boxlite-cli target: $1/$2" >&2
      exit 1
      ;;
  esac
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd tar
need_cmd mktemp

target_suffix="$(resolve_target_suffix "$GOOS_TARGET" "$GOARCH_TARGET")"
archive_name="boxlite-cli-${BOXLITE_CLI_VERSION}-${target_suffix}.tar.gz"
download_url="${BOXLITE_CLI_BASE_URL}/${BOXLITE_CLI_VERSION}/${archive_name}"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

archive_path="${tmpdir}/${archive_name}"
extract_dir="${tmpdir}/extract"

mkdir -p "$OUTPUT_DIR" "$extract_dir"

echo "fetching ${download_url}"
curl_args=(-fsSL)
# Corporate TLS inspection may present a self-signed chain; set BOXLITE_CURL_INSECURE=1 only in trusted CI.
if [ "${BOXLITE_CURL_INSECURE:-}" = "1" ]; then
  curl_args+=(-k)
fi
curl "${curl_args[@]}" "$download_url" -o "$archive_path"
tar -xzf "$archive_path" -C "$extract_dir"

boxlite_path="$(find "$extract_dir" -type f -name boxlite | head -n 1)"
if [ -z "$boxlite_path" ]; then
  echo "boxlite binary not found in ${archive_name}" >&2
  exit 1
fi

install -m 0755 "$boxlite_path" "${OUTPUT_DIR}/boxlite"
