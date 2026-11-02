# DockLantern v0.1.0-beta.6 — Gated Implementation and Acceptance Plan

**Status:** Proposed for operator review; architecture only is approved, implementation not authorized  
**Prepared:** 2026-11-01  
**Dependencies:** [ADR 0008](../architecture/decisions/0008-on-demand-compose-executor.md), [executor threat model](../security/beta6-executor-threat-model.md), [deployment contract](../architecture/beta6-deployment-contract.md)  
**Baseline:** [v0.1.0-beta.5](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.5), accepted on the IPv6-only test VPS with documented deferred restore/security checks

## Goal and non-goals

Deliver one **manually authorized, journaled, reversible-by-new-operation (best effort)** Docker Compose image update to a previously registered service while keeping the existing lightweight monitoring application and its **GET-only Docker proxy** isolated from Docker mutations.

Successful beta.6 execution means a human can review a single eligible deployment candidate in the GUI; independently inspect and confirm that precise candidate from a host-local Go executor; apply an image-only change to the registered service; verify the observed result; and inspect a sanitized Activity record. A failed or interrupted operation is detected and requires explicit reconciliation and manually approved recovery, without automatic replay.

No automatic/scheduled updates; no GUI-triggered privileged execution; no update-all/grouped deployments; no container builds; no private registry credential handling; no broad Compose feature support; no self-update or critical Traefik/DockLantern services; no automatic rollback or rollback of volumes, databases or external effects; no multi-host, Swarm or Kubernetes support.

**Governance:** This plan is documentation only. Each implementation issue and PR requires separate explicit operator authorization. Never merge PRs, tag/publish releases, deploy/install the executor, touch the VPS, or run destructive tests on shared hosts without approval. A merged planning PR is not implementation authorization.

## Hard boundaries across every phase

1. DockLantern's shipped monitoring image remains a lightweight process **without Docker/Compose CLI, direct Docker socket, privileged executor or Docker write permissions**. The restricted proxy remains GET-only and private.
2. A proposal, watched tag, Docker label, monitoring registration or image digest difference is **never deployment authority**.
3. Host-local interactive approval, protected executor registration, safe source reconstruction and exact immutable digest/platform verification are all required independently.
4. Docker/Compose writes are scoped to **one registered service in one project**, under a per-project cross-process lock and with durable journal intent before side effects.
5. Original Compose source and user-authored overrides are untouched. An operation-specific image override is validated; the committed managed override changes only after verified success.
6. Docker state, executor SQLite, and override files **are not one atomic transaction**. Ambiguous states block further operations and require inspection. Restore of an old database/override backup demands reconciliation.
7. Only disposable Docker hosts may be mutated before separate test-VPS acceptance approval. Test fixtures must be isolated and guarded; never run a POC against real projects.

## Phase sequencing

Dependencies are a **hard gate**, not just suggested PR order. Every phase can be split into small reviewable PRs, but the first Docker write-capable implementation (Phase 5) is blocked by acceptance of Phases 0–4.

| Phase | Scope | Exit gate |
|---|---|---|
| **0 — Resolve design decisions and test boundaries** | Confirm runtime support, host file ownership, safe Compose subset, backup/reconciliation policy and no-HEALTHCHECK semantics | Security/architecture review agrees on explicit limits and disposable test architecture |
| **1 — Isolated CLI skeleton and packaging** | Independent Go executable, local operator/TTY enforcement, protected directory layout, sanitized host environment, binary artifact security | CLI cannot mutate Docker; monitor cannot launch/modify executor; release distribution is independently scanned |
| **2 — Registrations, proposals, and approvals** | Executor-owned project/service allowlist, strict proposal parser, CLI preview and confirmation with no Docker writes | Forged/malformed/stale proposals rejected; monitoring enrollment cannot authorize deployment |
| **3 — Durable journal and reconciliation primitives** | Independent SQLite, project locks, phase-intent transitions, sanitized receipt outbox, crash/restart inspection using fakes | Ambiguous state blocks apply; no automatic replay; transactional/file durability tested |
| **4 — Read-only Compose preflight and candidate verification** | Trusted source/config reconstruction, immutable OCI/platform checks, supported-feature eligibility, exact image-only diff | CLI validates one eligible service; unsupported config, drift or untrusted inputs fail closed |
| **5 — Controlled apply and manually approved recovery** | Targeted immutable pull, scoped Compose replacement, verify, commit, interrupted-execution handling and separate recovery operation | Full end-to-end disposable-host success/failure/crash matrix; no sibling service mutation |
| **6 — GUI integration and release readiness** | Proposal/review, Activity receipts, docs/backup/recovery runbook, accessibility, audit/CI gates and separately approved test-VPS acceptance | All security tests and exact-commit release gates pass; operator explicitly approves each subsequent step |

