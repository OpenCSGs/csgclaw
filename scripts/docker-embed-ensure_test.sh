#!/usr/bin/env bash
# Tests for docker embed manifest ensure control flow.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENSURE_SCRIPT="${ROOT}/scripts/ensure-docker-embed-manifests.sh"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1"
  local want="$2"
  local msg="$3"
  if [ "${got}" != "${want}" ]; then
    fail "${msg}: got '${got}', want '${want}'"
  fi
}

setup_fixture() {
  FIXTURE="$(mktemp -d)"
  CHECK_LOG="${FIXTURE}/check.log"
  SYNC_LOG="${FIXTURE}/sync.log"
  CHECK_SEQUENCE_FILE="${FIXTURE}/check-sequence"
  CHECK_SCRIPT="${FIXTURE}/check.sh"
  SYNC_SCRIPT="${FIXTURE}/sync.sh"

  cat > "${CHECK_SCRIPT}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
log="$1"
sequence_file="$2"
IFS=, read -r status rest < "${sequence_file}"
printf '%s\n' "${status}" >> "${log}"
if [ -n "${rest:-}" ]; then
  printf '%s\n' "${rest}" > "${sequence_file}"
else
  printf '%s\n' "${status}" > "${sequence_file}"
fi
exit "${status}"
EOF

  cat > "${SYNC_SCRIPT}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'sync\n' >> "$1"
EOF

  chmod +x "${CHECK_SCRIPT}" "${SYNC_SCRIPT}"
}

cleanup_fixture() {
  if [ -n "${FIXTURE:-}" ] && [ -d "${FIXTURE}" ]; then
    rm -rf "${FIXTURE}"
  fi
}

run_ensure() {
  DOCKER_EMBED_CHECK_RETRIES=1 \
    CHECK_DOCKER_EMBED_MANIFESTS_SCRIPT="${CHECK_SCRIPT}" \
    CHECK_DOCKER_EMBED_MANIFESTS_ARGS="${CHECK_LOG} ${CHECK_SEQUENCE_FILE}" \
    SYNC_DOCKER_EMBED_IMAGE_REFS_SCRIPT="${SYNC_SCRIPT}" \
    SYNC_DOCKER_EMBED_IMAGE_REFS_ARGS="${SYNC_LOG}" \
    bash "${ENSURE_SCRIPT}" >/dev/null
}

run_ensure_capture_status() {
  set +e
  run_ensure
  local status=$?
  set -e
  printf '%s' "${status}"
}

write_check_sequence() {
  printf '%s\n' "$1" > "${CHECK_SEQUENCE_FILE}"
}

log_lines() {
  local path="$1"
  if [ ! -f "${path}" ]; then
    printf '%s' "0"
    return
  fi
  wc -l < "${path}" | tr -d '[:space:]'
}

test_current_manifest_skips_sync() {
  setup_fixture
  write_check_sequence "0"
  run_ensure
  assert_eq "$(log_lines "${CHECK_LOG}")" "1" "current manifest should run check once"
  assert_eq "$(log_lines "${SYNC_LOG}")" "0" "current manifest should not sync"
  cleanup_fixture
}

test_out_of_sync_runs_sync() {
  setup_fixture
  write_check_sequence "1"
  run_ensure
  assert_eq "$(log_lines "${CHECK_LOG}")" "1" "out-of-sync manifest should run check once"
  assert_eq "$(log_lines "${SYNC_LOG}")" "1" "out-of-sync manifest should sync"
  cleanup_fixture
}

test_killed_check_retries_without_sync_when_retry_is_current() {
  setup_fixture
  write_check_sequence "137,0"
  run_ensure
  assert_eq "$(log_lines "${CHECK_LOG}")" "2" "killed check should retry once"
  assert_eq "$(log_lines "${SYNC_LOG}")" "0" "killed then current check should not sync"
  cleanup_fixture
}

test_killed_check_retries_then_syncs_when_retry_is_out_of_sync() {
  setup_fixture
  write_check_sequence "137,1"
  run_ensure
  assert_eq "$(log_lines "${CHECK_LOG}")" "2" "killed check should retry before sync"
  assert_eq "$(log_lines "${SYNC_LOG}")" "1" "out-of-sync retry should sync"
  cleanup_fixture
}

test_repeated_killed_check_fails_without_sync() {
  setup_fixture
  write_check_sequence "137,137"
  status="$(run_ensure_capture_status)"
  assert_eq "${status}" "137" "repeated killed check should fail with signal status"
  assert_eq "$(log_lines "${CHECK_LOG}")" "2" "repeated killed check should honor retry limit"
  assert_eq "$(log_lines "${SYNC_LOG}")" "0" "repeated killed check should not sync"
  cleanup_fixture
}

main() {
  test_current_manifest_skips_sync
  test_out_of_sync_runs_sync
  test_killed_check_retries_without_sync_when_retry_is_current
  test_killed_check_retries_then_syncs_when_retry_is_out_of_sync
  test_repeated_killed_check_fails_without_sync
  echo "OK: docker embed ensure tests passed"
}

main "$@"
