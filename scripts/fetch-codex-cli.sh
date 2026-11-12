#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <goos> <goarch> <output-dir>" >&2
  exit 1
fi

GOOS_TARGET="$1"
GOARCH_TARGET="$2"
OUTPUT_DIR="$3"
CODEX_CLI_DOWNLOAD_BASE_URL="${CODEX_CLI_DOWNLOAD_BASE_URL:-https://csgclaw.opencsg.com/codex-cli}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PLATFORM_MAP_FILE="${SCRIPT_DIR}/codex-cli-platforms.txt"
CODEX_CLI_VERSION="$(tr -d '\r\n' < "${SCRIPT_DIR}/codex-cli-version.txt")"
if [[ ! "$CODEX_CLI_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid bundled Codex CLI version: ${CODEX_CLI_VERSION}" >&2
  exit 1
fi

if [ ! -f "$PLATFORM_MAP_FILE" ]; then
  echo "Codex CLI platform map is missing: ${PLATFORM_MAP_FILE}" >&2
  exit 1
fi

resolve_platform() {
  awk -v goos="$1" -v goarch="$2" '
    /^[[:space:]]*#/ || NF == 0 { next }
    $1 == goos && $2 == goarch { print $3, $4, $5; found = 1; exit }
    END { exit !found }
  ' "$PLATFORM_MAP_FILE"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd awk
if ! platform="$(resolve_platform "$GOOS_TARGET" "$GOARCH_TARGET")"; then
  echo "unsupported bundled Codex CLI target: ${GOOS_TARGET}/${GOARCH_TARGET}" >&2
  exit 1
fi
read -r download_os download_arch archive_binary <<<"$platform"
artifact="codex-package_${CODEX_CLI_VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}.tar.gz"
expected_checksum="$(awk -v artifact="$artifact" '$2 == artifact { print $1 }' "${SCRIPT_DIR}/codex-cli-checksums.txt")"
if [[ ! "$expected_checksum" =~ ^[0-9a-f]{64}$ ]]; then
  echo "missing or invalid Codex CLI checksum: ${artifact}" >&2
  exit 1
fi
download_url="${CODEX_CLI_DOWNLOAD_BASE_URL%/}/v${CODEX_CLI_VERSION}/${artifact}"

verify_checksum() {
  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$1" | awk '{print $1}')"
  else
    need_cmd shasum
    actual="$(shasum -a 256 "$1" | awk '{print $1}')"
  fi
  if [ "$actual" != "$expected_checksum" ]; then
    echo "Codex CLI checksum mismatch: ${artifact}" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd mktemp
need_cmd tar
need_cmd cp
need_cmd install

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

archive_path="$tmpdir/codex.tar.gz"
extract_dir="$tmpdir/extract"
mkdir -p "$extract_dir"
echo "fetching bundled Codex CLI ${download_url}"
curl -fsSL --retry 3 "$download_url" -o "$archive_path"
verify_checksum "$archive_path"
tar -xzf "$archive_path" -C "$extract_dir"

for entry in "bin/$archive_binary" codex-package.json codex-resources codex-path; do
  if [ ! -e "$extract_dir/$entry" ]; then
    echo "Codex package did not contain expected entry: $entry" >&2
    exit 1
  fi
done
# Preserve the official package layout: helpers in bin, resources and metadata
# beside bin. Codex resolves bwrap and other bundled helpers from this layout.
mkdir -p "$OUTPUT_DIR"
package_root="$(cd "$OUTPUT_DIR/.." && pwd)"
for entry in codex-resources codex-path; do
  rm -rf "${package_root:?}/$entry"
  cp -R "$extract_dir/$entry" "$package_root/$entry"
done
cp "$extract_dir/codex-package.json" "$package_root/codex-package.json"
for binary in "$extract_dir/bin/"*; do
  install -m 0755 "$binary" "$OUTPUT_DIR/$(basename "$binary")"
done
