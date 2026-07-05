ARG GO_IMAGE=golang:1.26.2-alpine
ARG RUNTIME_IMAGE=node:22.13.0-alpine
ARG ALPINE_MIRROR=https://mirrors.aliyun.com/alpine

FROM ${GO_IMAGE} AS gateway-build

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ARG ALPINE_MIRROR
ENV GOPROXY=${GOPROXY}

RUN if [ -n "${ALPINE_MIRROR}" ]; then \
      sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories; \
    fi && \
    apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" \
      -o /out/csgclaw-codex-gateway ./cmd/csgclaw-codex-gateway

FROM ${RUNTIME_IMAGE}

ARG LARK_CODING_AGENT_BRIDGE_PACKAGE=lark-channel-bridge@0.4.1
ARG CODEX_PACKAGE=@openai/codex@0.142.5
ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG ALPINE_MIRROR

RUN if [ -n "${ALPINE_MIRROR}" ]; then \
      sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${ALPINE_MIRROR}|g" /etc/apk/repositories; \
    fi && \
    apk add --no-cache \
      bash \
      ca-certificates \
      git \
      openssh-client \
      tini \
      wget && \
    npm config set registry ${NPM_REGISTRY} && \
    npm install -g ${LARK_CODING_AGENT_BRIDGE_PACKAGE} ${CODEX_PACKAGE} && \
    npm cache clean --force && \
    command -v lark-channel-bridge && \
    command -v codex

COPY --from=gateway-build /out/csgclaw-codex-gateway /usr/local/bin/csgclaw-codex-gateway

RUN cat > /usr/local/bin/csgclaw-codex-entrypoint <<'EOF'
#!/bin/sh
set -eu
if [ "$#" -gt 0 ]; then
  exec "$@"
fi
exec /usr/local/bin/csgclaw-codex-gateway
EOF

RUN chmod 755 /usr/local/bin/csgclaw-codex-gateway /usr/local/bin/csgclaw-codex-entrypoint && \
    command -v csgclaw-codex-gateway && \
    command -v csgclaw-codex-entrypoint && \
    mkdir -p \
      /home/codex/.codex-sandbox/workspace/projects \
      /home/codex/.codex-sandbox/codex-home && \
    chmod -R 0777 /home/codex

ENV HOME=/home/codex \
    LARK_CHANNEL_HOME=/home/codex/.codex-sandbox \
    LARK_CHANNEL_PROFILE=csgclaw \
    LARK_CHANNEL_CONFIG=/home/codex/.codex-sandbox/config.json \
    LARK_CHANNEL_CODEX_BIN=/usr/local/bin/codex \
    LARK_CHANNEL_TENANT=feishu \
    CODEX_HOME=/home/codex/.codex-sandbox/codex-home \
    CSGCLAW_CODEX_GATEWAY_WORKSPACE=/home/codex/.codex-sandbox/workspace/projects

WORKDIR /home/codex/.codex-sandbox/workspace/projects

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/csgclaw-codex-entrypoint"]
CMD ["/usr/local/bin/csgclaw-codex-gateway"]
