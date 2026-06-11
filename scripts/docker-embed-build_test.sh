#!/usr/bin/env bash
# Tests for selective docker embed image build selection.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMMON="${ROOT}/scripts/docker-embed-agent-toml-common.sh"
TEMPLATE="test-embed"
OTHER="other-embed"

# shellcheck source=scripts/docker-embed-agent-toml-common.sh
. "${COMMON}"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

setup_fixture() {
  FIXTURE="$(mktemp -d)"
  export DOCKER_EMBED_ROOT="${FIXTURE}"
  mkdir -p "${FIXTURE}/internal/templates/embed/${TEMPLATE}"
  mkdir -p "${FIXTURE}/internal/templates/embed/${OTHER}"
  mkdir -p "${FIXTURE}/cmd/csgclaw-cli"
  echo 'FROM scratch' > "${FIXTURE}/internal/templates/embed/${TEMPLATE}/Dockerfile"
  echo 'FROM scratch' > "${FIXTURE}/internal/templates/embed/${OTHER}/Dockerfile"
  echo 'package main; func main() {}' > "${FIXTURE}/cmd/csgclaw-cli/main.go"
  git -C "${FIXTURE}" init -q
  git -C "${FIXTURE}" config user.email "test@example.com"
  git -C "${FIXTURE}" config user.name "Test"
  git -C "${FIXTURE}" add -A
  git -C "${FIXTURE}" commit -q -m "baseline"
}

cleanup_fixture() {
  if [ -n "${FIXTURE:-}" ] && [ -d "${FIXTURE}" ]; then
    rm -rf "${FIXTURE}"
  fi
}

assert_build() {
  local template="$1" want="$2" msg="$3"
  local got=no
  if should_build_docker_embed_image "${template}"; then
    got=yes
  fi
  if [ "${got}" != "${want}" ]; then
    fail "${msg}: got '${got}', want '${want}'"
  fi
}

test_clean_tree_skips_build() {
  setup_fixture
  assert_build "${TEMPLATE}" no "clean tree"
  assert_build "${OTHER}" no "clean tree other template"
  cleanup_fixture
}

test_dockerfile_change_builds_one() {
  setup_fixture
  echo '# tweak' >> "${FIXTURE}/internal/templates/embed/${TEMPLATE}/Dockerfile"
  assert_build "${TEMPLATE}" yes "dockerfile changed"
  assert_build "${OTHER}" no "other template unchanged"
  cleanup_fixture
}

test_cli_change_builds_all() {
  setup_fixture
  echo '// tweak' >> "${FIXTURE}/cmd/csgclaw-cli/main.go"
  assert_build "${TEMPLATE}" yes "cli changed"
  assert_build "${OTHER}" yes "cli changed other template"
  cleanup_fixture
}

test_workspace_change_skips_build() {
  setup_fixture
  mkdir -p "${FIXTURE}/internal/templates/embed/${TEMPLATE}/workspace"
  echo 'note' > "${FIXTURE}/internal/templates/embed/${TEMPLATE}/workspace/NOTE.md"
  assert_build "${TEMPLATE}" no "workspace only"
  cleanup_fixture
}

test_force_build() {
  setup_fixture
  export DOCKER_EMBED_FORCE_BUILD=1
  assert_build "${TEMPLATE}" yes "force build"
  unset DOCKER_EMBED_FORCE_BUILD
  cleanup_fixture
}

main() {
  test_clean_tree_skips_build
  test_dockerfile_change_builds_one
  test_cli_change_builds_all
  test_workspace_change_skips_build
  test_force_build
  echo "OK: docker embed build selection tests passed"
}

main "$@"
