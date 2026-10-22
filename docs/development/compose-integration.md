# Disposable Docker Compose Integration Suite

The POC now includes a **live Docker Engine test** in addition to command-runner unit tests. It is intentionally isolated from any production deployment.

## CI

The `compose-integration` GitHub Actions job runs on an ephemeral Linux runner. It:

1. Creates a random project name, isolated temporary Compose directory, and uniquely named external network.
2. Starts two containers from a multi-file fixture with Traefik-style labels, a relative bind mount, and environment interpolation.
3. Discovers both Compose services by Docker labels.
4. Generates a managed image override, checks the original Compose definition is unchanged, and verifies no container was recreated during preparation.
5. Rejects a deployment lacking explicit confirmation.
6. Attempts to pull a nonexistent image and confirms the running container is not replaced.
7. Updates **only web** using the explicit POC confirmation flag, validates the new image, network, labels, mount and interpolated environment, and confirms worker retained its container ID.
8. Tears down **only the randomly named fixture project/network** even when assertions fail.

The script does not deploy or configure Traefik itself: labels/networks emulate an app behind an existing reverse proxy.

## Run locally (disposable Docker host only)

From a clean checkout, with a working Docker Engine and Docker Compose CLI:

```sh
go test -race ./...
go vet ./...
DOCKLANTERN_INTEGRATION_TEST=1 bash tests/integration/run-compose.sh
```

The explicit environment guard is mandatory. The suite pulls `nginx:1.27-alpine`, `nginx:1.28-alpine`, and `busybox:1.36` and requires `go`, `jq`, and `docker compose`. Use a disposable Docker-enabled machine or a clean GitHub Actions runner. The script creates and removes Docker containers, a bridge network and a temporary host directory. Do not run on a shared or production VPS.

## Scope and limitations

This suite tests the **Compose CLI backend**, not the official Go Compose SDK. It does not exercise DockLantern itself inside a Docker container, meaning it does not yet prove equal host/container absolute path mappings or Docker socket isolation. Other exclusions include credential handling, digest pinning, health-check rollback, concurrent operations, durable journals, restart recovery, and rootless Docker.

Before declaring the Compose executor suitable for unattended updates, add host/container path-parity tests with a purpose-built image, strict executor allowlist enforcement at the service boundary, and update journaling. The current CLI's registration file is user-supplied and not a secure authorization mechanism.
