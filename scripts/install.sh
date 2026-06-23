#!/usr/bin/env bash
set -euo pipefail

APP="${APP:-csgclaw}"
VERSION="${VERSION:-latest}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
LIB_DIR="${LIB_DIR:-$HOME/.local/lib/${APP}}"
CSGCLAW_HOME="${CSGCLAW_HOME:-$HOME/.csgclaw}"
SANDBOX_TOOLS_DIR="${SANDBOX_TOOLS_DIR:-${CSGCLAW_HOME}/sandbox-tools}"
# All release metadata and downloads use this host (override per env if needed).
MIRROR_HOST="${MIRROR_HOST:-https://csgclaw.opencsg.com}"
BASE_URL="${BASE_URL:-${MIRROR_HOST}/releases}"
# Latest-release JSON: top-level "tag_name" (GitHub releases/latest schema; mirror matches).
LATEST_RELEASE_URL="${LATEST_RELEASE_URL:-${MIRROR_HOST}/releases/latest}"
TMPDIR_INSTALL=""

cleanup() {
  if [ -n "${TMPDIR_INSTALL:-}" ] && [ -d "${TMPDIR_INSTALL:-}" ]; then
    rm -rf "$TMPDIR_INSTALL"
  fi
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

detect_os() {
  case "$(uname -s)" in
    Darwin) echo "darwin" ;;
    Linux) echo "linux" ;;
    *)
      echo "unsupported OS: $(uname -s)" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *)
      echo "unsupported architecture: $(uname -m)" >&2
      exit 1
      ;;
  esac
}

ensure_supported_platform() {
  case "$1/$2" in
    darwin/arm64|linux/amd64|linux/arm64) ;;
    *)
      echo "unsupported platform: $1/$2" >&2
      echo "prebuilt csgclaw binaries currently support macOS arm64, Linux amd64, and Linux arm64 only" >&2
      exit 1
      ;;
  esac
}

resolve_latest_version() {
  local json flat tag
  json="$(curl -fsSL "${LATEST_RELEASE_URL}")"
  flat="$(printf '%s' "$json" | tr -d '\n\r')"
  tag="$(printf '%s' "$flat" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  if [ -z "$tag" ]; then
    echo "failed to resolve latest release from ${LATEST_RELEASE_URL}" >&2
    echo "Expected JSON with top-level \"tag_name\" (GitHub releases/latest schema)." >&2
    exit 1
  fi
  echo "$tag"
}

ensure_install_dir() {
  mkdir -p "$INSTALL_DIR"
}

ensure_lib_dir() {
  mkdir -p "$LIB_DIR"
}

check_path_hint() {
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      cat <<EOF

$INSTALL_DIR is not on your PATH.
Add this line to your shell profile:
  export PATH="$INSTALL_DIR:\$PATH"
EOF
      ;;
  esac
}

main() {
  need_cmd curl
  need_cmd tar
  need_cmd mktemp
  need_cmd install

  local os arch version archive_name download_url archive_path extracted_path bundle_path bundle_bin_path bundle_cli_path install_root
  os="$(detect_os)"
  arch="$(detect_arch)"
  ensure_supported_platform "$os" "$arch"
  version="$VERSION"
  if [ "$version" = "latest" ]; then
    version="$(resolve_latest_version)"
  fi

  archive_name="${APP}_${version}_${os}_${arch}.tar.gz"
  download_url="${BASE_URL}/${version}/${archive_name}"

  TMPDIR_INSTALL="$(mktemp -d)"
  trap cleanup EXIT
  archive_path="${TMPDIR_INSTALL}/${archive_name}"

  echo "Downloading ${download_url}"
  curl -fsSL "$download_url" -o "$archive_path"

  tar -xzf "$archive_path" -C "$TMPDIR_INSTALL"
  bundle_path="${TMPDIR_INSTALL}/${APP}"
  bundle_bin_path="${bundle_path}/bin/${APP}"
  bundle_cli_path="${bundle_path}/bin/csgclaw_dir/csgclaw-cli"

  ensure_install_dir
  ensure_lib_dir

  if [ -f "$bundle_bin_path" ]; then
    if [ ! -f "$bundle_cli_path" ]; then
      echo "archive did not contain ${APP}/bin/csgclaw_dir/csgclaw-cli" >&2
      exit 1
    fi
    install_root="${LIB_DIR}/${version}"
    rm -rf "$install_root"
    mkdir -p "$install_root"
    cp -R "$bundle_path" "$install_root/"
    ln -sfn "${install_root}/${APP}/bin/${APP}" "${INSTALL_DIR}/${APP}"
    extracted_path="${install_root}/${APP}/bin/${APP}"
  else
    extracted_path="${TMPDIR_INSTALL}/${APP}"
    if [ ! -f "$extracted_path" ]; then
      echo "archive did not contain ${APP}" >&2
      exit 1
    fi
    install -m 0755 "$extracted_path" "${INSTALL_DIR}/${APP}"
    extracted_path="${INSTALL_DIR}/${APP}"
  fi

  mkdir -p "$SANDBOX_TOOLS_DIR"
  install -m 0755 "$bundle_cli_path" "${SANDBOX_TOOLS_DIR}/csgclaw-cli"

  cat <<EOF
Installed ${APP} ${version} to ${extracted_path}
Installed sandbox CLI to ${SANDBOX_TOOLS_DIR}/csgclaw-cli

Next steps:
  ${APP} serve
EOF
  check_path_hint
}

main "$@"
