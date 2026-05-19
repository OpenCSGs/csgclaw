ARG GO_IMAGE=golang:1.26.2-alpine
ARG RUNTIME_IMAGE=alpine:3.23

FROM ${GO_IMAGE} AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG VERSION_PKG=csgclaw/internal/version

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}" \
      -o /out/csgclaw ./cmd/csgclaw && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
      -ldflags="-s -w -X ${VERSION_PKG}.Version=${VERSION} -X ${VERSION_PKG}.Commit=${COMMIT} -X ${VERSION_PKG}.BuildTime=${BUILD_TIME}" \
      -o /out/csgclaw-cli ./cmd/csgclaw-cli

FROM ${RUNTIME_IMAGE}

USER root

RUN apk add --no-cache ca-certificates tzdata

COPY --from=build /out/csgclaw /usr/local/bin/csgclaw
COPY --from=build /out/csgclaw-cli /usr/local/bin/csgclaw-cli

RUN chmod 755 /usr/local/bin/csgclaw /usr/local/bin/csgclaw-cli

WORKDIR /opt/csgclaw

ENTRYPOINT ["/usr/local/bin/csgclaw"]
CMD ["--help"]
