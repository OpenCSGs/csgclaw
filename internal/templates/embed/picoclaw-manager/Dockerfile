# PicoClaw manager sandbox image: upstream picoclaw base + prebuilt csgclaw-cli.
#
# Build context must include bin/csgclaw-cli (see Makefile or .gitlab/ci-picoclaw-image.sh).
#
# Local build from repository root:
#   make build-picoclaw-manager-image
#
# Manual build:
#   make stage-docker-embed-cli
#   docker build -f internal/templates/embed/picoclaw-manager/Dockerfile \
#     --build-arg PICOCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.27 \
#     -t picoclaw-manager:local .

ARG PICOCLAW_IMAGE=opencsg-registry.cn-beijing.cr.aliyuncs.com/opencsghq/picoclaw:2026.5.27

FROM ${PICOCLAW_IMAGE}

USER root

COPY bin/csgclaw-cli /usr/local/bin/csgclaw-cli
RUN chmod 755 /usr/local/bin/csgclaw-cli

USER picoclaw
