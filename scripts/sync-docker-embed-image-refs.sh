#!/usr/bin/env bash
# Sync agent.toml [image].ref from the current version field (no bump, no :dev).
#
# Usage:
#   ./scripts/sync-docker-embed-image-refs.sh                    # all docker templates
#   ./scripts/sync-docker-embed-image-refs.sh picoclaw-manager   # one template
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMON="${ROOT}/scripts/docker-embed-agent-toml-common.sh"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"

# shellcheck source=scripts/docker-embed-agent-toml-common.sh
. "${COMMON}"

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
  sync_agent_toml_version_and_ref "${name}"
done
