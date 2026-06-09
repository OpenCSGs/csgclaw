#!/usr/bin/env bash
# Detect embed agent.toml version changes on main for CI build gating.
# CI never modifies agent.toml — it reads version/image.ref and builds images with that tag.
# Writes picoclaw-build.env (GitLab dotenv report) for downstream jobs.
set -euo pipefail

: "${CI_COMMIT_SHA:?CI_COMMIT_SHA must be set}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${CI_PROJECT_DIR:-${ROOT}}/picoclaw-build.env"
MANAGER_TEMPLATE="picoclaw-manager"
WORKER_TEMPLATE="picoclaw-worker"
CURRENT_REF="HEAD"

read_agent_toml_field_at_ref() {
  local template="$1"
  local git_ref="$2"
  local field="$3"
  local path="internal/templates/embed/${template}/agent.toml"

  if ! git cat-file -e "${git_ref}:${path}" 2>/dev/null; then
    return 0
  fi

  git show "${git_ref}:${path}" | awk -v field="${field}" '
    $0 ~ "^" field "[[:space:]]*=" {
      value = $0
      sub(/^[^=]*=[[:space:]]*/, "", value)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^"/, "", value)
      gsub(/"$/, "", value)
      print value
      exit
    }
  '
}

read_agent_toml_version_at_ref() {
  read_agent_toml_field_at_ref "$1" "$2" "version"
}

read_agent_toml_image_ref_at_ref() {
  read_agent_toml_field_at_ref "$1" "$2" "ref"
}

image_ref_tag() {
  local ref="$1"
  printf '%s' "${ref##*:}"
}

find_previous_main_commit() {
  if git rev-parse "${CURRENT_REF}~1" >/dev/null 2>&1; then
    git rev-parse "${CURRENT_REF}~1"
    return 0
  fi
  return 1
}

version_changed() {
  local template="$1"
  local previous_ref="$2"
  local current previous

  current="$(read_agent_toml_version_at_ref "${template}" "${CURRENT_REF}")"
  if [ -z "${previous_ref}" ]; then
    if [ -n "${current}" ]; then
      return 0
    fi
    return 1
  fi

  previous="$(read_agent_toml_version_at_ref "${template}" "${previous_ref}")"
  if [ -z "${previous}" ]; then
    return 0
  fi
  if [ -z "${current}" ]; then
    echo "missing version in ${template}/agent.toml at ${CI_COMMIT_SHA}" >&2
    exit 1
  fi
  [ "${current}" != "${previous}" ]
}

validate_version_and_ref() {
  local template="$1"
  local version="$2"
  local ref ref_tag

  if [ -z "${version}" ]; then
    echo "missing version in ${template}/agent.toml at ${CI_COMMIT_SHA}" >&2
    exit 1
  fi

  ref="$(read_agent_toml_image_ref_at_ref "${template}" "${CURRENT_REF}")"
  if [ -z "${ref}" ]; then
    echo "missing image.ref in ${template}/agent.toml at ${CI_COMMIT_SHA}" >&2
    echo "bump version and image.ref locally with make build-all before merging to main" >&2
    exit 1
  fi

  ref_tag="$(image_ref_tag "${ref}")"
  if [ "${ref_tag}" != "${version}" ]; then
    echo "image.ref tag ${ref_tag} does not match version ${version} in ${template}/agent.toml" >&2
    echo "sync locally with make build-all before merging to main" >&2
    exit 1
  fi
}

previous_ref=""
if previous_ref="$(find_previous_main_commit)"; then
  echo "previous main commit: ${previous_ref}"
else
  echo "no previous commit found"
fi

manager_version="$(read_agent_toml_version_at_ref "${MANAGER_TEMPLATE}" "${CURRENT_REF}")"
worker_version="$(read_agent_toml_version_at_ref "${WORKER_TEMPLATE}" "${CURRENT_REF}")"

manager_build=false
worker_build=false
if version_changed "${MANAGER_TEMPLATE}" "${previous_ref:-}"; then
  manager_build=true
  validate_version_and_ref "${MANAGER_TEMPLATE}" "${manager_version}"
fi
if version_changed "${WORKER_TEMPLATE}" "${previous_ref:-}"; then
  worker_build=true
  validate_version_and_ref "${WORKER_TEMPLATE}" "${worker_version}"
fi

any_build=false
if [ "${manager_build}" = true ] || [ "${worker_build}" = true ]; then
  any_build=true
fi

{
  printf 'PICOCLAW_MANAGER_VERSION=%s\n' "${manager_version}"
  printf 'PICOCLAW_WORKER_VERSION=%s\n' "${worker_version}"
  printf 'PICOCLAW_MANAGER_BUILD=%s\n' "${manager_build}"
  printf 'PICOCLAW_WORKER_BUILD=%s\n' "${worker_build}"
  printf 'PICOCLAW_ANY_BUILD=%s\n' "${any_build}"
  if [ -n "${previous_ref}" ]; then
    printf 'PICOCLAW_PREVIOUS_COMMIT=%s\n' "${previous_ref}"
  fi
} > "${ENV_FILE}"

echo "picoclaw-manager version=${manager_version} build=${manager_build}"
echo "picoclaw-worker version=${worker_version} build=${worker_build}"
echo "picoclaw any_build=${any_build}"
cat "${ENV_FILE}"
