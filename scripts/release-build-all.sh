#!/usr/bin/env bash
# Build release archives (csgclaw + csgclaw-cli) for one or all platforms listed in
# scripts/release-platforms.txt. Shared by GitHub Actions (one matrix cell) and GitLab
# (parallel matrix), and by local full builds.
#
# Usage:
#   ./scripts/release-build-all.sh              # all platforms from release-platforms.txt
#   ./scripts/release-build-all.sh linux amd64  # single platform
#
# Required env:
#   VERSION – tag or version string (e.g. v0.3.0)
#
# Optional env (see scripts/package-release.sh):
#   COMMIT, BUILD_TIME, DIST_DIR, GOCACHE, BOXLITE_CLI_VERSION, BOXLITE_CLI_BASE_URL
#   RELEASE_APPS – space-separated subset of csgclaw and csgclaw-cli (default: both)
#
# Do not set PACKAGE_MODE or INCLUDE_BOXLITE for APP=csgclaw: defaults in
# package-release.sh match bundled boxlite only on linux/* and darwin/arm64.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT}"

PLATFORMS_FILE="${ROOT}/scripts/release-platforms.txt"

VERSION="${VERSION:?VERSION is required}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
DIST_DIR="${DIST_DIR:-dist}"
GOCACHE="${GOCACHE:-${ROOT}/.gocache}"
mkdir -p "${DIST_DIR}" "${GOCACHE}"
export GOCACHE

chmod +x scripts/package-release.sh scripts/fetch-boxlite-cli.sh

RELEASE_APPS="${RELEASE_APPS:-csgclaw csgclaw-cli}"

release_builds_app() {
  case " ${RELEASE_APPS} " in
    *" $1 "*) return 0 ;;
    *) return 1 ;;
  esac
}

build_pair() {
  local goos="$1" goarch="$2"
  if release_builds_app csgclaw; then
    CGO_ENABLED=0 APP=csgclaw VERSION="${VERSION}" COMMIT="${COMMIT}" BUILD_TIME="${BUILD_TIME}" \
      DIST_DIR="${DIST_DIR}" GOCACHE="${GOCACHE}" \
      BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}" \
      ./scripts/package-release.sh "${goos}" "${goarch}"
  fi
  if release_builds_app csgclaw-cli; then
    CGO_ENABLED=0 APP=csgclaw-cli VERSION="${VERSION}" COMMIT="${COMMIT}" BUILD_TIME="${BUILD_TIME}" \
      DIST_DIR="${DIST_DIR}" GOCACHE="${GOCACHE}" \
      BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}" \
      INCLUDE_BOXLITE=0 \
      ./scripts/package-release.sh "${goos}" "${goarch}"
  fi
}

run_all_from_file() {
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "${line}" || "${line}" =~ ^# ]] && continue
    read -r goos goarch <<<"${line}"
    if [[ -z "${goos:-}" || -z "${goarch:-}" ]]; then
      echo "invalid platform line in ${PLATFORMS_FILE}: ${line}" >&2
      exit 1
    fi
    build_pair "${goos}" "${goarch}"
  done <"${PLATFORMS_FILE}"
}

if [[ "$#" -eq 2 ]]; then
  build_pair "$1" "$2"
elif [[ "$#" -eq 0 ]]; then
  [[ -f "${PLATFORMS_FILE}" ]] || {
    echo "missing ${PLATFORMS_FILE}" >&2
    exit 1
  }
  run_all_from_file
else
  echo "usage: $0 [<goos> <goarch>]" >&2
  exit 1
fi
