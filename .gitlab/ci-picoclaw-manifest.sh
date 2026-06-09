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
: "${CI_PROJECT_DIR:?CI_PROJECT_DIR must be set}"

manifest="${CI_PROJECT_DIR}/internal/templates/embed/${image_name}/agent.toml"
if [ ! -f "${manifest}" ]; then
  echo "missing manifest: ${manifest}" >&2
  exit 1
fi

PICOCLAW_IMAGE_VERSION="$(awk -F= '
  /^version[[:space:]]*=[[:space:]]*/ {
    value = $2
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
    gsub(/^"/, "", value)
    gsub(/"$/, "", value)
    print value
    exit
  }
' "${manifest}")"

if [ -z "${PICOCLAW_IMAGE_VERSION}" ]; then
  echo "missing version in ${manifest}" >&2
  exit 1
fi

repo="${ACR_REGISTRY}/opencsghq/${image_name}"
amd64_ref="${repo}:${PICOCLAW_IMAGE_VERSION}-amd64"
arm64_ref="${repo}:${PICOCLAW_IMAGE_VERSION}-arm64"
target_ref="${repo}:${PICOCLAW_IMAGE_VERSION}"

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
