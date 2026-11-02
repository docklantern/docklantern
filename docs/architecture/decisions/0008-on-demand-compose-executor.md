# ADR 0008 — On-Demand, Operator-Authorized Compose Executor

**Status:** Accepted architecture for beta.6; implementation and deployment not authorized  
**Date:** 2026-11-01  
**Target:** v0.1.0-beta.6 (design baseline only)  
**Supersedes:** ADR 0005's in-process executor topology; refines ADR 0006  
**Related:** ADRs 0002, 0003, 0004, 0005, 0006, 0007; credential-scope issue #62

## Context

DockLantern v0.1.0-beta.5 is deployed and accepted on an IPv6-only test VPS as a **monitoring-only** application. It uses a private GET-only Docker socket proxy for discovery, observes OCI registry manifests, maintains image/platform history in SQLite, and provides authenticated watched-tag and Check Now controls. Monitoring enrollment does not grant deployment authority.

The existing experimental `internal/poc/executor.go` and disposable Compose integration tests demonstrate targeted Compose image replacement and image-only overrides. They are not production-safe: operator authorization, immutable candidate verification, independent privileges, durable journaling, interruption recovery, and broad Compose-configuration validation are incomplete.

ADR 0005 originally preferred an executor module in the web/controller container with Docker write access. Giving the always-on monitoring process that access would undo a useful security boundary. The design must also work on small Linux VPS hosts and preserve original operator-managed Compose files.

## Decision

**Use a separate, on-demand, host-native Go CLI named `docklantern-executor`.** It is invoked exclusively by an authorized operator through a host-local terminal or SSH session. The executor uses a validated installation of the host Docker Engine/Compose CLI. It does not run as a daemon, serve network requests, or accept commands from the web process.

The existing monitoring application keeps the restricted GET-only Docker proxy. It must not mount the Docker socket directly, invoke the executor, access executor-owned deployment credentials, or receive Docker API write permissions.

The executor will ship as a separate build artifact from the same application source repository. The monitoring Docker image must continue to exclude both the executor and Docker/Compose CLI.

### Authority and component ownership

| Component | Allowed responsibility | Explicitly forbidden |
|---|---|---|
| Monitoring GUI/controller | Inspect evidence, generate untrusted proposals, show sanitized receipts | Execute Docker writes, grant deployment permission, launch executor |
| Operator | Independently register projects; preview and confirm each operation in a trusted terminal | Delegate unattended execution through a GUI action |
| Host executor | Enforce protected registrations, revalidate proposals, pull/apply scoped Compose changes, journal/reconcile | Accept arbitrary shell commands, Compose flags, host paths or Docker endpoints from the web |
| Docker Engine | Authoritative observed container/image/health state | Be treated as proof of original Compose configuration or upgrade ordering |

An attacker controlling only the monitoring application must not be able to trigger a Docker write through DockLantern. The proposal inbox is **not** a privileged command queue.

### Deployment enrollment is separate from monitoring

Monitoring labels (`com.docklantern.monitor=true`), GUI monitoring enrollment, watched-tag overrides, image history, and digest changes **never** imply deployment authorization.

Deployment requires an independently approved, protected executor registration with:

- Exact Docker Engine/context and Compose project name.
- Canonical host project directory, ordered Compose source files, explicit env files, profiles and interpolation inputs.
- Approved service allowlist and filesystem roots.
- Supported Compose configuration characteristics and enablement policy.

Existing monitoring registrations, including historical `registrations.enabled` values, are **not** converted into executor authorization. Discovery labels are hints, not authoritative Compose source or configuration.

The executor must verify trusted ownership, safe path resolution and permissions for approved configuration inputs. A missing or mutable/untrusted input, path escape, symlink race, project mismatch or unrepresentable Compose configuration blocks execution; monitoring remains available.

### GUI proposal, host confirmation, and filesystem handoff

Beta.6 uses a filesystem handoff, not RPC or a persistent privileged sidecar:

1. A bounded, versioned **proposal inbox** is writable by DockLantern and readable by the executor. Proposals are untrusted data.
2. An **executor-owned state directory** contains approved registrations, project locks, a durable journal and managed overrides. DockLantern cannot modify it.
3. An **executor-written sanitized receipt outbox** is read-only to DockLantern and supplies Activity/Service Details history. Raw journals and source configs are not exposed.

