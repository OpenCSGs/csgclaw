#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WEB_APP_DIR="${WEB_APP_DIR:-$ROOT_DIR/web/app}"
NODE_VERSION="$(tr -d '[:space:]' < "$ROOT_DIR/.nvmrc")"

fail() {
  printf '%s\n' "$*" >&2
  exit 1
}

node_supported() {
  if ! command -v node >/dev/null 2>&1; then
    return 1
  fi
  node -e "const [major, minor] = process.versions.node.split('.').map(Number); process.exit((major === 22 && minor >= 13) || major === 23 || major === 24 ? 0 : 1)"
}

load_nvm() {
  if command -v nvm >/dev/null 2>&1; then
    return 0
  fi
  local nvm_dir="${NVM_DIR:-$HOME/.nvm}"
  if [ -s "$nvm_dir/nvm.sh" ]; then
    export NVM_DIR="$nvm_dir"
    # shellcheck source=/dev/null
    . "$nvm_dir/nvm.sh"
  fi
}

ensure_node() {
  if node_supported; then
    return 0
  fi

  load_nvm
  if command -v nvm >/dev/null 2>&1; then
    if ! nvm use "$NODE_VERSION" >/dev/null 2>&1; then
      printf 'Installing Node.js %s from .nvmrc with nvm...\n' "$NODE_VERSION" >&2
      nvm install "$NODE_VERSION" >/dev/null
      nvm use "$NODE_VERSION" >/dev/null
    elif ! node_supported; then
      printf 'Installing Node.js %s from .nvmrc with nvm...\n' "$NODE_VERSION" >&2
      nvm install "$NODE_VERSION" >/dev/null
      nvm use "$NODE_VERSION" >/dev/null
    fi
  fi

  if ! node_supported; then
    fail "Node.js >=22.13.0 and <25 is required for the Web UI. Install Node.js $NODE_VERSION or another supported version; current node is $(command -v node >/dev/null 2>&1 && node -v || printf 'missing')."
  fi
}

pnpm_supported() {
  node -e "const version = process.argv[1] || ''; const major = Number(version.split('.')[0]); process.exit(major >= 9 && major < 12 ? 0 : 1)" "$1"
}

run_pnpm() {
  cd "$WEB_APP_DIR"
  if command -v pnpm >/dev/null 2>&1; then
    local actual
    actual="$(pnpm --version)"
    if pnpm_supported "$actual"; then
      exec pnpm "$@"
    fi
  fi

  if command -v corepack >/dev/null 2>&1; then
    exec corepack pnpm "$@"
  fi

  fail "pnpm >=9 and <12 is required for the Web UI. Install a supported pnpm version, or use Node.js with Corepack."
}

check_toolchain() {
  ensure_node
  cd "$WEB_APP_DIR"

  local pnpm_version
  if command -v pnpm >/dev/null 2>&1; then
    pnpm_version="$(pnpm --version)"
  elif command -v corepack >/dev/null 2>&1; then
    pnpm_version="$(corepack pnpm --version)"
  else
    fail "pnpm >=9 and <12 is required for the Web UI. Install a supported pnpm version, or use Node.js with Corepack."
  fi
  if ! pnpm_supported "$pnpm_version"; then
    fail "pnpm >=9 and <12 is required for the Web UI; current pnpm is $pnpm_version."
  fi

  printf 'Web toolchain OK: Node.js %s, pnpm %s\n' "$(node -v)" "$pnpm_version"
}

if [ "${1:-}" = "--check" ]; then
  check_toolchain
  exit 0
fi

ensure_node
run_pnpm "$@"
