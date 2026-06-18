#!/usr/bin/env sh
# Resolve a version string for local make builds (tags preferred).
set -eu

if git rev-parse --git-dir >/dev/null 2>&1; then
  if tag="$(git describe --tags --exact-match 2>/dev/null)"; then
    printf '%s+local\n' "$tag"
    exit 0
  fi
  if tag="$(git describe --tags --always --dirty 2>/dev/null)"; then
    printf '%s+local\n' "$tag"
    exit 0
  fi
fi

printf 'dev+local\n'
