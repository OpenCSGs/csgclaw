#!/usr/bin/env bash
# GitLab CI helper: build release archives (package-release.sh) and upload to SSH.
# Intended to run after before_script has configured ~/.ssh and REMOTE_SSH.
#
# Environment (required):
#   CI_COMMIT_TAG – release tag, e.g. v0.3.0
#   REMOTE_SSH    – e.g. root@cdn.example.com
#
# Optional:
#   CI_PROJECT_DIR – repo root (default: git root or pwd)
#   DIST_DIR       – default dist
#   PACKAGE_MODE, BOXLITE_CLI_VERSION, REMOTE_BASE_DIR, SSH_PORT, SSH_IDENTITY_FILE
#   GOPROXY – set by .gitlab/ci.yml variables (Go modules); optional override in CI vars
set -euo pipefail

ROOT="${CI_PROJECT_DIR:-}"
if [[ -z "${ROOT}" ]]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi
cd "${ROOT}"

chmod +x scripts/package-release.sh scripts/sync-agent-runtimes.sh scripts/fetch-boxlite-cli.sh

VERSION="${CI_COMMIT_TAG:?CI_COMMIT_TAG is required}"
COMMIT="$(git rev-parse --short HEAD)"
BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
DIST_DIR="${DIST_DIR:-dist}"
mkdir -p "${DIST_DIR}" "${ROOT}/.gocache"
GOCACHE="${GOCACHE:-${ROOT}/.gocache}"
export GOCACHE
PACKAGE_MODE="${PACKAGE_MODE:-bundled-boxlite-cli}"

ssh_scp_opts() {
  SSH_OPTS=(-p "${SSH_PORT:-22}")
  SCP_OPTS=(-P "${SSH_PORT:-22}")
  if [[ -n "${SSH_IDENTITY_FILE:-}" ]]; then
    SSH_OPTS+=(-i "${SSH_IDENTITY_FILE}")
    SCP_OPTS+=(-i "${SSH_IDENTITY_FILE}")
  fi
}

build_pair() {
  local goos="$1" goarch="$2"
  CGO_ENABLED=0 APP=csgclaw VERSION="${VERSION}" COMMIT="${COMMIT}" BUILD_TIME="${BUILD_TIME}" \
    DIST_DIR="${DIST_DIR}" GOCACHE="${GOCACHE}" PACKAGE_MODE="${PACKAGE_MODE}" \
    BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}" \
    ./scripts/package-release.sh "${goos}" "${goarch}"
  CGO_ENABLED=0 APP=csgclaw-cli VERSION="${VERSION}" COMMIT="${COMMIT}" BUILD_TIME="${BUILD_TIME}" \
    DIST_DIR="${DIST_DIR}" GOCACHE="${GOCACHE}" BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}" \
    ./scripts/package-release.sh "${goos}" "${goarch}"
}

build_pair linux amd64
build_pair linux arm64
build_pair darwin arm64

REMOTE_BASE="${REMOTE_BASE_DIR:-/data/csgclaw}"
REL="${REMOTE_BASE}/releases/${VERSION}"
ssh_scp_opts
ssh "${SSH_OPTS[@]}" "${REMOTE_SSH}" "mkdir -p '${REL}'"
scp "${SCP_OPTS[@]}" "${DIST_DIR}"/* "${REMOTE_SSH}:${REL}/"
ssh "${SSH_OPTS[@]}" "${REMOTE_SSH}" "find '${REL}' -type f -exec chmod 0644 {} \\;"

ssh_scp_opts
ssh "${SSH_OPTS[@]}" "${REMOTE_SSH}" "mkdir -p '${REMOTE_BASE}'"
scp "${SCP_OPTS[@]}" scripts/install.sh "${REMOTE_SSH}:${REMOTE_BASE}/install.sh"
ssh "${SSH_OPTS[@]}" "${REMOTE_SSH}" "chmod 0644 '${REMOTE_BASE}/install.sh'"

echo "==> Uploaded ${VERSION} to ${REMOTE_SSH}:${REMOTE_BASE}"
