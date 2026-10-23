# DockLantern monitoring dashboard and registry scheduler.
# Deliberately DOES NOT launch the experimental Compose update executor.
FROM golang:1.26-alpine@sha256:27f2d3b83cfc5803c8078f8e730af0f7c431bf067afaec8f60a36fb763187cba AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -buildvcs=false -ldflags="-s -w -X main.version=${VERSION}" -o /out/docklantern-inventory ./cmd/docklantern-inventory

# Docker CLI is used for optional read-only inventory scanning.
# A mounted Docker socket remains highly privileged; omit it by default.
FROM docker:28-cli@sha256:625d9431a9f54c5a2bc90f24f0e1c3d55b1349fd857dd85035f98c2c9acbdd4d
RUN apk add --no-cache ca-certificates && mkdir -p /data && chmod 0700 /data
COPY --from=build /out/docklantern-inventory /usr/local/bin/docklantern-inventory
WORKDIR /data
EXPOSE 8787
ENTRYPOINT ["/usr/local/bin/docklantern-inventory"]
CMD ["-mode", "serve", "-db", "/data/docklantern.db", "-listen", "0.0.0.0:8787", "-run-monitor"]
