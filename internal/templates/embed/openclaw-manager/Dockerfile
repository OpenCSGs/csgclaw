# OpenClaw manager sandbox image: CSGClaw OpenClaw base + prebuilt csgclaw-cli.
#
# Build context must include bin/csgclaw-cli (see Makefile or .gitlab/ci-docker-embed-image.sh).
#
# Local build from repository root:
#   make build-openclaw-manager-image
#
# Manual build:
#   make stage-docker-embed-cli
#   docker build -f internal/templates/embed/openclaw-manager/Dockerfile \
#     --build-arg OPENCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260610.2-csgclaw \
#     -t openclaw-manager:local .

# Default upstream OpenClaw base image (single source of truth for local make / CI builds).
ARG OPENCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/openclaw:20260610.2-csgclaw

FROM ${OPENCLAW_IMAGE}

USER root

COPY bin/csgclaw-cli /usr/local/bin/csgclaw-cli
RUN chmod 755 /usr/local/bin/csgclaw-cli

USER node
