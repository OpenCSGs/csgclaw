# OpenClaw worker sandbox image: CSGClaw OpenClaw base + prebuilt csgclaw-cli.
#
# Build context must include bin/csgclaw-cli (see Makefile or .gitlab/ci-docker-embed-image.sh).
#
# Local build from repository root:
#   make build-openclaw-worker-image
#
# Manual build:
#   make stage-docker-embed-cli
#   docker build -f internal/templates/embed/openclaw-worker/Dockerfile \
#     --build-arg OPENCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260610.2-csgclaw \
#     -t openclaw-worker:local .

# Default upstream OpenClaw base image (keep in sync with openclaw-manager/Dockerfile).
ARG OPENCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260610.2-csgclaw

FROM ${OPENCLAW_IMAGE}

USER root

COPY bin/csgclaw-cli /usr/local/bin/csgclaw-cli
RUN chmod 755 /usr/local/bin/csgclaw-cli

USER node
