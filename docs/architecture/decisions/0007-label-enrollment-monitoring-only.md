# ADR 0007 — Label-Based Monitoring Enrollment Without Compose Source Mounts

**Status:** Accepted for v0.1.0-beta.3 monitoring; implementation pending  
**Date:** 2026-10-25

## Context

The beta.2 deployment required an operator to:
1. Add read-only mounts for every watched project's Compose directory to DockLantern.
2. Extend DockLantern's `-roots` allowlist and recreate its container.
3. Manually register the project and create a watch policy via the GUI.

That is appropriate as a foundation for **future source-validated Compose deployments**, but unnecessarily burdensome for **read-only OCI registry monitoring**. DockLantern's restricted Docker proxy already exposes container Compose project/service labels, image references and image/platform metadata via GET. Existing ADR 0003 requires explicit file validation before deployment; this ADR separates that safety boundary from monitoring-only enrollment.

## Decision

1. **Monitoring-only enrollment** can be authorized by `com.docklantern.monitor=true` on a Compose service or by an explicit, authenticated GUI action for an unlabeled Compose service. Container discovery never implicitly enables unlabeled services.
2. `com.docklantern.monitor=false` is a hard opt-out overriding GUI enrollment. Removal of a previously true label deactivates label-owned enrollment unless separately GUI-enrolled; preserve historical records. A GUI interval override by itself does not count as enrollment.
3. Optional `com.docklantern.interval` accepts a validated 60s–24h duration. Effective interval precedence is GUI override > Docker label > durable global default (initially 5 minutes). Users can reset an override. Pause/resume is a separate persisted scheduling state. Changes are live, not container-restart dependent.
4. Derive project/service identity from Compose labels and image/platform from verified Docker metadata. Monitor one policy per Compose service even when there are multiple replicas. Reject inconsistent replicas or invalid labels with visible explanations.
5. Only reconcile on complete successful discovery snapshots; do not treat Docker/proxy failure as proof of service removal. Stop stale label-only checks when a service is no longer present, preserving history for diagnosis. Changes must be idempotent and persisted in SQLite.
6. **No new Docker permissions:** monitoring continues using bounded GET-only requests through the existing isolated Docker socket proxy and read-only registry lookups. Labels grant no file access, Docker writes, pull commands, Compose execution, service recreates or deployment authorization.
7. Existing beta.2 registered monitoring policies and registrations must migrate without loss or unintended re-enrollment. Preserve legacy/manual enrollment separately from label/GUI provenance.
8. **Future managed Compose deployments** still require explicit project registration and validation of ordered source files, environment interpolation, paths, profiles and executor permissions (ADRs 0003 and 0005). A label-only service must never be eligible for managed deployment solely because it is being monitored.

## Alternatives considered

- **Continue manual registrations and per-project bind mounts:** safe for future deployment management but operationally expensive and unnecessary for monitoring.
- **Watch every discovered Compose service automatically:** attractive onboarding but unexpected registry traffic/credentials and changes to services the operator did not intentionally enroll; rejected in favor of opt-in.
- **Only use Docker labels, with no GUI overrides:** declarative but inconvenient for operational tuning and pauses; rejected in favor of persisted source-aware precedence.
- **Give DockLantern unrestricted Docker/Compose access to avoid mounts:** expands host privileges and creates deployment risk; rejected for beta.3.

## Consequences

**Benefits**
- Onboard a new monitored Compose service by changing its own deployment only.
- No DockLantern config edits, restarts, or source mounts for ordinary monitoring.
- Runtime GUI interval changes can be persisted without container recreation.
- Keep the Docker socket proxy GET-only and retain a clear future deployment authorization boundary.

**Tradeoffs**
- Need provenance-aware schema migration and careful Docker-label ↔ GUI reconciliation.
- Docker labels change only when the *monitored service* is recreated; this differs from instant GUI settings.
- Service removals, replicas with conflicting configuration, unexpected untagged image references and incomplete image metadata must yield explicit safe/inactive states.
- A registry digest change is an image/platform-scoped observation, not a verified newer release nor a per-service deployment action.

## Validation

The beta.3 release must pass the [beta.3 implementation plan](../../product/beta3-plan.md) and tracking issues [#27](https://github.com/docklantern/docklantern/issues/27), [#29](https://github.com/docklantern/docklantern/issues/29), [#30](https://github.com/docklantern/docklantern/issues/30), [#31](https://github.com/docklantern/docklantern/issues/31), and [#34](https://github.com/docklantern/docklantern/issues/34).

**Supersession:** ADR 0003 remains authoritative for **deployment registration/validation** but its manual-registration requirement does not apply to monitoring-only enrollment beginning with beta.3. ADR 0005's executor/write boundary is unchanged.
