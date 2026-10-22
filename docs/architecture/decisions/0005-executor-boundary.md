# ADR 0005 — Compose Execution Boundary

**Status:** Accepted as initial design; integration validation required  
**Date:** 2026-10-21

## Context

DockLantern manages existing Docker Compose projects on Linux hosts without changing their source files. Operators expect a lightweight self-hosted deployment. Compose resolves paths using the first Compose file or explicit project directory, while bind mount source paths are interpreted by the Docker daemon on the host. A manager running in a separate container can therefore misresolve files or ask Docker to mount the wrong host directory.

The Docker socket provides effectively host-root capabilities. Read-only source mounts do not reduce those socket privileges.

## Decision

The first implementation targets **one Go-based DockLantern container on one local Linux Docker Engine**, with:

1. Read-only, allowlisted Compose source roots bind-mounted at the **same absolute paths** inside DockLantern as on the Docker host (e.g. `/srv/deployments:/srv/deployments:ro`).
2. A separate writable DockLantern data volume for SQLite, update journals and generated image-only overrides.
3. Docker Engine API access through a deliberately isolated executor module; no arbitrary host shell/Compose commands exposed by the HTTP layer.
4. Compose project registration that records the **host-absolute** working directory, ordered source file list, project name, interpolation context and applicable profiles; validate before allowing write operations.
5. A scoped executor operation interface (validate, pull, deploy selected services, inspect, recover). The executor accepts registered project/service identifiers and internally resolved policies, **not** web-supplied command strings or file paths.
6. Compose Go SDK as the **preferred integration candidate**, subject to tested parity with supported Compose CLI behavior. A pinned Compose CLI inside the same container is a fallback if the SDK does not satisfy operational requirements. The SDK/CLI choice remains an implementation validation item, not an excuse to reimplement Compose lifecycle semantics using raw Docker API calls.
7. A host-side executor **may be introduced later**, using a constrained local authenticated transport, when filesystem parity, deployment security or advanced host integrations cannot be supported safely in-container.

For projects whose source references are outside the configured roots, cannot be reliably reproduced, or require inaccessible host configuration, keep monitoring available but **disable managed deployment** until validation succeeds.

## Mount and path example

Host filesystem:
```text
/srv/deployments/spokanetech/compose.yaml
/srv/deployments/spokanetech/.env
/srv/deployments/traefik/compose.yaml
```

DockLantern container:
```yaml
services:
  docklantern:
    image: ghcr.io/docklantern/docklantern:<version>
    restart: unless-stopped
    volumes:
      - /srv/deployments:/srv/deployments:ro
      - docklantern_data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    networks:
      - web
    # Authentication and Traefik routing configuration TBD

volumes:
  docklantern_data:

networks:
  web:
    external: true
```

This is an **architectural example, not a production-hardened deployment**. It assumes an existing `web` network. It illustrates same-absolute-path mounts, not a secure Docker socket boundary. Do not use the floating `latest` tag for a security-sensitive management tool.

## Compose invocation semantics

For each registered project, reconstruct a controlled equivalent of:

```sh
docker compose --project-directory /srv/deployments/spokanetech \
  -p spokanetech \
  -f /srv/deployments/spokanetech/compose.yaml \
  -f /srv/deployments/spokanetech/compose.prod.yaml \
  -f /data/projects/spokanetech/image-override.yaml \
  config --quiet
```

Then pull and deploy **specific service selections**, never `down` / wholesale redeployment by default. The last override contains only image references; Compose resolves relative references against the base project/file context. Preserve profiles, explicit interpolation inputs, file order, project naming, and any supported includes/extends. Do not assume all runtime environment variables can be recovered from existing containers.

## Deployment state machine

`CHECK → RESOLVE_DIGEST → AUTHORIZE → PULL → SNAPSHOT → STAGE_OVERRIDE → VALIDATE_CONFIG → DEPLOY → VERIFY → COMMIT`, with failed or interrupted operations entering `NEEDS_RECONCILIATION` or an explicitly requested best-effort recovery path.

- Resolve the platform-correct OCI manifest and pin the deployment candidate to immutable image content; track both tag/reference and digest.
- Snapshot previous running image ID, registry reference where known, desired state and effective Compose configuration.
- Write a journal before side effects. Generate the override atomically and verify the merged effective config changes only authorized image references before deployment.
- Pull before replacement. Serialize deploy operations at least per Compose project.
- Wait for healthy/running state with timeout, distinguishing services with no HEALTHCHECK from actually healthy services.
- Record failure without promising rollback of database migrations, volumes or external state.
- On restart, reconcile incomplete journal entries against Docker actual state before repeating deployment.

## Source/desired/actual drift

Monitor Docker events and periodically rescan. An external deployment is **not** automatically reversed. The user can adopt it after identity/provenance checks; adoption updates stored desired state and managed override without pull or recreation. A plain image pull is not an external deployment.

## Security implications

The dashboard and controller must authorize only explicit enrolled project/service operations. Docker socket access must be treated as highly privileged; use auth, network restrictions and defense-in-depth, and evaluate a host executor or hardened socket mediation before calling production deployments hardened. Secrets in read-only Compose roots must be redacted and never exposed through config/log APIs.

## Validation gates before enabling managed updates

- Same absolute path bind mounts with relative bind volumes and absolute host paths.
- Multiple Compose files (including user overrides), profiles, env interpolation, env_file and includes.
- Traefik external networks, labels, volumes and non-updated services remain unchanged.
- Correct project selection, services sharing a single image, and concurrent update requests.
- Unavailable source, symlink escapes, read-only roots, secret handling and malformed overrides.
- Retry, interrupted update, failed pull, unhealthy service, registry auth and recovery.
- Compose SDK version compatibility and CLI fallback behavior on Linux AMD64/ARM64.

**Fallback trigger:** if the containerized approach cannot safely reproduce a valid Compose project, do not deploy that project. Evaluate a separate host executor under a new ADR rather than silently widening filesystem access.
