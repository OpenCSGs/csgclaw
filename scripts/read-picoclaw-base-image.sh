#!/usr/bin/env bash
# Print upstream picoclaw base image ref from embed Dockerfile ARG default.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE="${ROOT}/internal/templates/embed/picoclaw-manager/Dockerfile"

if [ ! -f "${DOCKERFILE}" ]; then
  echo "missing Dockerfile: ${DOCKERFILE}" >&2
  exit 1
fi

ref="$(grep '^ARG PICOCLAW_IMAGE=' "${DOCKERFILE}" | head -1 | sed 's/^ARG PICOCLAW_IMAGE=//' | tr -d ' \r')"
if [ -z "${ref}" ]; then
  echo "missing ARG PICOCLAW_IMAGE in ${DOCKERFILE}" >&2
  exit 1
fi

printf '%s' "${ref}"
