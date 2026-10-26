# DockLantern Architecture Overview

**Status:** Go runtime selected; single-container Compose execution preferred, pending integration validation.

## Logical components

- **Dashboard/API (Go):** service discovery, policies, version state, update history and validated user actions.
- **Controller (Go):** OCI registry polling, bounded schedules, update policies, desired state and drift analysis.
- **Compose executor (Go module):** allowlisted project/service operations, merging original Compose files and generated image overrides.
- **Persistent store:** desired images, policies, credential references, operation journal and history.
- **Docker Engine:** authoritative running container/image/health state.
- **OCI registries:** source of remote manifests and authenticated pulls.

These are logical modules, not required microservices. Prefer a single Go process and container for the first supported Linux host configuration.

## Source / desired / actual

1. **Source:** user-controlled, read-only Compose definitions and interpolation context.
2. **Desired:** stored deployment image references and live polling/update policies.
3. **Actual:** running image IDs, digests and container status from Docker Engine.

A generated image-only override is derived from desired state, one per project, and applied last after the original ordered Compose files. External Compose invocations omitting it may cause drift; show and optionally adopt that drift, do not auto-revert.

## Compose execution: initial design

Mount configured host Compose roots **read-only at identical absolute paths** in the DockLantern container (e.g. `/srv/deployments:/srv/deployments:ro`). Store DB/journal/overrides in a separate `/data` volume. The executor operates against an allowlist of registered project names, original file order, project directory, environment context and allowed services. Docker socket access is privileged and must be isolated from unauthenticated web operations.

Use the official Docker Compose Go SDK if validation proves sufficient parity with expected CLI behavior; otherwise consider a pinned Compose CLI invoked through tightly controlled arguments. The executor backend is internal and replaceable, allowing a future host-side agent where path/security needs warrant it.

**Never deploy** a project whose effective configuration cannot be faithfully reconstructed or whose paths/credentials are inaccessible. Such projects remain monitor-only.

See [ADR 0005](decisions/0005-executor-boundary.md) for executor decision and [ADR 0006](decisions/0006-runtime-selection.md) for language choice.

## Discovery / onboarding

Find running containers using Compose labels. **Beginning with beta.3, monitoring-only enrollment** uses an explicit opt-in service label or an authenticated GUI opt-in, with Docker image/platform metadata and no Compose source mounts. The monitored project/service is reconciled from discovery; default is not monitored. Monitoring labels do not authorize deployment. **Managed deployment remains gated** on separately registered and validated Compose project paths, file order, environment context, profiles and effective configuration. Discovery metadata is a hint, not an immutable record of original source. See [ADR 0007](decisions/0007-label-enrollment-monitoring-only.md).

## Update lifecycle

Poll OCI manifests by platform-specific digest. For authorized update: resolve candidate digest, pull, snapshot prior running identity/effective config, stage and validate image override, deploy targeted services, wait for readiness, and record success or failure. Lock by project, journal operations and reconcile unfinished state on restart. Recovery is best effort; external persistent changes cannot be rolled back automatically.

## Cooperative external updates

Docker events plus reconciliation identify source/desired/actual differences, including same-tag/new-digest externally deployed images. Offer Adopt, Restore or Ignore. Adoption is configuration-only, with no pull or service recreation.

## Initial deployment compatibility

Linux VPS/cloud VM/home lab Docker Engine; Docker Compose projects with existing Traefik networks. No requirement to run Traefik, no automatic `docker compose down`, no multi-host or Swarm/Kubernetes support in the first milestone.

## Security note

Read-only Compose mounts protect against accidental writes **but not reading secrets**. Unrestricted Docker socket access carries high host privilege. Require authenticated dashboard, allowlisted project/service operations, redacted secrets and a hardened deployment review before broad public deployment.
