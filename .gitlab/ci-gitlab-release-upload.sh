#!/usr/bin/env bash
# GitLab CI helper: upload dist/ and install scripts to SSH storage (no build).
# Builds run in parallel job release-build; merged artifacts are available here.
#
# Environment (required):
#   CI_COMMIT_TAG – release tag, e.g. v0.3.0
#   REMOTE_SSH    – e.g. root@cdn.example.com
#
# Optional:
#   CI_PROJECT_DIR – repo root (default: git root or pwd)
#   DIST_DIR       – default dist
#   REMOTE_BASE_DIR, SSH_PORT, SSH_IDENTITY_FILE
set -euo pipefail

ROOT="${CI_PROJECT_DIR:-}"
if [[ -z "${ROOT}" ]]; then
  ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
fi
cd "${ROOT}"

VERSION="${CI_COMMIT_TAG:?CI_COMMIT_TAG is required}"
DIST_DIR="${DIST_DIR:-dist}"

if [[ ! -d "${DIST_DIR}" ]] || ! compgen -G "${DIST_DIR}/*" >/dev/null; then
  echo "no files in ${DIST_DIR}/; run release-build or set DIST_DIR" >&2
  exit 1
fi

ssh_scp_opts() {
  SSH_OPTS=(-p "${SSH_PORT:-22}")
  SCP_OPTS=(-P "${SSH_PORT:-22}")
  if [[ -n "${SSH_IDENTITY_FILE:-}" ]]; then
    SSH_OPTS+=(-i "${SSH_IDENTITY_FILE}")
    SCP_OPTS+=(-i "${SSH_IDENTITY_FILE}")
  fi
}

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
scp "${SCP_OPTS[@]}" scripts/install.ps1 "${REMOTE_SSH}:${REMOTE_BASE}/install.ps1"
ssh "${SSH_OPTS[@]}" "${REMOTE_SSH}" "chmod 0644 '${REMOTE_BASE}/install.ps1'"

echo "==> Uploaded ${VERSION} to ${REMOTE_SSH}:${REMOTE_BASE}"
