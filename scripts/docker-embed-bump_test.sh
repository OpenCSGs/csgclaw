#!/usr/bin/env bash
# Tests for conditional docker embed version bump (image inputs only).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMON="${ROOT}/scripts/docker-embed-agent-toml-common.sh"
TEMPLATE="test-embed"
ACR_REGISTRY="registry.example.com"

# shellcheck source=scripts/docker-embed-agent-toml-common.sh
. "${COMMON}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

assert_eq() {
  local got="$1" want="$2" msg="$3"
  if [ "${got}" != "${want}" ]; then
    fail "${msg}: got '${got}', want '${want}'"
  fi
}

setup_fixture() {
  FIXTURE="$(mktemp -d)"
  export DOCKER_EMBED_ROOT="${FIXTURE}"
  mkdir -p "${FIXTURE}/internal/templates/embed/${TEMPLATE}/workspace"
  mkdir -p "${FIXTURE}/cmd/csgclaw-cli"
  cat > "${FIXTURE}/internal/templates/embed/${TEMPLATE}/Dockerfile" <<'EOF'
FROM scratch
EOF
  cat > "${FIXTURE}/cmd/csgclaw-cli/main.go" <<'EOF'
package main
func main() {}
EOF
  git -C "${FIXTURE}" init -q
  git -C "${FIXTURE}" config user.email "test@example.com"
  git -C "${FIXTURE}" config user.name "Test"
}

write_agent_toml() {
  local version="$1"
  local ref="$2"
  cat > "${FIXTURE}/internal/templates/embed/${TEMPLATE}/agent.toml" <<EOF
name = "${TEMPLATE}"
description = "test"
role = "manager"
runtime_kind = "picoclaw_sandbox"
updated_at = "2026-01-01T00:00:00Z"
version = "${version}"

[image]
ref = "${ref}"
EOF
}

commit_all() {
  git -C "${FIXTURE}" add -A
  git -C "${FIXTURE}" commit -q -m "$1"
}

cleanup_fixture() {
  if [ -n "${FIXTURE:-}" ] && [ -d "${FIXTURE}" ]; then
    rm -rf "${FIXTURE}"
  fi
}

run_bump() {
  ACR_REGISTRY="${ACR_REGISTRY}" bump_agent_toml_version_and_ref "${TEMPLATE}"
}

test_no_changes_skips_bump() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.3" \
    "clean workspace should not bump"
  cleanup_fixture
}

test_workspace_change_skips_bump() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  echo "change" > "${FIXTURE}/internal/templates/embed/${TEMPLATE}/workspace/NOTE.md"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.3" \
    "workspace-only change should not bump image version"
  cleanup_fixture
}

test_dockerfile_change_bumps_once() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  echo "# tweak" >> "${FIXTURE}/internal/templates/embed/${TEMPLATE}/Dockerfile"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "first bump with Dockerfile change"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "second bump should stay at 0.1.4"
  cleanup_fixture
}

test_commit_then_dockerfile_change_bumps_again() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  echo "# tweak" >> "${FIXTURE}/internal/templates/embed/${TEMPLATE}/Dockerfile"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "initial bump"
  commit_all "bumped"
  echo "# more" >> "${FIXTURE}/internal/templates/embed/${TEMPLATE}/Dockerfile"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.5" \
    "bump after commit and new Dockerfile change"
  cleanup_fixture
}

test_version_ref_only_change_skips_bump() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  write_agent_toml "0.1.4" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.4"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "version/ref-only change should not bump again"
  cleanup_fixture
}

test_cli_change_bumps_once() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  echo "// change" >> "${FIXTURE}/cmd/csgclaw-cli/main.go"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "cli change should bump"
  run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "second cli bump should stay at 0.1.4"
  cleanup_fixture
}

test_force_bump() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3"
  commit_all "baseline"
  DOCKER_EMBED_FORCE_BUMP=1 run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.4" \
    "force bump on clean tree"
  DOCKER_EMBED_FORCE_BUMP=1 run_bump
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.5" \
    "force bump again"
  cleanup_fixture
}

test_no_changes_syncs_ref() {
  setup_fixture
  write_agent_toml "0.1.3" "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:wrong-tag"
  commit_all "baseline"
  run_bump
  assert_eq "$(read_agent_toml_image_ref "$(docker_embed_manifest_path "${TEMPLATE}")")" \
    "${ACR_REGISTRY}/opencsghq/${TEMPLATE}:0.1.3" \
    "out-of-sync ref should be synced without bump"
  assert_eq "$(read_agent_toml_version "$(docker_embed_manifest_path "${TEMPLATE}")")" "0.1.3" \
    "version unchanged during sync"
  cleanup_fixture
}

main() {
  test_no_changes_skips_bump
  test_workspace_change_skips_bump
  test_dockerfile_change_bumps_once
  test_commit_then_dockerfile_change_bumps_again
  test_version_ref_only_change_skips_bump
  test_cli_change_bumps_once
  test_force_bump
  test_no_changes_syncs_ref
  echo "OK: docker embed bump tests passed"
}

main "$@"
