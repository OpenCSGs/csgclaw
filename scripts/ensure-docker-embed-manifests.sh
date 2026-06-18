#!/usr/bin/env bash
# Ensure docker embed agent.toml image refs match their version fields.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECK_SCRIPT="${CHECK_DOCKER_EMBED_MANIFESTS_SCRIPT:-${ROOT}/scripts/check-docker-embed-manifests.sh}"
SYNC_SCRIPT="${SYNC_DOCKER_EMBED_IMAGE_REFS_SCRIPT:-${ROOT}/scripts/sync-docker-embed-image-refs.sh}"
CHECK_RETRIES="${DOCKER_EMBED_CHECK_RETRIES:-1}"
SYNC_RETRIES="${DOCKER_EMBED_SYNC_RETRIES:-${CHECK_RETRIES}}"

check_args=()
sync_args=()
if [ -n "${CHECK_DOCKER_EMBED_MANIFESTS_ARGS:-}" ]; then
  read -r -a check_args <<< "${CHECK_DOCKER_EMBED_MANIFESTS_ARGS}"
fi
if [ -n "${SYNC_DOCKER_EMBED_IMAGE_REFS_ARGS:-}" ]; then
  read -r -a sync_args <<< "${SYNC_DOCKER_EMBED_IMAGE_REFS_ARGS}"
fi

nonnegative_integer_or_default() {
  local value="$1"
  local fallback="$2"
  if [[ "${value}" =~ ^[0-9]+$ ]]; then
    printf '%s' "${value}"
    return
  fi
  printf '%s' "${fallback}"
}

run_with_sigkill_retry() {
  local label="$1"
  local retries="$2"
  shift 2

  retries="$(nonnegative_integer_or_default "${retries}" "1")"
  local attempt=0
  local status=0
  while true; do
    "$@"
    status=$?
    if [ "${status}" -eq 0 ]; then
      return 0
    fi
    if [ "${status}" -eq 137 ] && [ "${attempt}" -lt "${retries}" ]; then
      attempt=$((attempt + 1))
      printf '%s\n' "${label} was killed by SIGKILL; retrying (${attempt}/${retries})" >&2
      sleep 1
      continue
    fi
    return "${status}"
  done
}

run_with_sigkill_retry "docker embed manifest check" "${CHECK_RETRIES}" "${CHECK_SCRIPT}" "${check_args[@]}"
status=$?
if [ "${status}" -eq 0 ]; then
  exit 0
fi
if [ "${status}" -ne 1 ]; then
  printf '%s\n' "docker embed manifest check failed with exit ${status}; not syncing image refs" >&2
  exit "${status}"
fi

printf '%s\n' "docker embed agent.toml version/ref out of sync; running sync-docker-embed-image-refs"
run_with_sigkill_retry "docker embed manifest sync" "${SYNC_RETRIES}" "${SYNC_SCRIPT}" "${sync_args[@]}"
exit $?
