# Security Architecture — Monitoring and Planned Executor

**Status:** beta.5 monitoring boundary implemented and accepted on a test VPS; beta.6 executor design accepted but **not implemented or production-hardened**. See [ADR 0008](decisions/0008-on-demand-compose-executor.md).

Detailed proposed beta.6 controls: [executor threat model](../security/beta6-executor-threat-model.md) and [deployment contract](beta6-deployment-contract.md). Neither document grants implementation or deployment approval.

## Existing monitoring boundary

- The dashboard is authenticated, served behind trusted HTTPS, and uses origin/CSRF checks for mutations to monitoring settings.
- The monitor uses a private GET-only Docker socket proxy; permitted Docker metadata can still reveal sensitive information.
- There is **no Docker write access, direct socket mount, Compose CLI or executor launch path** in the monitoring container.
- Monitoring labels, GUI enrollment, watched-tag overrides and registry digest changes do not authorize deployment.

## Additional beta.6 threats and controls (design requirements)

| Threat | Required control |
|---|---|
| Compromised GUI crafts privileged actions | Proposal is untrusted; CLI cannot be invoked remotely; independent executor allowlist and interactive host-local approval |
| Proposal path traversal, symlinks or oversized content | Bounded versioned schema, safe directory ownership, no path-derived authority, atomic read/validation |
| Privileged data overwritten by monitor | Distinct host-owned executor state/journal/overrides; monitor cannot write or modify executor authority |
| Malicious Compose file/env changes | Verify trusted file ownership/permissions, file ordering, interpolation, allowlisted roots, and config fingerprint at execution |
| CLI environment or path hijack | Fixed trusted Docker/Compose executable paths and Docker context, sanitized environment, no arbitrary shell or web-controlled flags |
| Moving tags or stale observations | Independently verify immutable OCI digest/platform and actual running state; reject stale/ambiguous plans |
| Scope escape to siblings or Docker host | Restrict supported Compose subset, compare normalized configs, scoped service update, no arbitrary Docker operations |
| Concurrent Compose/CI actions | Per-project durable lock, preflight revalidation, fail-closed drift detection; external tools do not honor DockLantern locks |
| Crash after side effect | Journal intent before execution; reconcile actual state without automatically replaying mutations |
| Credentials/secrets leaked in UI or logs | Redact Compose-resolved environments, OCI credentials, Docker command output, journal and receipt contents |
| False safety claims after failed health check | Distinguish running from healthy, record partial/uncertain outcomes, manual recovery; no guaranteed data rollback |
| Loss of VPS/SQLite/operation history | Off-host backups and tested restore for both monitor and executor state before real execution |

## Authorization and recovery

The on-demand host-native executor has Docker write authority only during a local operator invocation, but that authority is effectively host-administrative on common Docker installations. Runtime separation is **not** Docker daemon sandboxing.

Registrations are executor-owned and separate from monitoring-era `registrations.enabled`. No web-to-executor API or persistent privileged daemon is planned for beta.6. Every apply/recovery operation requires independent review and interactive confirmation. Invalid, incomplete or unsupported Compose projects remain monitor-only.

A failed pull/config preflight must not replace a running service. Later-stage failure can leave actual state different from committed desired state; block further operations pending reconciliation. Manual image recovery does not restore persistent application data.

## Release security gates

Disposable-host adversarial and interrupted-execution tests, version-pinned Compose compatibility, trusted-file and plan parsing reviews, stable monitoring proxy permission tests, and backup **restore** drill are mandatory before any real host execution. Use explicit operator gates for implementation merges, release tags/publication and test-VPS changes. No security or production certification is implied by an accepted ADR.
