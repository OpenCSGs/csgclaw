#!/usr/bin/env bash
# Write [image].ref into staged dist/agent.toml for docker embed templates.
# Discovers templates via Dockerfile; default ref:
#   ${ACR_REGISTRY}/opencsghq/<template-name>:${CI_COMMIT_TAG|VERSION|dev}
#
# Per-template override (optional):
#   EMBED_IMAGE_REF_picoclaw_manager=registry.example/custom:tag
#   (template dir picoclaw-manager -> EMBED_IMAGE_REF_picoclaw_manager)
#
# Legacy overrides (still supported):
#   PICOCLAW_MANAGER_IMAGE_REF / PICOCLAW_WORKER_IMAGE_REF
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"

# Escape a value for a TOML basic string (double-quoted).
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

patch_agent_toml() {
  local template="$1"
  local image_ref="$2"
  local manifest="${ROOT}/internal/templates/embed/${template}/dist/agent.toml"
  local escaped_ref

  if [ -z "${image_ref}" ]; then
    return 0
  fi
  validate_image_ref "${image_ref}"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest} (run scripts/prepare-docker-embed-dist.sh first)" >&2
    exit 1
  fi

  escaped_ref="$(toml_escape_basic_string "${image_ref}")"
  export PICOCLAW_IMAGE_REF="${escaped_ref}"

  awk '
    BEGIN {
      ref = ENVIRON["PICOCLAW_IMAGE_REF"]
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

  unset PICOCLAW_IMAGE_REF
  mv "${manifest}.tmp" "${manifest}"
  echo "patched ${manifest} -> ${image_ref}"
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
  esac
  return 1
}

image_ref_for_template() {
  local template="$1"
  local key ref tag

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
    tag="${CI_COMMIT_TAG:-${VERSION:-dev}}"
    printf '%s' "${ACR_REGISTRY}/opencsghq/${template}:${tag}"
    return 0
  fi

  return 1
}

chmod +x "${LIST_SCRIPT}"
templates=()
while IFS= read -r name; do
  [ -n "${name}" ] && templates+=("${name}")
done < <("${LIST_SCRIPT}")

patched=0
for template in "${templates[@]}"; do
  if ! ref="$(image_ref_for_template "${template}")"; then
    echo "no image ref for ${template}; set ACR_REGISTRY or EMBED_IMAGE_REF_*" >&2
    exit 1
  fi
  if [ -n "${ref}" ]; then
    patch_agent_toml "${template}" "${ref}"
    patched=1
  fi
done

if [ "${patched}" -eq 0 ]; then
  echo "no docker embed templates to patch" >&2
  exit 1
fi
