#!/usr/bin/env bash
# List builtin embed template directories that ship a container image (Dockerfile present).
#
# Usage:
#   ./scripts/list-docker-embed-templates.sh
# Prints one template name per line (e.g. picoclaw-manager), sorted.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EMBED_ROOT="${ROOT}/internal/templates/embed"

if [ ! -d "${EMBED_ROOT}" ]; then
  echo "missing embed root: ${EMBED_ROOT}" >&2
  exit 1
fi

names=()
for dir in "${EMBED_ROOT}"/*; do
  [ -d "${dir}" ] || continue
  [ -f "${dir}/Dockerfile" ] || continue
  names+=("$(basename "${dir}")")
done

if [ "${#names[@]}" -eq 0 ]; then
  echo "no embed templates with Dockerfile under ${EMBED_ROOT}" >&2
  exit 1
fi

printf '%s\n' "${names[@]}" | sort
