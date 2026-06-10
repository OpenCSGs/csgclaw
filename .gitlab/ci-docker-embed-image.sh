#!/bin/sh
set -eu

if [ "$#" -ne 3 ]; then
  echo "usage: ci-docker-embed-image.sh <goarch> <dockerfile> <image-name>" >&2
  exit 1
fi

goarch="$1"
dockerfile="$2"
image_name="$3"

: "${CI_PROJECT_DIR:?CI_PROJECT_DIR must be set}"
: "${ACR_REGISTRY:?ACR_REGISTRY must be set}"

manifest="${CI_PROJECT_DIR}/internal/templates/embed/${image_name}/agent.toml"
if [ ! -f "${manifest}" ]; then
  echo "missing manifest: ${manifest}" >&2
  exit 1
fi

EMBED_IMAGE_VERSION="$(awk -F= '
  /^version[[:space:]]*=[[:space:]]*/ {
    value = $2
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
    gsub(/^"/, "", value)
    gsub(/"$/, "", value)
    print value
    exit
  }
' "${manifest}")"

if [ -z "${EMBED_IMAGE_VERSION}" ]; then
  echo "missing version in ${manifest}" >&2
  exit 1
fi

DOCKER_EMBED_CLI_VERSION="${DOCKER_EMBED_CLI_VERSION:-${PICOCLAW_CLI_VERSION:-${CI_COMMIT_SHORT_SHA:-unknown}}}"
archive="${CI_PROJECT_DIR}/dist/csgclaw-cli_${DOCKER_EMBED_CLI_VERSION}_linux_${goarch}.tar.gz"
staging_dir="${CI_PROJECT_DIR}/bin"
cli_path="${staging_dir}/csgclaw-cli"

if [ ! -f "${archive}" ]; then
  echo "missing release artifact: ${archive}" >&2
  echo "docker embed image builds reuse csgclaw-cli from docker-embed-cli-build (scripts/release-build-all.sh)" >&2
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

set -- /kaniko/executor \
  --context "${CI_PROJECT_DIR}" \
  --dockerfile "${CI_PROJECT_DIR}/${dockerfile}" \
  --custom-platform "linux/${goarch}" \
  --destination "${ACR_REGISTRY}/opencsghq/${image_name}:${EMBED_IMAGE_VERSION}-${goarch}"

case "${image_name}" in
  picoclaw-*)
    if [ -n "${PICOCLAW_BASE_IMAGE:-}" ]; then
      set -- "$@" --build-arg "PICOCLAW_IMAGE=${PICOCLAW_BASE_IMAGE}"
      echo "using PICOCLAW_IMAGE override: ${PICOCLAW_BASE_IMAGE}"
    else
      echo "using PICOCLAW_IMAGE from Dockerfile default"
    fi
    ;;
  openclaw-*)
    if [ -n "${OPENCLAW_BASE_IMAGE:-}" ]; then
      set -- "$@" --build-arg "OPENCLAW_IMAGE=${OPENCLAW_BASE_IMAGE}"
      echo "using OPENCLAW_IMAGE override: ${OPENCLAW_BASE_IMAGE}"
    else
      echo "using OPENCLAW_IMAGE from Dockerfile default"
    fi
    ;;
  *)
    echo "unknown docker embed image ${image_name}" >&2
    exit 1
    ;;
esac

exec "$@"
