#!/usr/bin/env bash
# Stage runtime files into dist/ for embed templates that build container images.
# Discovers templates via Dockerfile (see scripts/list-docker-embed-templates.sh).
#
# Usage:
#   ./scripts/prepare-docker-embed-dist.sh                    # all docker templates
#   ./scripts/prepare-docker-embed-dist.sh picoclaw-manager   # one template
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"

prepare_template_dist() {
  local name="$1"
  local src="${ROOT}/internal/templates/embed/${name}"
  local dst="${src}/dist"

  if [ ! -f "${src}/Dockerfile" ]; then
    echo "template ${name} has no Dockerfile; skip prepare" >&2
    exit 1
  fi
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
  chmod +x "${LIST_SCRIPT}"
  templates=()
  while IFS= read -r name; do
    [ -n "${name}" ] && templates+=("${name}")
  done < <("${LIST_SCRIPT}")
  if [ "${#templates[@]}" -eq 0 ]; then
    exit 1
  fi
  set -- "${templates[@]}"
fi

for name in "$@"; do
  prepare_template_dist "${name}"
done