The deployment guide will specify host paths, mount modes, ownership, permissions, atomic publication and limits. Monitor-writable paths must not overlap trusted executable, configuration, override, journal or receipt directories. Plan IDs are lookup identifiers, **not bearer credentials**. Reject path traversal, symlink/hardlink substitution, oversized files, inconsistent ownership and unsupported proposal versions.

An operator previews the proposed action in the GUI and then uses the host-local CLI to **independently preview and confirm** the exact project, service, candidate digest and expected replacement impact. No `--yes`, unattended execution or web-initiated helper is supported in beta.6.

### Image identity and candidate requirements

Keep separate:

1. Operator-owned **source** Compose image.
2. Docker-observed **actual** container/image identity.
3. Monitoring **watch** target and its OCI evidence.
4. Proposed immutable **candidate** image/platform digest.
5. Executor-owned last **committed desired** image.

A remote digest difference is not proof of a newer semantic release or sufficient authority to deploy. The executor re-resolves/validates the selected platform, image repository, digest and approval freshness; it must not silently substitute the latest value of a moving tag. Store index and platform manifest digests separately where relevant; never confuse Docker local image configuration ID with an OCI manifest digest.

Initial beta.6 execution supports publicly accessible, prebuilt images from the approved service's current registry/repository, with a single verified platform. Private registry pull credentials, cross-repository deployments and credential-scope-aware persistence (#62) are out of scope. Ambiguous provenance or unsupported registry reachability (including IPv6 limitations) blocks execution.

### Compose execution contract

Use a tested and explicitly supported **host Docker Compose CLI**, invoked with a fixed executable path, trusted project directory/file list, known environment, bounded timeouts and typed arguments. Reject unintended Docker contexts, inherited Compose overrides and arbitrary CLI input. Do not build images or use raw Docker API replacement as a substitute for Compose lifecycle semantics.

For every operation:

- Reconstruct the exact approved Compose model, including source file order, working directory, interpolation and profiles.
- Compare the effective configuration with the current committed managed override against the same configuration plus the operation-specific image-only override. **Only the selected service's image field may differ.**
- Validate project identity, service/container membership, current image state and source/config fingerprints again before side effects.
- Pull the immutable candidate before replacement.
- Perform only a scoped, selected-service Compose operation with unrelated services/dependencies protected; do not run project-wide `down`, prune volumes, recreate unrelated targets or perform automatic builds.
- Verify actual resulting identity and running/health evidence before recording success. `running` without a HEALTHCHECK is not equivalent to application readiness.

Initially support single-project, single-service, single-replica, prebuilt-image configurations with tested semantics. Disable managed execution for untested `build`, lifecycle hooks, ambiguous `include`/`extends`, complex profiles/dependencies, privileged/socket-mount services and unsupported filesystem/network configurations. Supported feature eligibility is explicit and fail-closed.

### Managed override transaction

Retain ADR 0002: original operator-managed Compose files are never edited; DockLantern-managed image-only overrides are a separate desired-state layer applied after the original ordered source files.

For an authorized operation:

1. Preserve the current **committed** project override and running baseline.
2. Generate and validate an **operation-specific** immutable image-only override.
3. Apply using the source + committed override + operation override in that order.
4. Verify the actual selected service against the authorized candidate.
5. After verification, **atomically promote** the candidate to the committed managed override and record the committed desired state.

The committed override remains unchanged until successful verification; an interrupted/failed apply can leave actual state different from committed desired state. Record that discrepancy and block additional updates until reconciliation. Never infer success from a successful pull, a Compose process exit code alone, or a registry observation.

Other operator-driven Compose/CI deployments may omit DockLantern's managed override. Detect such drift and require manual resolution; never silently revert external changes.

### Durable journal, serialization, and recovery

Use a separate **executor-owned SQLite journal**, not direct writes into the monitoring database. Record an operation and durable phase intent **before** each side effect. Entries include: operation ID, plan hash/version/expiry, approved project/service, operator confirmation metadata, original/current/candidate image identity, effective-config fingerprint, previous override reference, phase, timestamps and redacted outcome.

