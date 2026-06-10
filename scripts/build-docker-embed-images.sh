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

base_image_arg_for_template() {
  local name="$1"
  case "${name}" in
    picoclaw-*) printf 'PICOCLAW_IMAGE' ;;
    openclaw-*) printf 'OPENCLAW_IMAGE' ;;
    *) return 1 ;;
  esac
}

base_image_override_key_for_template() {
  local name="$1"
  case "${name}" in
    picoclaw-*) printf 'PICOCLAW_BASE_IMAGE' ;;
    openclaw-*) printf 'OPENCLAW_BASE_IMAGE' ;;
    *) return 1 ;;
  esac
}

build_one() {
  local name="$1"
  local tag image dockerfile base_arg override_key override_value

  tag="$(image_tag_for_template "${name}")"
  image="${ACR_REGISTRY}/opencsghq/${name}:${tag}"
  dockerfile="${ROOT}/internal/templates/embed/${name}/Dockerfile"
  if [ ! -f "${dockerfile}" ]; then
    echo "missing Dockerfile: ${dockerfile}" >&2
    exit 1
  fi

  base_arg=""
  override_key=""
  override_value=""
  if base_arg="$(base_image_arg_for_template "${name}")"; then
    override_key="$(base_image_override_key_for_template "${name}")"
    override_value="${!override_key:-}"
  fi

  if [ -n "${override_value}" ]; then
    echo "docker build ${name} -> ${image} (${base_arg} override from ${override_key}: ${override_value})"
    docker build -f "${dockerfile}" \
      --build-arg "${base_arg}=${override_value}" \
      -t "${image}" \
      "${ROOT}"
    return
  fi

  if [ -n "${base_arg}" ]; then
    echo "docker build ${name} -> ${image} (${base_arg} from Dockerfile default)"
  else
    echo "docker build ${name} -> ${image} (Dockerfile defaults)"
  fi
  docker build -f "${dockerfile}" \
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
