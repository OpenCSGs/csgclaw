#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: ci-picoclaw-manifest.sh <image-name>" >&2
  exit 1
fi

image_name="$1"

: "${ACR_REGISTRY:?ACR_REGISTRY must be set}"
: "${ACR_USERNAME:?ACR_USERNAME must be set}"
: "${ACR_PASSWORD:?ACR_PASSWORD must be set}"
: "${CI_COMMIT_TAG:?CI_COMMIT_TAG must be set}"

repo="${ACR_REGISTRY}/opencsghq/${image_name}"
amd64_ref="${repo}:${CI_COMMIT_TAG}-amd64"
arm64_ref="${repo}:${CI_COMMIT_TAG}-arm64"
target_ref="${repo}:${CI_COMMIT_TAG}"

crane auth login "$ACR_REGISTRY" -u "$ACR_USERNAME" -p "$ACR_PASSWORD"

for ref in "$amd64_ref" "$arm64_ref"; do
  if ! crane manifest "$ref" >/dev/null; then
    echo "missing pushed image manifest: ${ref}" >&2
    exit 1
  fi
done

crane index append \
  -m "$amd64_ref" \
  -m "$arm64_ref" \
  -t "$target_ref"
