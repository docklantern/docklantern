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

# Monitoring-only runtime: no Docker CLI, Compose or Buildx binaries.
# Optional discovery uses the constrained read-only HTTP socket proxy.
FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates && mkdir -p /data && chmod 0700 /data
COPY --from=build /out/docklantern-inventory /usr/local/bin/docklantern-inventory
WORKDIR /data
EXPOSE 8787
ENTRYPOINT ["/usr/local/bin/docklantern-inventory"]
CMD ["-mode", "serve", "-db", "/data/docklantern.db", "-listen", "0.0.0.0:8787", "-run-monitor"]
