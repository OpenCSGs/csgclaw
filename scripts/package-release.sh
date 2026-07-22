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
PACKAGE_MODE="${PACKAGE_MODE:-}"
INCLUDE_BOXLITE="${INCLUDE_BOXLITE:-}"
VERSION_PKG="${VERSION_PKG:-csgclaw/internal/version}"
BOXLITE_CLI_VERSION="${BOXLITE_CLI_VERSION:-v0.9.0}"
BOXLITE_CLI_BASE_URL="${BOXLITE_CLI_BASE_URL:-https://github.com/boxlite-ai/boxlite/releases/download}"
WEB_STATIC_DIST_DIR="${WEB_STATIC_DIST_DIR:-web/static-dist}"
SANDBOX_CLI_CMD_PATH="${SANDBOX_CLI_CMD_PATH:-./cmd/csgclaw-cli}"
LDFLAGS="-X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}"
if [ "$APP" = "csgclaw-cli" ]; then
  LDFLAGS="-s -w ${LDFLAGS}"
fi

mkdir -p "$DIST_DIR"

require_web_assets() {
  if [ "$APP" != "csgclaw" ]; then
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

supports_boxlite_bundle() {
  case "$1/$2" in
    darwin/arm64|linux/amd64|linux/arm64) return 0 ;;
    *) return 1 ;;
  esac
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

resolve_include_boxlite() {
  if [ -n "$INCLUDE_BOXLITE" ]; then
    echo "$INCLUDE_BOXLITE"
    return
  fi

  if [ -n "$PACKAGE_MODE" ]; then
    case "$PACKAGE_MODE" in
      bundled-boxlite-cli) echo "1" ;;
      legacy-single-binary) echo "0" ;;
      *)
        echo "unsupported PACKAGE_MODE: ${PACKAGE_MODE}" >&2
        exit 1
        ;;
    esac
    return
  fi

  if [ "$APP" = "csgclaw" ] && supports_boxlite_bundle "$GOOS_TARGET" "$GOARCH_TARGET"; then
    echo "1"
    return
  fi

  echo "0"
}

INCLUDE_BOXLITE="$(resolve_include_boxlite)"
case "$INCLUDE_BOXLITE" in
  0|1) ;;
  *)
    echo "INCLUDE_BOXLITE must be 0 or 1, got: ${INCLUDE_BOXLITE}" >&2
    exit 1
    ;;
esac

if [ "$APP" != "csgclaw" ] && [ "$INCLUDE_BOXLITE" = "1" ]; then
  echo "INCLUDE_BOXLITE=1 is only supported for APP=csgclaw" >&2
  exit 1
fi

if [ "$APP" = "csgclaw" ] && [ "$INCLUDE_BOXLITE" = "1" ] && ! supports_boxlite_bundle "$GOOS_TARGET" "$GOARCH_TARGET"; then
  echo "bundled boxlite is not supported for ${GOOS_TARGET}/${GOARCH_TARGET}" >&2
  exit 1
fi

require_web_assets

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
  sandbox_cli_dir="${stage_dir}/sandbox-tools"
  host_cli_name="csgclaw-cli"
  if [ "$GOOS_TARGET" = "windows" ]; then
    host_cli_name="csgclaw-cli.exe"
  fi
  mkdir -p "$sandbox_cli_dir"
  env CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -ldflags "-s -w ${LDFLAGS}" -o "${sandbox_cli_dir}/csgclaw-cli" "${SANDBOX_CLI_CMD_PATH}"
  env CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" GOCACHE="$GOCACHE" \
    go build -ldflags "-s -w ${LDFLAGS}" -o "${stage_dir}/${host_cli_name}" "${SANDBOX_CLI_CMD_PATH}"
fi

if [ "$APP" = "csgclaw" ] && [ "$INCLUDE_BOXLITE" = "1" ]; then
  BOXLITE_CLI_VERSION="$BOXLITE_CLI_VERSION" \
  BOXLITE_CLI_BASE_URL="$BOXLITE_CLI_BASE_URL" \
  "$(dirname "$0")/fetch-boxlite-cli.sh" "$GOOS_TARGET" "$GOARCH_TARGET" "$stage_dir"
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
