# DockLantern monitoring dashboard and registry scheduler.
# Deliberately DOES NOT launch the experimental Compose update executor.
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/docklantern-inventory ./cmd/docklantern-inventory

# Docker CLI is used for optional read-only inventory scanning.
# A mounted Docker socket remains highly privileged; omit it by default.
FROM docker:28-cli
RUN apk add --no-cache ca-certificates && mkdir -p /data && chmod 0700 /data
COPY --from=build /out/docklantern-inventory /usr/local/bin/docklantern-inventory
WORKDIR /data
EXPOSE 8787
ENTRYPOINT ["/usr/local/bin/docklantern-inventory"]
CMD ["-mode", "serve", "-db", "/data/docklantern.db", "-listen", "0.0.0.0:8787", "-run-monitor"]
