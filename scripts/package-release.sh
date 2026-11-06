#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <goos> <goarch>" >&2
  exit 1
fi

GOOS_TARGET="$1"
GOARCH_TARGET="$2"
APP="${APP:-csgclaw}"
CMD_PATH="${CMD_PATH:-./cmd/${APP}}"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
DIST_DIR="${DIST_DIR:-dist}"
GOCACHE="${GOCACHE:-$(pwd)/.gocache}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-}"
VERSION_PKG="${VERSION_PKG:-csgclaw/internal/version}"
WEB_STATIC_DIST_DIR="${WEB_STATIC_DIST_DIR:-web/static-dist}"
HOST_CLI_CMD_PATH="${HOST_CLI_CMD_PATH:-./cmd/csgclaw-cli}"
LDFLAGS="-X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}"
if [ "$APP" = "csgclaw-cli" ]; then
  LDFLAGS="-s -w ${LDFLAGS}"
fi

mkdir -p "$DIST_DIR"

require_ui_assets() {
  if [ "$APP" != "csgclaw" ]; then
    return
  fi

  local need_web=1 need_custom=0
  case ",${GO_BUILD_TAGS// /,}," in
    *,headless,*) return ;;
    *,ui_both,*) need_custom=1 ;;
    *,ui_custom,*) need_web=0; need_custom=1 ;;
  esac
  if [ "$need_custom" = 1 ] && [ ! -f web/custom/static-dist/index.html ]; then
    echo 'missing custom UI build output; run make prepare-custom-ui UI_DIR=<frontend-dist>' >&2
    exit 1
  fi
  if [ "$need_web" = 0 ]; then
    return
  fi

  if [ -f "${WEB_STATIC_DIST_DIR}/index.html" ]; then
    return
  fi

  cat >&2 <<EOF
missing Web UI build output: ${WEB_STATIC_DIST_DIR}/index.html

Build the embedded Web UI assets before packaging ${APP}.
- Local release flow: run 'make build-web' first.
- CI release flow: download the built web/static-dist artifact before running this script.
EOF
  exit 1
}

to_windows_path() {
  local path="$1"

  if command -v cygpath >/dev/null 2>&1; then
    cygpath -w "$path"
    return
  fi

  case "$path" in
    [A-Za-z]:\\*) printf '%s\n' "$path" ;;
    *)
      if [[ "$path" =~ ^/([A-Za-z])/(.*)$ ]]; then
        printf '%s\n' "${BASH_REMATCH[1]^}:\\${BASH_REMATCH[2]//\//\\}"
      else
        printf '%s\n' "${path//\//\\}"
      fi
      ;;
  esac
}

require_ui_assets

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

binary_name="$APP"
archive_ext="tar.gz"
if [ "$GOOS_TARGET" = "windows" ]; then
  binary_name="${APP}.exe"
  archive_ext="zip"
fi

stage_dir="$tmpdir"
binary_output="${tmpdir}/${binary_name}"
archive_source="${binary_name}"
if [ "$APP" = "csgclaw" ]; then
  stage_dir="${tmpdir}/${APP}/bin"
  mkdir -p "$stage_dir"
  binary_output="${stage_dir}/${binary_name}"
  archive_source="${APP}"
  cat > "${tmpdir}/${APP}/.csgclaw-bundle.json" <<EOF
{"app":"csgclaw","layout":"official-bundle","version":"${VERSION}"}
EOF
fi

if [ -n "$GO_BUILD_TAGS" ]; then
  env GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -tags "$GO_BUILD_TAGS" -ldflags "${LDFLAGS}" -o "${binary_output}" "${CMD_PATH}"
else
  env GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -ldflags "${LDFLAGS}" -o "${binary_output}" "${CMD_PATH}"
fi

if [ "$APP" = "csgclaw" ]; then
  host_cli_name="csgclaw-cli"
  if [ "$GOOS_TARGET" = "windows" ]; then
    host_cli_name="csgclaw-cli.exe"
  fi
  env CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -ldflags "-s -w ${LDFLAGS}" -o "${stage_dir}/${host_cli_name}" "${HOST_CLI_CMD_PATH}"
fi

if [ "$APP" = "csgclaw" ]; then
  CODEX_CLI_DOWNLOAD_BASE_URL="${CODEX_CLI_DOWNLOAD_BASE_URL:-https://csgclaw.opencsg.com/codex-cli/latest}" \
    "$(dirname "$0")/fetch-codex-cli.sh" "$GOOS_TARGET" "$GOARCH_TARGET" "$stage_dir"
fi

archive_base="${APP}_${VERSION}_${GOOS_TARGET}_${GOARCH_TARGET}"

if [ "$GOOS_TARGET" = "windows" ]; then
  archive_path="${DIST_DIR}/${archive_base}.zip"
  archive_output="$archive_path"
  case "$archive_output" in
    /*) ;;
    *) archive_output="${PWD}/${archive_output}" ;;
  esac
  if command -v zip >/dev/null 2>&1; then
    (
      cd "$tmpdir"
      if [ "$archive_source" = "$APP" ]; then
        zip -qr "${archive_output}" "${archive_source}"
      else
        zip -q "${archive_output}" "${binary_name}"
      fi
    )
  elif command -v powershell.exe >/dev/null 2>&1; then
    archive_input="$(to_windows_path "${tmpdir}/${archive_source}")"
    archive_output_windows="$(to_windows_path "${archive_output}")"
    ARCHIVE_INPUT="$archive_input" ARCHIVE_OUTPUT="$archive_output_windows" MSYS2_ARG_CONV_EXCL='*' \
      powershell.exe -NoLogo -NoProfile -Command \
      "Compress-Archive -LiteralPath \$env:ARCHIVE_INPUT -DestinationPath \$env:ARCHIVE_OUTPUT -Force" >/dev/null
  else
    echo "zip or powershell.exe is required to package Windows artifacts" >&2
    exit 1
  fi
else
  tar -C "$tmpdir" -czf "${DIST_DIR}/${archive_base}.tar.gz" "${archive_source}"
fi

echo "packaged ${DIST_DIR}/${archive_base}.${archive_ext}"
