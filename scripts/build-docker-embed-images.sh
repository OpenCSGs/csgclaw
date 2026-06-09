#!/usr/bin/env bash
# Build container images for embed templates that include a Dockerfile.
# Image tag defaults to agent.toml version (after bump-docker-embed-version.sh).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMON="${ROOT}/scripts/docker-embed-agent-toml-common.sh"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"

# shellcheck source=scripts/docker-embed-agent-toml-common.sh
. "${COMMON}"

: "${ACR_REGISTRY:?ACR_REGISTRY must be set}"

image_tag_for_template() {
  local name="$1"
  local manifest version

  manifest="$(docker_embed_manifest_path "${name}")"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest}" >&2
    exit 1
  fi
  version="$(read_agent_toml_version "${manifest}")"
  if [ -z "${version}" ]; then
    echo "missing version in ${manifest}; run scripts/bump-docker-embed-version.sh first" >&2
    exit 1
  fi
  printf '%s' "${version}"
}

build_one() {
  local name="$1"
  local tag image

  tag="$(image_tag_for_template "${name}")"
  image="${ACR_REGISTRY}/opencsghq/${name}:${tag}"
  if [ -n "${PICOCLAW_BASE_IMAGE:-}" ]; then
    echo "docker build ${name} -> ${image} (PICOCLAW_IMAGE override: ${PICOCLAW_BASE_IMAGE})"
    docker build -f "${ROOT}/internal/templates/embed/${name}/Dockerfile" \
      --build-arg "PICOCLAW_IMAGE=${PICOCLAW_BASE_IMAGE}" \
      -t "${image}" \
      "${ROOT}"
    return
  fi

  echo "docker build ${name} -> ${image} (PICOCLAW_IMAGE from Dockerfile default)"
  docker build -f "${ROOT}/internal/templates/embed/${name}/Dockerfile" \
    -t "${image}" \
    "${ROOT}"
}

chmod +x "${LIST_SCRIPT}"
if [ "$#" -eq 0 ]; then
  while IFS= read -r name; do
    [ -z "${name}" ] && continue
    build_one "${name}"
  done < <("${LIST_SCRIPT}")
else
  for name in "$@"; do
    build_one "${name}"
  done
fi
