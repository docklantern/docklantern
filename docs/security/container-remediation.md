# PR #23 — Container vulnerability remediation

## Scope

The monitoring-only Docker image previously inherited a Docker CLI runtime containing outdated Alpine packages and large precompiled Docker/Compose/Buildx binaries. These are unnecessary for OCI registry checking, the authenticated HTTP dashboard, SQLite storage and **HTTP-only** inventory from the constrained socket proxy. PR #23 replaces the runtime with an Alpine base pinned to its OCI manifest digest, applies current Alpine security updates at build time and ships only the DockLantern binary plus CA certificates and basic shell utilities.

The optional `docker-socket-proxy` container is still based on the known Tecnativa v0.5.0 release but now uses an immutable upstream manifest and `apk upgrade --no-cache`. Its actual rebuilt image is scanned by Trivy, not its old vulnerable upstream tag. A successful security scan does not make Docker socket access risk-free.

## Compatibility and intentional omissions

- The production monitoring image **does not contain `docker`, `docker compose`, or `docker buildx`**. This is an intentional reduction in attack surface.
- For read-only discovery in the packaged image, enable the optional `compose.discovery.yaml` overlay. Discovery then uses HTTP GET requests to the isolated Docker socket proxy; no Docker socket is mounted into DockLantern.
- Direct CLI discovery (`-mode discover`, `-mode scan` without a configured HTTP proxy) continues to work when the standalone binary is run **on a trusted host with Docker CLI installed**, but not inside the packaged monitoring container.
- The restricted proxy still uses an internal network, `POST=0`, and only the required container/image GET endpoints. Existing disposable integration tests assert mutation requests are blocked.
- The proxy service must be **built locally** by Docker Compose using its added `build` directive. `docklantern-socket-proxy:patched` is a local image name; it is **not** published to a registry by this PR.
- Both base images are pinned in `build/base-images.lock`; the privileged proxy base is pinned in `build/proxy-base.lock`. The release-input CI gate enforces all references.

## Evidence and remaining work

PR #22 recorded 40 HIGH/CRITICAL Alpine findings and significant Docker CLI/plugin advisories in the old monitoring image, and 6 fixable HIGH findings in the old proxy. CI security scans on PR #23 must pass for the rebuilt monitoring and proxy artifacts, while Go source vulnerability checking continues to use the pinned builder. All nine functional/integration jobs must also pass.

`apk upgrade` fetches current repository packages during build, so this is **not bit-for-bit reproducible** and must be scanned again when a version tag is released. The runtime is small and pinned at the base manifest, but additional governance (lock Alpine package indexes/versions, artifact SBOM, signer attestations and external audit) remains future hardening. The socket proxy still has a host Docker socket and should only be enabled when necessary; it has no public port.

**No update execution**, release tag, GHCR image push or production VPS deployment occurs in PR #23.
