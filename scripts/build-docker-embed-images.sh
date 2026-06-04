#!/usr/bin/env bash
# Build container images for embed templates that include a Dockerfile.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"

: "${ACR_REGISTRY:?ACR_REGISTRY must be set}"
: "${PICOCLAW_BASE_IMAGE:?PICOCLAW_BASE_IMAGE must be set}"
: "${DOCKER_EMBED_IMAGE_TAG:?DOCKER_EMBED_IMAGE_TAG must be set}"

build_one() {
  local name="$1"
  local image="${ACR_REGISTRY}/opencsghq/${name}:${DOCKER_EMBED_IMAGE_TAG}"
  echo "docker build ${name} -> ${image}"
  docker build -f "${ROOT}/internal/templates/embed/${name}/Dockerfile" \
    --build-arg PICOCLAW_IMAGE="${PICOCLAW_BASE_IMAGE}" \
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