## Phase 0 — Decisions to lock before coding

Proposed Phase 0 decisions and open validation evidence are tracked in the [Phase 0 executor security and compatibility record](../architecture/beta6-phase0-decisions.md) ([issue #80](https://github.com/docklantern/docklantern/issues/80)). This proposal does not claim the target VPS Engine/Compose versions have been verified or authorize implementation.

### Deliverables

- A versioned **support matrix** with Docker Engine, Docker Compose CLI/plugin, Go toolchain, Linux architecture(s) and supported local Engine configuration. For first real-host validation, prefer the already-tested Linux/amd64 IPv6-only VPS; do not claim linux/arm64 runtime parity from a cross-compiled artifact alone. Monitoring image keeps its existing multi-architecture support.
- Host ownership/permission design: separate monitor-writable inbox, executor-only registrations/journal/overrides, executor-written monitor-read-only receipts; trusted Compose source owners/ancestors and safe root paths. Decide the operating-system account and explicit sudo boundary for host execution.
- Concrete **Compose allowlist** (single prebuilt-image replica; registered files/env/profiles and approved external networking) and denylist (build/hooks/socket mounts/privileged/unsupported includes/dependencies). Decide stable source snapshot/recheck procedure to prevent TOCTOU between validation and Compose execution.
- Define immutable OCI index/platform digest handling and an explicit **readiness classification**. Recommended: service with HEALTHCHECK can be marked `verified_healthy`; no-HEALTHCHECK success requires exact image/container and stable `running` evidence but is labeled `verified_running_only`, **not healthy/application-ready**.
- Propose conservative, measurable proposal/receipt/journal/output limits, retry/timeout policy and retention; specify version compatibility and how incompatible journal/schema state blocks writes.
- Backup/restore design for both monitoring and executor SQLite and committed overrides, including off-host custody and mandatory post-restore reconciliation. No VPS write operations for this planning step.

### Acceptance

- The security reviewer confirms the separation, threat-model mapping and OS ownership story.
- Every currently deferred item in the [deployment contract](../architecture/beta6-deployment-contract.md) is either resolved or recorded as an explicit beta.6 exclusion; no hidden default privileges.
- A test-host fixture strategy demonstrates safe, randomized Compose projects/networks and destructive-test guards.
- **Gate 0:** Explicit operator approval of implementation scope and GitHub issue breakdown, separate from accepting these docs.

## Phase 1 — Independent CLI boundary and distribution

### Implementation boundaries

- Add `cmd/docklantern-executor` (Go) and nonprivileged preview/status command scaffolding with version reporting. No production Docker mutation functions at this stage.
- Provision/validate the executor-owned directory layout and distinct monitor proposal/receipt mounts without changing the monitor socket access. Maintain existing hardening (`cap_drop: ALL`, read-only monitoring filesystem, no direct socket).
- Use trusted executable paths and restrictive runtime environment, fixed local Docker Engine/context, no shell and bounded subprocess output. Refuse unsupported Engine/Compose versions.
- Define a dedicated, reviewed standalone Linux binary release artifact (checksum and reproducible version identification where practical), **not** inclusion in the monitoring Docker image.
- Extend security audit/CI scope to scan the **actual executor binary and its dependencies**, not merely the monitoring container, which current release jobs validate. Do not automatically publish/install it as part of planning.

### Acceptance

- Monitoring process/container cannot execute the CLI or alter executor-protected state under tested UID/GID and mount settings.
- CLI rejects poisoned `PATH`, `HOME`, `DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`, `COMPOSE_FILE`, plugin/config paths and missing/noninteractive authorization on mutating commands.
- Checksums/versions identify the executor binary tested; monitoring resource footprint and existing proxy GET-only enforcement remain unchanged.
- Map to **SEC-01, SEC-03, SEC-05, SEC-14**.

## Phase 2 — Protected registration and proposal contract

### Implementation boundaries

- Build strict versioned JSON validation per [deployment contract](../architecture/beta6-deployment-contract.md), rejecting duplicate/unknown keys, unbounded inputs and unexpected nested fields.
- Protect host-local registration creation/editing and independently allowlist exact project, ordered sources, env/profile context, approved repository/platform and service(s).
- Load proposals by bounded opaque ID using secure directory-relative file opening; no web-provided host path, shell argument, Docker endpoint or permission.
- Add CLI preview and interactive confirmation UI **without** permission to pull/up; all monitor-observed metadata is treated as a hint and refreshed by executor.
- Define stale-plan expiry and exact plan content hash, preventing substitution after approval. Include proposal publication quotas and garbage collection.

### Acceptance

- Syntactically correct but unauthorized projects/services/repositories cannot pass.
- Historical `registrations.enabled`, monitoring label opt-in and watched-tag changes never grant deployment permission.
- Wrong schema, duplicates, oversized/malformed files, path traversals and symlink/hardlink swap attempts reject safely.
- A forged proposal cannot launch Docker writes, and CLI approval cannot be automated with `--yes`.
- Map to **SEC-01, SEC-02, SEC-07, SEC-10, SEC-12**.

## Phase 3 — Journal and recovery-state groundwork

### Implementation boundaries

- Add executor-owned SQLite versioned migrations and protected journal models: `operations`, append-only `phase_events`, `baselines`, `override_records` and `receipt_publications`.
- Persist phase intent and stable operation identifiers; introduce project-level **cross-process** locking and block work with unresolved prior state.
- Atomically publish redacted receipts, preserving idempotency across interruption without exposing raw Compose output or environment values.
- Implement inspect-only `status` and `reconcile` over a **fake Docker/Compose backend**. Reconcile must not rerun a pull, recreate a service, promote overrides or mutate Docker.
- Define filesystem sync and override promotion intent/result semantics; make interrupted journal/override states explicit, including an old backup restored against newer Docker state.
- Add backup and controlled restore exercises for protected state on a disposable host.

### Acceptance

- Crash injection at each journal transition leaves durable evidence; uncertainty becomes `needs_reconciliation`, never falsely `succeeded`.
- Concurrent processes cannot mutate the same registered project through DockLantern; unrelated projects' locks remain independent.
- No raw secrets or unbounded child-process output in journal/receipts.
- Restore of stale executor DB or managed overrides blocks new apply until inspection.
- Map to **SEC-08, SEC-09, SEC-10, SEC-12, SEC-13**.

## Phase 4 — Read-only Compose planning and OCI preflight

### Implementation boundaries

- Reconstruct effective Compose config with **approved host file order, working directory, profiles, env interpolation and committed override**.
- Validate referenced inputs/parent ownership and the exact supported Compose feature subset. Reject unknown/unsafe constructs; use a stable-source strategy safe against file replacement and supported by real Compose behavior.
- Implement a normalized effective config comparison permitting **only selected service `image`** to change.
- Resolve an approved **public immutable OCI image** for the correct platform, tracking repo/index/platform manifest distinctions and separating OCI digests from local image configuration IDs.
- Inspect actual Docker project/service identity before local preview and immediately before later side effects. Detect external drift; no mutation or automatic adoption.
- Build disposable read-only integration tests for paths, Traefik-style external networks, env files/profiles and misleading Docker labels.

### Acceptance

- Supported single-service fixture produces a deterministic effective-config diff and exact candidate identity.
- Invalid source, service identity, topology/feature, file owner, digest/platform, unavailable registry or changed state fails closed before pull.
- Only the selected service image differs; unrelated labels, mount paths, networks, ports, environment, dependencies and services remain identical.
- IPv6-only registry access limitations are surfaced as explicit eligibility failure, not fallback to unintended credentials/registries.
- Map to **SEC-04, SEC-05, SEC-06, SEC-07, SEC-11, SEC-16**.

**Gate before Phase 5:** Operator-approved design and passing Phase 1–4 security suites plus disposable-host boundary tests. The executor must never be enabled on a real VPS during this gate.

## Phase 5 — Targeted apply, verification, and manual recovery (disposable hosts only)

### Implementation boundaries

- Under durable lock and new terminal confirmation, independently re-run all protected registration/actual state/immutable candidate checks.
- Journal intent **before** a Docker pull; pull the immutable candidate; create and validate operation-specific image-only override; then invoke strictly scoped `docker compose up` using tested args, no dependency recreation, build, volume removal or `down`.
- Inspect actual resulting container/image ID/platform and running/HEALTHCHECK status. Differentiate image replacement, healthy readiness and `verified_running_only`.
- Promote committed desired override **only after** successful verification. Journal promotion and publication gaps require inspection/reconciliation, not blind replay.
- Add explicit `recover` as a **new** locally confirmed operation using previous verified immutable image/override if available, never application-data rollback or automatic recovery.
- Test failed pulls, failed up, timeout, health failure, changed source, outside Compose interference, interrupt/KILL at all critical phases and missing previous image.

### Acceptance

- One authorized container is replaced with the exact approved digest; sibling service/container IDs, mounts, labels and networks remain unchanged on disposable fixtures.
- Failure **before** replacement preserves target; failure **after** replacement is recorded accurately, including ambiguous and unhealthy outcomes.
- The committed override cannot falsely represent unverified desired state; duplicate CLI invocation does not replay a half-finished update.
- Manual recovery requires fresh confirmation and verifiably restores the previous image where feasible; documentation explicitly disclaims database/volume rollback.
- Map to **SEC-04, SEC-06, SEC-07, SEC-08, SEC-09, SEC-10, SEC-11, SEC-15, SEC-16**.

## Phase 6 — GUI integration, full regression, and separately gated release

### Implementation boundaries

- Service Details: distinguish **monitoring enrollment** from **deployment eligibility**; provide exact candidate, diff and an explanation that the host-local CLI performs final approval.
- Images/Image Details: show OCI evidence and a path to eligible related services; avoid any implied automatic update or cross-service Update All.
- Activity: consume executor-written **sanitized read-only receipts**, accurately show proposed/authorized/applying/verified/failed/reconciliation/recovered; stale/missing receipts show unknown or pending.
- No GUI-to-executor API, daemon, scheduled job, sudo bridge or shell command handler.
- Supply operator installation, ownership, binary version verification, registration, backup and restore, approval, recovery and uninstall instructions, explicitly identifying Docker write privileges.
- Extend workflows to test the **released host executor artifact**, not only the monitoring container, with security tests and a strict release gate before any tag; include kernel/platform and Compose CLI support policy.
- Review beta.5 monitoring compatibility: watched-tag save/reset, Check Now, SQLite v5, accessibility, image-first navigation, performance, GET-only proxy, 3 monitored hello services and exact version reporting.

### CI/security acceptance before any VPS work

- Existing `go test -race ./...`, `go vet ./...`, integration/packaging jobs and all monitoring-image/proxy/Go vulnerability checks pass on the exact release candidate.
- New executor binary vulnerability/SBOM/provenance checks and mandatory **SEC-01 through SEC-16** suites pass on disposable hosts.
- Off-host backups and tested restoration **of monitor and executor state** documented; restore cannot silently mark unresolved project state as healthy.
- All supported Compose CLI/Engine versions and initial Linux architecture validated; do not claim untested multi-platform parity.
- No persistent executor CPU/memory usage at idle; monitor/proxy footprint and GUI responsiveness compared with accepted beta.5 baseline, with regressions explicitly reviewed.
- A full release threat-model/manual security review records residual risks, exceptions and an explicit no-go/go recommendation.

### Separate operator-controlled acceptance and release gates

1. **Approval to prepare the test VPS:** verify current backup/restore state, baseline service container IDs/images, Docker CLI versions, IPv6 registry reachability and security proxy posture. No Docker mutations until separately approved.
2. **Approval to install the executor and enable a single hello-service test:** preferably `hello-go/web`, using an approved public immutable digest; confirm the affected service is disposable/test-only and contains no irreplaceable data. Never target Traefik or DockLantern itself.
3. **Approval to run update and recovery:** demonstrate exact selected-service replacement, preserved other hello-service IDs, correct journal/Activity receipts, interrupted-operation visibility and manually approved restoration.
4. **Approval to merge, tag, publish and deploy:** each is an independent action after final source-commit CI/security evidence and operator review. A version tag may trigger automated publication—do not push a tag as a preview.

The accepted beta.5 test VPS backups were integrity-checked locally but **not restored**, and off-host custody was deferred. Those exceptions do **not** carry over as acceptable beta.6 executor-release exceptions.

## GitHub tracking breakdown (created; implementation not authorized)

[Umbrella #79](https://github.com/docklantern/docklantern/issues/79) and nine implementation-planning issues [#80–#88](https://github.com/docklantern/docklantern/issues/80) were created for dependency tracking only. Each implementation phase/PR, merge, release and VPS action still requires separate explicit approval.

| Proposed ticket | Depends on | Proposed ownership/scope |
|---|---|---|
| [B6-00](https://github.com/docklantern/docklantern/issues/80) — Close security design/open contract decisions | ADR 0008 and PR #77 documents | Support matrix, Compose allowlist, source stability, host ownership, readiness/backup policy |
| [B6-01](https://github.com/docklantern/docklantern/issues/81) — Standalone Go executor skeleton and artifact packaging | B6-00 | Host-local CLI, fixed Docker environment, protection boundary, binary scan |
| [B6-02](https://github.com/docklantern/docklantern/issues/82) — Protected registrations, strict proposals and CLI approval | B6-01 | Untrusted handoff, typed contracts, allowlist, expiry, TTY |
| [B6-03](https://github.com/docklantern/docklantern/issues/83) — Durable journal, locks and inspect-only reconciliation | B6-01/B6-02 | SQLite migrations, fsync, sanitized receipts, crash tests |
| [B6-04](https://github.com/docklantern/docklantern/issues/84) — Immutable OCI + read-only Compose preflight | B6-02/B6-03 | Source validation, image-only effective diff, runtime drift detection |
| [B6-05](https://github.com/docklantern/docklantern/issues/85) — Scoped Compose apply and manually confirmed recovery | B6-03/B6-04 | First authorized Docker mutations on disposable fixtures |
| [B6-06](https://github.com/docklantern/docklantern/issues/86) — GUI proposals, Service/Image Details and Activity | B6-02/B6-03 and stable receipt contract | No privileged web execution, image-first accessible UI |
| [B6-07](https://github.com/docklantern/docklantern/issues/87) — Adversarial/compatibility and exact-artifact CI security | All features | SEC-01–SEC-16, actual host binary audit, regression and release checks |
| [B6-08](https://github.com/docklantern/docklantern/issues/88) — Release, backup restore and test-VPS operator acceptance | All prior phases and separate approvals | Runbooks, off-host backup+restore, service test, version/release gates |

These links are cross-referenced issue-tracking records, not GitHub native sub-issue relationships. Issue creation authorizes **neither implementation nor merges, releases, or host operations**.

## Release definition of done

A beta.6 candidate is **not releasable** unless all of the following are evidenced for the exact final commit and released executor artifact:

- The monitor retains its GET-only proxy and cannot launch/use Docker writes.
- One independently approved registered project/service can be safely updated to an immutable candidate via **host-local interactive confirmation only**.
- No unauthorized Compose configuration or sibling service is modified in the disposable integration test.
- Operation journal, committed override and actual Docker image reconcile after success, failure, crash and restore; ambiguous states block new updates.
- Manual recovery works where the prior image is available; no false promise of application-data rollback.
- Secret-safe proposal/receipt/output paths and all threat-model acceptance tests pass.
- Off-host backup and completed restore exercise are documented before a real-host execution gate.
- Independent CLI binary packaging/scanning and existing monitoring security scans pass.
- Release notes disclose supported Compose subset, platform limits, residual risks, required privileges and recovery limitations.
- The operator separately approves any remaining GitHub merges, release tag/publication, executor installation and VPS Compose test.

**No scope expansion through opportunistic PRs.** Private registry execution/credential persistence remains deferred to [issue #62](https://github.com/docklantern/docklantern/issues/62) or later planning.
