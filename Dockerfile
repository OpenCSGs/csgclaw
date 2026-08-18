ARG GO_IMAGE=golang:1.26.2-alpine
ARG NODE_IMAGE=node:22.13.0-alpine
ARG PNPM_VERSION=11.1.3
ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG RUNTIME_IMAGE=alpine:3.23
ARG APK_REPOSITORY=https://mirrors.aliyun.com/alpine

# The Web UI is architecture-independent. Build it on the BuildKit worker so a
# multi-platform image build does not run Node under QEMU for every target.
FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS web

WORKDIR /src/web/app

ARG PNPM_VERSION
ARG NPM_REGISTRY
RUN npm config set registry ${NPM_REGISTRY} && npm install -g pnpm@${PNPM_VERSION}

COPY web/app/package.json web/app/pnpm-lock.yaml web/app/.npmrc ./
RUN pnpm config set registry ${NPM_REGISTRY} && pnpm install --frozen-lockfile

COPY web/app ./
RUN pnpm build && test -f ../static-dist/index.html

# CGO is disabled below, so Go can cross-compile the target binary while the
# compiler itself runs natively on the BuildKit worker.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ARG APK_REPOSITORY
ENV GOPROXY=${GOPROXY}

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_REPOSITORY}|g" /etc/apk/repositories && \
    apk add --no-cache bash ca-certificates curl tar gzip

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/web/static-dist ./web/static-dist

# These are supplied automatically by Buildx. Do not give them defaults: a
# default here would override the target platform selected by --platform.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG VERSION_PKG=csgclaw/internal/version
ARG CODEX_CLI_DOWNLOAD_BASE_URL=https://csgclaw.opencsg.com/codex-cli/latest

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}" \
      -o /out/csgclaw ./cmd/csgclaw && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}" \
      -o /out/csgclaw-cli ./cmd/csgclaw-cli

RUN set -eux; \
    test "${TARGETOS}" = linux || { echo "unsupported bundled Codex CLI target: ${TARGETOS}/${TARGETARCH}" >&2; exit 1; }; \
    CODEX_CLI_DOWNLOAD_BASE_URL="${CODEX_CLI_DOWNLOAD_BASE_URL}" \
      ./scripts/fetch-codex-cli.sh "${TARGETOS}" "${TARGETARCH}" /out; \
    test -x /out/codex

FROM ${RUNTIME_IMAGE}

USER root

ARG APK_REPOSITORY

RUN sed -i "s|https://dl-cdn.alpinelinux.org/alpine|${APK_REPOSITORY}|g" /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata

COPY --from=build /out/csgclaw /opt/csgclaw/bin/csgclaw
COPY --from=build /out/csgclaw-cli /opt/csgclaw/bin/csgclaw-cli
COPY --from=build /out/codex /opt/csgclaw/bin/codex

RUN chmod 755 /opt/csgclaw/bin/csgclaw /opt/csgclaw/bin/csgclaw-cli /opt/csgclaw/bin/codex && \
    /opt/csgclaw/bin/codex --version && \
    printf '%s\n' '{"app":"csgclaw","layout":"official-bundle"}' > /opt/csgclaw/.csgclaw-bundle.json && \
    ln -s /opt/csgclaw/bin/csgclaw /usr/local/bin/csgclaw && \
    ln -s /opt/csgclaw/bin/csgclaw-cli /usr/local/bin/csgclaw-cli && \
    ln -s /opt/csgclaw/bin/codex /usr/local/bin/codex

WORKDIR /opt/csgclaw

ENTRYPOINT ["/opt/csgclaw/bin/csgclaw"]
CMD ["--help"]
