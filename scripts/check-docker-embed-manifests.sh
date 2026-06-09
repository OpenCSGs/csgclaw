#!/usr/bin/env bash
# Exit 0 when all docker embed agent.toml files are current; 1 when sync is needed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMON="${ROOT}/scripts/docker-embed-agent-toml-common.sh"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"

# shellcheck source=scripts/docker-embed-agent-toml-common.sh
. "${COMMON}"

chmod +x "${LIST_SCRIPT}"
while IFS= read -r name; do
  [ -z "${name}" ] && continue
  if ! docker_embed_manifest_is_current "${name}"; then
    exit 1
  fi
done < <("${LIST_SCRIPT}")

exit 0
