#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: ci-picoclaw-image.sh <goarch> <dockerfile> <image-name>" >&2
  exit 1
fi

goarch="$1"
dockerfile="$2"
image_name="$3"

: "${CI_PROJECT_DIR:?CI_PROJECT_DIR must be set}"
: "${CI_COMMIT_TAG:?CI_COMMIT_TAG must be set}"
: "${ACR_REGISTRY:?ACR_REGISTRY must be set}"

archive="${CI_PROJECT_DIR}/dist/csgclaw-cli_${CI_COMMIT_TAG}_linux_${goarch}.tar.gz"
staging_dir="${CI_PROJECT_DIR}/bin"
cli_path="${staging_dir}/csgclaw-cli"

if [ ! -f "${archive}" ]; then
  echo "missing release artifact: ${archive}" >&2
  echo "picoclaw image builds reuse csgclaw-cli from release-build (scripts/release-build-all.sh)" >&2
  exit 1
fi

mkdir -p "${staging_dir}"
rm -f "${cli_path}"
tar -xzf "${archive}" -C "${staging_dir}" csgclaw-cli
chmod 755 "${cli_path}"

if [ ! -f "${cli_path}" ]; then
  echo "failed to stage ${cli_path} from ${archive}" >&2
  exit 1
fi

export PICOCLAW_BASE_TAG="${PICOCLAW_BASE_TAG:-2026.5.27}"

/kaniko/executor \
  --context "${CI_PROJECT_DIR}" \
  --dockerfile "${CI_PROJECT_DIR}/${dockerfile}" \
  --custom-platform "linux/${goarch}" \
  --destination "${ACR_REGISTRY}/opencsghq/${image_name}:${CI_COMMIT_TAG}-${goarch}" \
  --build-arg PICOCLAW_IMAGE="${ACR_REGISTRY}/opencsghq/picoclaw:${PICOCLAW_BASE_TAG}"
