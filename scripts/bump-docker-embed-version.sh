#!/usr/bin/env bash
# Bump agent.toml version (last segment) and sync [image].ref for docker embed templates.
#
# Usage:
#   ./scripts/bump-docker-embed-version.sh                    # all docker templates
#   ./scripts/bump-docker-embed-version.sh picoclaw-manager   # one template
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
  bump_agent_toml_version_and_ref "${name}"
done
