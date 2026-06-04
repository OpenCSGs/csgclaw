#!/usr/bin/env bash
# Write [image].ref into staged PicoClaw dist/agent.toml manifests.
#
# Usage:
#   PICOCLAW_MANAGER_IMAGE_REF=registry/picoclaw-manager:v1.0.0 \
#   PICOCLAW_WORKER_IMAGE_REF=registry/picoclaw-worker:v1.0.0 \
#     ./scripts/patch-picoclaw-embed-image-refs.sh
#
# CI / make defaults (when explicit refs are unset):
#   ${ACR_REGISTRY}/opencsghq/picoclaw-{manager,worker}:${CI_COMMIT_TAG or VERSION or dev}
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

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

  # Avoid [[ glob ]] with $'\0' (unreliable on macOS /bin/bash 3.2).
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
    echo "missing image ref for ${template}" >&2
    exit 1
  fi
  validate_image_ref "${image_ref}"
  if [ ! -f "${manifest}" ]; then
    echo "missing manifest: ${manifest} (run scripts/prepare-picoclaw-embed-dist.sh first)" >&2
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

manager_ref=""
worker_ref=""

if [ -n "${PICOCLAW_MANAGER_IMAGE_REF+x}" ]; then
  manager_ref="${PICOCLAW_MANAGER_IMAGE_REF}"
elif [ -n "${ACR_REGISTRY:-}" ]; then
  tag="${CI_COMMIT_TAG:-${VERSION:-dev}}"
  manager_ref="${ACR_REGISTRY}/opencsghq/picoclaw-manager:${tag}"
fi

if [ -n "${PICOCLAW_WORKER_IMAGE_REF+x}" ]; then
  worker_ref="${PICOCLAW_WORKER_IMAGE_REF}"
elif [ -n "${ACR_REGISTRY:-}" ]; then
  tag="${CI_COMMIT_TAG:-${VERSION:-dev}}"
  worker_ref="${ACR_REGISTRY}/opencsghq/picoclaw-worker:${tag}"
fi

if [ -n "${manager_ref}" ]; then
  patch_agent_toml picoclaw-manager "${manager_ref}"
fi
if [ -n "${worker_ref}" ]; then
  patch_agent_toml picoclaw-worker "${worker_ref}"
fi
if [ -z "${manager_ref}" ] && [ -z "${worker_ref}" ]; then
  echo "no PicoClaw image refs to patch; set PICOCLAW_*_IMAGE_REF or ACR_REGISTRY" >&2
  exit 1
fi
