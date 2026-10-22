# Containerized Compose execution — validation (PR #5)

## Purpose

Validate the **same-absolute-host-path** design from ADR 0005 by executing the experimental Go POC **inside a container**, against a disposable Compose project on the same Docker host.

## What this proves

- Compose source files can be mounted read-only at their unchanged absolute paths.
- A Docker CLI + Compose plugin image can load the original multi-file project and an updater-generated override.
- A relative bind mount still resolves to the host's original project directory during a service replacement.
- Existing Traefik-style labels, an external Docker network, and interpolated environment settings survive replacement.
- A `worker` service retains the same container ID when `web` is updated using `--no-deps`.
- Invalid image pulls and missing confirmation fail without replacing the running service.
- Original Compose source is not overwritten.
- Docker labels can be discovered through the mounted Docker socket.

## Run

Only on an **ephemeral/disposable Docker host** with Go, Docker Engine, Compose plugin and jq:

```sh
DOCKLANTERN_INTEGRATION_TEST=1 bash tests/integration/run-containerized.sh
```

CI runs this on a GitHub-hosted Linux runner. All resources use a randomly named Compose project/network and a temporary directory and are removed by a trap. No connection to the user's VPS is made.

The disposable POC image is built from `Dockerfile.poc`: a static Go executable plus a pinned-major Docker CLI image with the Compose plugin.

## Security and architectural limitations

This test intentionally gives the disposable executor container direct Docker socket access (effectively host administrative privilege). A read-only Compose mount **does not** provide a security boundary against an adversarial process with that socket.

The current prototype requires a user-supplied JSON registration file and `-confirm-unsafe-poc-deploy`. Neither provides a safe production authorization model. Do not run against real projects.

This test does **not** validate the official Go Compose SDK, remote Docker engines, rootless Docker, multi-platform images, health-based rollback, journaling, secret storage, or host-side executor isolation. Docker CLI/plugin versions must be pinned/tested for releases, and the approach must be hardened before unattended production deployments.
