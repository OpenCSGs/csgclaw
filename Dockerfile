ARG GO_IMAGE=golang:1.26.2-alpine
ARG NODE_IMAGE=node:22.13.0-alpine
ARG PNPM_VERSION=11.1.3
ARG NPM_REGISTRY=https://registry.npmmirror.com
ARG RUNTIME_IMAGE=alpine:3.23

FROM ${NODE_IMAGE} AS web

WORKDIR /src/web/app

ARG PNPM_VERSION
ARG NPM_REGISTRY
RUN npm config set registry ${NPM_REGISTRY} && npm install -g pnpm@${PNPM_VERSION}

COPY web/app/package.json web/app/pnpm-lock.yaml web/app/.npmrc ./
RUN pnpm config set registry ${NPM_REGISTRY} && pnpm install --frozen-lockfile

COPY web/app ./
RUN pnpm build && test -f ../static-dist/index.html

FROM ${GO_IMAGE} AS build

WORKDIR /src

ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}

RUN apk add --no-cache bash ca-certificates curl tar gzip

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web /src/web/static-dist ./web/static-dist

ARG TARGETOS=linux
ARG TARGETARCH=amd64
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
    /out/codex --version

FROM ${RUNTIME_IMAGE}

USER root

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/csgclaw /opt/csgclaw/bin/csgclaw
COPY --from=build /out/csgclaw-cli /opt/csgclaw/bin/csgclaw-cli
COPY --from=build /out/codex /opt/csgclaw/bin/codex

RUN chmod 755 /opt/csgclaw/bin/csgclaw /opt/csgclaw/bin/csgclaw-cli /opt/csgclaw/bin/codex && \
    printf '%s\n' '{"app":"csgclaw","layout":"official-bundle"}' > /opt/csgclaw/.csgclaw-bundle.json && \
    ln -s /opt/csgclaw/bin/csgclaw /usr/local/bin/csgclaw && \
    ln -s /opt/csgclaw/bin/csgclaw-cli /usr/local/bin/csgclaw-cli && \
    ln -s /opt/csgclaw/bin/codex /usr/local/bin/codex

WORKDIR /opt/csgclaw

ENTRYPOINT ["/opt/csgclaw/bin/csgclaw"]
CMD ["--help"]
