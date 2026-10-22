# DockLantern Architecture Overview

**Status:** Architectural direction established; implementation stack/executor unresolved.

## Logical components

- **Dashboard/API:** Service discovery, policies, version state, update history, validated user actions.
- **Controller:** Registry polling, bounded schedules, update policies, desired state, drift analysis.
- **Compose executor:** Allowlisted registered project/service operations using the correct file order and host paths.
- **Persistent store:** Desired image selection, service configuration, credential references, operation journal and history.
- **Docker Engine:** Authoritative actual container/image/health state.
- **OCI registries:** Source of remote image manifests and authenticated pulls.

These are logical modules, not required microservices.

## Source / desired / actual

1. **Source:** User-controlled Compose definitions and interpolation context. Read-only to DockLantern.
2. **Desired:** Stored deployment image references and live monitoring policies.
3. **Actual:** Container/image IDs and health retrieved from the Docker Engine.

A generated Compose override is derived from desired state, one per project, and applied after the original ordered Compose files. Ordinary Compose invocations that omit it can introduce drift; this should be reported, not automatically reversed.

## Discovery / onboarding

Use Docker Compose labels as hints for project, service, working directory and config file paths. Find files within configured read-only roots. Register projects explicitly and validate the effective Compose model before allowing managed updates. Absent or invalid Compose sources stay monitor-only.

## Update lifecycle

Check remote image platform digest; resolve immutable identity; pull; journal old running image/effective configuration; stage and validate override; apply scoped Compose deployment; verify readiness with timeout; record success/failure. Serialize conflicting operations and rate-limit registry checks. Rollback is best-effort, never a guarantee for persistent application state.

## Adoption

Detect external deployment changes, including new digests under the same mutable tag. Offer Adopt / Restore / Ignore. Adoption changes desired configuration and history only—no pull, restart or container recreation.

## File/privilege boundary

Candidate initial approach: configured read-only bind mounts of Compose roots and separate writable updater data. Correct host paths matter because Docker bind mounts refer to paths on the daemon host. Test this with real Compose fixtures. A small host-side executor remains a possible safer approach. An unrestricted Docker socket grants extensive host privileges even when Compose files are read-only.
