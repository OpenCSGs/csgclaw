#!/usr/bin/env bash
# Detect docker embed agent.toml changes on main for CI build gating.
# Compares the pushed range (CI_COMMIT_BEFORE_SHA..HEAD), not just HEAD~1.
# CI never modifies agent.toml - it reads version/image.ref and builds images with that tag.
# Writes docker-embed-build.env (GitLab dotenv report) for downstream jobs.
set -euo pipefail

: "${CI_COMMIT_SHA:?CI_COMMIT_SHA must be set}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${CI_PROJECT_DIR:-${ROOT}}/docker-embed-build.env"
LIST_SCRIPT="${ROOT}/scripts/list-docker-embed-templates.sh"
CURRENT_REF="HEAD"
EMPTY_SHA="0000000000000000000000000000000000000000"

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

agent_toml_path() {
  local template="$1"
  printf 'internal/templates/embed/%s/agent.toml' "${template}"
}

find_compare_base() {
  local before_sha="${CI_COMMIT_BEFORE_SHA:-}"

  if [ -n "${before_sha}" ] && [ "${before_sha}" != "${EMPTY_SHA}" ]; then
    if git cat-file -e "${before_sha}^{commit}" 2>/dev/null; then
      printf '%s' "${before_sha}"
      return 0
    fi
    echo "CI_COMMIT_BEFORE_SHA ${before_sha} is not reachable; falling back to HEAD~1" >&2
  fi

  if git rev-parse "${CURRENT_REF}~1" >/dev/null 2>&1; then
    git rev-parse "${CURRENT_REF}~1"
    return 0
  fi
  return 1
}

manifest_changed_in_push() {
  local template="$1"
  local compare_base="$2"
  local path

  path="$(agent_toml_path "${template}")"
  if [ -z "${compare_base}" ]; then
    return 0
  fi

  if ! git cat-file -e "${compare_base}^{commit}" 2>/dev/null; then
    return 0
  fi

  if git diff --name-only "${compare_base}" "${CURRENT_REF}" -- "${path}" | grep -q .; then
    return 0
  fi
  return 1
}

version_changed_since_base() {
  local template="$1"
  local compare_base="$2"
  local current previous

  current="$(read_agent_toml_version_at_ref "${template}" "${CURRENT_REF}")"
  if [ -z "${compare_base}" ]; then
    if [ -n "${current}" ]; then
      return 0
    fi
    return 1
  fi

  previous="$(read_agent_toml_version_at_ref "${template}" "${compare_base}")"
  if [ -z "${previous}" ]; then
    return 0
  fi
  if [ -z "${current}" ]; then
    echo "missing version in ${template}/agent.toml at ${CI_COMMIT_SHA}" >&2
    exit 1
  fi
  [ "${current}" != "${previous}" ]
}

should_build_template() {
  local template="$1"
  local compare_base="$2"

  if manifest_changed_in_push "${template}" "${compare_base}"; then
    return 0
  fi
  version_changed_since_base "${template}" "${compare_base}"
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

env_prefix_for_template() {
  local template="$1"
  template="${template//-/_}"
  printf '%s' "${template^^}"
}

chmod +x "${LIST_SCRIPT}"
templates=()
while IFS= read -r name; do
  [ -n "${name}" ] && templates+=("${name}")
done < <("${LIST_SCRIPT}")

if [ "${#templates[@]}" -eq 0 ]; then
  echo "no docker embed templates found" >&2
  exit 1
fi

compare_base=""
if compare_base="$(find_compare_base)"; then
  echo "compare base: ${compare_base} (range ${compare_base}..${CURRENT_REF})"
else
  echo "no compare base found; treating embed manifests as new"
fi

declare -A versions
declare -A builds
any_build=false
for template in "${templates[@]}"; do
  version="$(read_agent_toml_version_at_ref "${template}" "${CURRENT_REF}")"
  build=false
  if should_build_template "${template}" "${compare_base:-}"; then
    build=true
    validate_version_and_ref "${template}" "${version}"
  fi
  versions["${template}"]="${version}"
  builds["${template}"]="${build}"
  if [ "${build}" = true ]; then
    any_build=true
  fi
done

{
  for template in "${templates[@]}"; do
    prefix="$(env_prefix_for_template "${template}")"
    printf '%s_VERSION=%s\n' "${prefix}" "${versions[${template}]}"
    printf '%s_BUILD=%s\n' "${prefix}" "${builds[${template}]}"
  done
  printf 'DOCKER_EMBED_ANY_BUILD=%s\n' "${any_build}"
  # Compatibility for existing CI job names and older downstream references.
  printf 'PICOCLAW_ANY_BUILD=%s\n' "${any_build}"
  if [ -n "${compare_base}" ]; then
    printf 'DOCKER_EMBED_COMPARE_BASE=%s\n' "${compare_base}"
    printf 'DOCKER_EMBED_PREVIOUS_COMMIT=%s\n' "${compare_base}"
    printf 'PICOCLAW_COMPARE_BASE=%s\n' "${compare_base}"
    printf 'PICOCLAW_PREVIOUS_COMMIT=%s\n' "${compare_base}"
  fi
} > "${ENV_FILE}"

for template in "${templates[@]}"; do
  echo "${template} version=${versions[${template}]} build=${builds[${template}]}"
done
echo "docker embed any_build=${any_build}"
cat "${ENV_FILE}"
