#!/usr/bin/env bash
# Stage PicoClaw runtime files into each template's dist/ directory.
# Copies everything under internal/templates/embed/<name>/ except Dockerfile and dist/.
#
# Usage:
#   ./scripts/prepare-picoclaw-embed-dist.sh                  # manager + worker
#   ./scripts/prepare-picoclaw-embed-dist.sh picoclaw-manager # one template
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

prepare_template_dist() {
  local name="$1"
  local src="${ROOT}/internal/templates/embed/${name}"
  local dst="${src}/dist"

  if [ ! -d "${src}" ]; then
    echo "missing template source: ${src}" >&2
    exit 1
  fi

  rm -rf "${dst}"
  mkdir -p "${dst}"
  find "${src}" -mindepth 1 -maxdepth 1 ! -name Dockerfile ! -name dist -exec cp -R {} "${dst}/" \;
  echo "prepared ${dst}"
}

if [ "$#" -eq 0 ]; then
  set -- picoclaw-manager picoclaw-worker
fi

for name in "$@"; do
  prepare_template_dist "${name}"
done
