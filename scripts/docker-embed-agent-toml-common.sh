#!/usr/bin/env bash
# Shared helpers for docker embed template agent.toml (version + image.ref).
set -euo pipefail

docker_embed_root() {
  if [ -z "${DOCKER_EMBED_ROOT:-}" ]; then
    DOCKER_EMBED_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  fi
  printf '%s' "${DOCKER_EMBED_ROOT}"
}

docker_embed_manifest_path() {
  local template="$1"
  printf '%s/internal/templates/embed/%s/agent.toml' "$(docker_embed_root)" "${template}"
}

toml_escape_basic_string() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  printf '%s' "$value"
}

validate_image_ref() {
  local ref="$1"
  local stripped

  stripped="${ref//$'\n'/}"
  if [ "${#stripped}" -ne "${#ref}" ]; then
    echo "image ref must not contain control characters" >&2
    exit 1
  fi
  stripped="${ref//$'\r'/}"
  if [ "${#stripped}" -ne "${#ref}" ]; then
    echo "image ref must not contain control characters" >&2
    exit 1
  fi
}

read_agent_toml_version() {
  local manifest="$1"
  awk -F= '
    /^version[[:space:]]*=[[:space:]]*/ {
      value = $2
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
  ' "${manifest}"
}

read_agent_toml_image_ref() {
  local manifest="$1"
  awk '
    /^ref[[:space:]]*=[[:space:]]*/ {
      value = $0
      sub(/^[^=]*=[[:space:]]*/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
  ' "${manifest}"
}

image_ref_tag() {
  local ref="$1"
  printf '%s' "${ref##*:}"
}

# Exit 0 when manifest version and image.ref tag are in sync; 1 when sync is needed.
docker_embed_manifest_is_current() {
  local template="$1"
  local manifest version ref ref_tag

  manifest="$(docker_embed_manifest_path "${template}")"
  if [ ! -f "${manifest}" ]; then
    return 1
  fi

  ref="$(read_agent_toml_image_ref "${manifest}")"
  if [ -z "${ref}" ]; then
    return 1
  fi

  version="$(read_agent_toml_version "${manifest}")"
  if [ -z "${version}" ]; then
    return 1
  fi

  ref_tag="$(image_ref_tag "${ref}")"
  if [ "${ref_tag}" != "${version}" ]; then
    return 1
  fi

  return 0
}

increment_version_last_segment() {
  local version="$1"
  if [[ ! "${version}" =~ ^([0-9]+(\.[0-9]+)*)\.([0-9]+)$ ]]; then
    echo "invalid version format: ${version} (expected e.g. 0.1.0)" >&2
    return 1
  fi
  local prefix="${BASH_REMATCH[1]}"
  local last="${BASH_REMATCH[3]}"
  printf '%s.%s' "${prefix}" "$((last + 1))"
}

embed_image_ref_env_key() {
  local template="$1"
  local key="EMBED_IMAGE_REF_${template}"
  key="${key//-/_}"
  printf '%s' "${key}"
}

legacy_image_ref_for_template() {
  local template="$1"
  case "${template}" in
    picoclaw-manager)
      if [ -n "${PICOCLAW_MANAGER_IMAGE_REF+x}" ]; then
        printf '%s' "${PICOCLAW_MANAGER_IMAGE_REF}"
        return 0
      fi
      ;;
    picoclaw-worker)
      if [ -n "${PICOCLAW_WORKER_IMAGE_REF+x}" ]; then
        printf '%s' "${PICOCLAW_WORKER_IMAGE_REF}"
        return 0
      fi
      ;;
    openclaw-manager)
      if [ -n "${OPENCLAW_MANAGER_IMAGE_REF+x}" ]; then
        printf '%s' "${OPENCLAW_MANAGER_IMAGE_REF}"
        return 0
      fi
      ;;
    openclaw-worker)
      if [ -n "${OPENCLAW_WORKER_IMAGE_REF+x}" ]; then
        printf '%s' "${OPENCLAW_WORKER_IMAGE_REF}"
        return 0
      fi
      ;;
  esac
  return 1
}

