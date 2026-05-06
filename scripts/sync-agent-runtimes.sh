#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
src="${repo_root}/runtimes"
dst="${repo_root}/internal/agent/embed/runtimes"

if [ ! -d "${src}/picoclaw/manager/workspace" ]; then
  echo "missing PicoClaw manager workspace: ${src}/picoclaw/manager/workspace" >&2
  exit 1
fi
if [ ! -d "${src}/picoclaw/worker/workspace" ]; then
  echo "missing PicoClaw worker workspace: ${src}/picoclaw/worker/workspace" >&2
  exit 1
fi
if [ ! -d "${src}/openclaw/worker/workspace" ]; then
  echo "missing OpenClaw worker workspace: ${src}/openclaw/worker/workspace" >&2
  exit 1
fi

rm -rf "${dst}"
mkdir -p "$(dirname "${dst}")"
cp -R "${src}" "${dst}"