Lifecycle: `PREPARE → AUTHORIZE → PREFLIGHT → PULL → STAGE → APPLY → VERIFY → COMMIT`, followed by a terminal verified outcome or `NEEDS_RECONCILIATION`. Side-effecting phases are not blindly retried. One executor operation at a time per project; an in-process mutex alone is insufficient—use a host-level lock and inspect unfinished journals. External Compose invocations do not respect that lock, so revalidate observed state and fail closed on conflicts.

After a failure or executor crash, the next host-local invocation reconciles the journal with Docker state **without automatic replay**. A clear no-change failure may be recorded as failed; unknown, partially applied, mixed-replica or ambiguous states remain blocked pending operator action.

Manual recovery is a **new, separately authorized operation**: inspect actual state, verify that the previous immutable image is still available or can be safely retrieved, preview restoring the prior desired override, perform scoped replacement and verify the result. Image restoration does **not** reverse application database migrations, persistent volume changes or external writes. No automatic rollback or guaranteed application recovery claims in beta.6.

Executor state and sanitized receipts must persist across executor exit, application restart and crashes. Monitoring SQLite and executor state have separate, explicitly documented backups and ownership.

### UI and acceptance boundaries

- **Overview/Images:** Preserve image-first monitoring; show reviewable candidates without equating digest difference to new software.
- **Image Details:** Show OCI evidence and linked service eligibility; no cross-service Update All action.
- **Services/Service Details:** Show deployment registration independently of monitoring enrollment; preview a precise single-service proposal and explain the host-local approval step.
- **Activity:** Distinguish proposed, locally authorized, running, verified, failed, uncertain, reconciled and recovered; display sanitized receipts only.

Before beta.6 can execute on a real host, require a separately approved threat model, trustworthy project registration, proposal tampering/path tests, exact CLI/config validation, stale-state and concurrency tests, interrupted-pull/apply/verify recovery tests, backup **and actual restore** exercises with an off-host copy, and disposable-host Compose integration tests. Recheck the monitoring proxy remains GET-only. Test VPS acceptance, merge, release tagging/publication, and deployment all require separate explicit operator approvals.

### Explicit beta.6 exclusions

No automatic or scheduled deployments, GUI-triggered executor, background privileged agent, monitor Docker writes, label-driven deployment authorization, private registry deployment, multi-credential support, multi-host, image builds, arbitrary Compose editing, multi-service/update-all, automatic rollback, data rollback, external drift correction, or self-update of DockLantern/host-critical services.

## Alternatives considered

1. **Privileged executor embedded in the monitoring container:** simpler operationally but collapses the web/monitoring trust boundary; rejected.
2. **Always-on privileged sidecar:** enables one-click GUI execution but permanently exposes a host-powerful executor control channel; deferred.
3. **Ephemeral privileged executor container:** maintains runtime isolation and pinned CLI packaging, but requires exact host/container path parity and privileged socket mounting; retained as a possible future packaging option.
4. **Host-native on-demand CLI:** uses real host paths and the existing Docker/Compose installation, avoids a new network service, and ensures each operation requires local operator participation; selected.

## Consequences

**Benefits:** Monitoring isolation persists, privilege is absent from the always-running app, updates are explicit and auditable, original Compose sources remain untouched, and no additional continuously running infrastructure is required.

**Tradeoffs:** Updates require SSH/local terminal access; the installed Compose CLI must be version-controlled or compatibility-validated; filesystem ownership and shared handoff need careful hardening; separate state/backup domains require disciplined recovery; broad Compose support and browser-only execution are deferred. Docker write access remains effectively host-administrative while the executor runs.

## Relationship to earlier ADRs

- **ADR 0005:** Superseded for executor topology/SDK preference and container mount model. Its validation, source/desired/actual separation and careful failure-handling principles remain applicable.
- **ADR 0006:** Go remains the language, but monitoring and deployment execution are separate binaries/processes. The Compose CLI is the beta.6 execution backend; SDK research remains nonbinding.
- **ADRs 0002/0003:** Managed image-only overrides and explicit project registration remain, strengthened by independent executor authorization and atomic commit.
- **ADR 0004:** External adoption remains manual; broad adoption/restore/ignore workflows are future work.
- **ADR 0007:** Monitoring enrollment and its GET-only guarantees remain unchanged.

**Approval boundary:** This ADR accepts an architectural direction only. It does **not** authorize implementing an executor, opening implementation PRs, enabling Docker writes, making test-VPS changes, merging this documentation PR, or publishing releases/tags.