image_ref_for_template() {
  local template="$1"
  local tag="$2"
  local key ref

  if ref="$(legacy_image_ref_for_template "${template}")"; then
    printf '%s' "${ref}"
    return 0
  fi

  key="$(embed_image_ref_env_key "${template}")"
  if [ -n "${!key+x}" ]; then
    printf '%s' "${!key}"
    return 0
  fi

  if [ -n "${ACR_REGISTRY:-}" ]; then
    printf '%s' "${ACR_REGISTRY}/opencsghq/${template}:${tag}"
    return 0
  fi

  return 1
}

patch_agent_toml_ref() {
  local template="$1"
  local image_ref="$2"
  local manifest
  local escaped_ref

  manifest="$(docker_embed_manifest_path "${template}")"
  if [ -z "${image_ref}" ]; then
    return 0
  fi
  validate_image_ref "${image_ref}"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest}" >&2
    exit 1
  fi

  escaped_ref="$(toml_escape_basic_string "${image_ref}")"
  export DOCKER_EMBED_IMAGE_REF="${escaped_ref}"

  awk '
    BEGIN {
      ref = ENVIRON["DOCKER_EMBED_IMAGE_REF"]
      in_image = 0
      ref_done = 0
      has_image_section = 0
    }
    /^\[image\]/ {
      has_image_section = 1
      print
      in_image = 1
      next
    }
    in_image && /^ref = / {
      print "ref = \"" ref "\""
      ref_done = 1
      in_image = 0
      next
    }
    /^\[/ {
      if (in_image && !ref_done) {
        print "ref = \"" ref "\""
        ref_done = 1
      }
      in_image = 0
    }
    { print }
    END {
      if (has_image_section && in_image && !ref_done) {
        print "ref = \"" ref "\""
        ref_done = 1
      }
      if (!has_image_section) {
        print ""
        print "[image]"
        print "ref = \"" ref "\""
      }
    }
  ' "${manifest}" > "${manifest}.tmp"

  unset DOCKER_EMBED_IMAGE_REF
  mv "${manifest}.tmp" "${manifest}"
}

patch_agent_toml_version() {
  local template="$1"
  local version="$2"
  local manifest
  local escaped_version

  manifest="$(docker_embed_manifest_path "${template}")"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest}" >&2
    exit 1
  fi

  escaped_version="$(toml_escape_basic_string "${version}")"
  export DOCKER_EMBED_TEMPLATE_VERSION="${escaped_version}"

  awk '
    BEGIN {
      version = ENVIRON["DOCKER_EMBED_TEMPLATE_VERSION"]
      version_done = 0
    }
    /^version[[:space:]]*=[[:space:]]*/ {
      print "version = \"" version "\""
      version_done = 1
      next
    }
    { print }
    END {
      if (!version_done) {
        print "version = \"" version "\""
      }
    }
  ' "${manifest}" > "${manifest}.tmp"

  unset DOCKER_EMBED_TEMPLATE_VERSION
  mv "${manifest}.tmp" "${manifest}"
}

bump_agent_toml_version_and_ref() {
  local template="$1"
  local manifest current next ref

  manifest="$(docker_embed_manifest_path "${template}")"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest}" >&2
    exit 1
  fi

  current="$(read_agent_toml_version "${manifest}")"
  if [ -z "${current}" ]; then
    next="0.1.0"
  else
    next="$(increment_version_last_segment "${current}")"
  fi

  if ! ref="$(image_ref_for_template "${template}" "${next}")"; then
    echo "no image ref for ${template}; set ACR_REGISTRY or EMBED_IMAGE_REF_*" >&2
    exit 1
  fi

  patch_agent_toml_version "${template}" "${next}"
  patch_agent_toml_ref "${template}" "${ref}"
  echo "bumped ${manifest} -> version=${next} ref=${ref}"
}

sync_agent_toml_version_and_ref() {
  local template="$1"
  local manifest version ref

  manifest="$(docker_embed_manifest_path "${template}")"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest}" >&2
    exit 1
  fi

  version="$(read_agent_toml_version "${manifest}")"
  if [ -z "${version}" ]; then
    version="0.1.0"
    patch_agent_toml_version "${template}" "${version}"
  fi

  if ! ref="$(image_ref_for_template "${template}" "${version}")"; then
    echo "no image ref for ${template}; set ACR_REGISTRY or EMBED_IMAGE_REF_*" >&2
    exit 1
  fi

  patch_agent_toml_ref "${template}" "${ref}"
  echo "synced ${manifest} -> version=${version} ref=${ref}"
}
