# Beta.6 — Host Executor Threat Model

**Status:** Proposed security baseline for review; not implemented or validated  
**Date:** 2026-11-01  
**Architecture:** [ADR 0008](../architecture/decisions/0008-on-demand-compose-executor.md)  
**Companion contract:** [Beta.6 deployment contract](../architecture/beta6-deployment-contract.md)

## Purpose and security objective

Evaluate what changes when DockLantern introduces explicit, manually approved Compose execution after the monitoring-only beta.5 release. This is a design-level threat model, not an audit, exploit test or assertion that the executor exists.

**Primary objective:** compromise of the monitoring web process, its SQLite database, its registry observations, or its proposal inbox must **not by itself** authorize or perform Docker mutations or obtain host-administrative execution.

Secondary objectives: prevent deployment outside registered projects/services; prevent accidental source/desired/actual drift; protect Compose secrets and executor state; make partial execution detectable and recoverable only with fresh operator approval.

## Assets and authority

| Asset | Why it matters | Trusted owner |
|---|---|---|
| Docker Engine and running Compose services | Mutation can replace workloads, access volumes and escalate to host control | Host administrator |
| Host Docker/Compose executable and plugins | Command execution and lifecycle semantics | Host administrator/package manager |
| Approved deployment registrations and policy | Determines which project/service/image can be mutated | Host executor/administrator |
| Original Compose files, env files, profiles and referenced inputs | Configure privileged actions and secrets during recreation | Independently trusted project owner |
| Committed and operation-specific image overrides | Carry last committed desired image and candidate image | Host executor |
| Operation journal, locks and reconciliation records | Preserve evidence after crash, replacement or restore | Host executor |
| Monitoring database and observed OCI metadata | Useful suggestions; not authorization | Monitoring application, lower trust |
| Proposal inbox and result receipts | Cross-boundary exchange; untrusted inbound and sanitized outbound | Monitor writes proposals; executor writes receipts |
| Host operator terminal and SSH account | Confirms a specific, newly validated operation | Authorized human operator |
| OCI registries and candidate images | Supply deployment bytes; not intrinsically trusted or safe | External systems |

No promise of host isolation is possible while an executor holds write access to a rootful Docker daemon. An image pinned by digest is immutable with respect to that digest, **not** necessarily benign.

## Scope and attacker assumptions

### In scope

- A web-session or server-side compromise of the monitoring process.
- Forged/modified deployment proposals, registry evidence or monitoring SQLite entries.
- Malformed paths, symlinks/hardlinks, file replacement and time-of-check/time-of-use (TOCTOU) races in lower-trust directories.
- Changed source Compose inputs or stale runtime observations.
- Untrusted OCI manifests, mutable tags and unexpected platform resolutions.
- Docker/Compose environment, executable, plugin or daemon-context confusion.
- Inadvertent operator misapproval, resource exhaustion, external Compose interference, interrupted updates and leaked secrets.
- Restore of executor SQLite or override files inconsistent with Docker actual state.

### Outside the guaranteed boundary

- A compromised host root account or Docker daemon.
- A compromised authorized host operator/SSH session or trusted Compose source owner.
- A malicious container image that an informed operator explicitly approves and deploys.
- Full application-data rollback or prevention of changes made by the updated application.
- Remote/multi-host engines, privileged GUI execution, unattended automation and private-registry credentials (excluded from beta.6).

These exclusions identify residual risks, not reasons to bypass control checks.

## Trust boundaries and data flows

1. **Monitoring boundary (lower trust).** The existing authenticated GUI, registry monitor and monitoring database continue to use a private GET-only Docker socket proxy. No direct Docker socket, Docker write action, executor binary, privileged callback or executable IPC channel is allowed in the monitor.
2. **Proposal boundary.** The monitor can publish versioned, bounded **data** into a dedicated writable inbox. An executor treats these bytes as untrusted. Proposal identifiers are not tokens or permission grants.
3. **Local authorization boundary.** The operator invokes a host-installed binary in an interactive local/SSH terminal. The executor checks its protected registration, reconstructs trusted Compose inputs, independently previews the proposed operation and requires confirmation of exact project, service and candidate digest.
4. **Executor state boundary.** Only the executor/authorized host administrator may change registrations, locks, private journal, override files, trusted executable/configuration or result receipts. Only the receipt output is readable by the monitor, and it is never a control channel.
5. **Docker mutation boundary.** The executor has effectively host-administrative privileges while invoked; every Docker/Compose argument, daemon endpoint, environment variable and eligible service is constrained by executor-owned configuration.
6. **External content boundary.** Registry responses and image bytes are untrusted; revalidate OCI digests/platforms and approved repository identity before an operation. Image immutability does not establish publisher trust.
7. **Recovery boundary.** The journal, override and Docker actual state may disagree after failure. Reconciliation reads actual state without automatically replaying mutations or claiming success.

## Threat register

Severity reflects qualitative impact for a single-host DockLantern installation, not measured exploitability.

| ID | Threat and likely entry | Severity | Required mitigation | Verification |
|---|---|---|---|---|
| T01 | Compromised GUI submits a forged update | Critical | No execution path from monitor, host-local interactive authorization, protected allowlist | SEC-01 |
| T02 | Proposal uses traversal, symlink/hardlink or rename race to escape inbox | Critical | Root-owned parent, separate mount, safe descriptor-relative reads, regular-file/inode checks, strict identifiers | SEC-02 |
| T03 | Monitor writes executor configuration, override, journal or receipt files | Critical | Distinct owners/mounts/permissions, no overlapping writable directory ancestry | SEC-03 |
| T04 | Untrusted Compose input changes mounts, hooks, privileges or side effects | Critical | Trusted ownership, supported-feature allowlist, normalized effective-config diff and fail-closed checks | SEC-04 |
| T05 | Host `PATH`, Compose plugin, daemon context or inherited env hijacks command | Critical | Trusted absolute binaries, checked plugin, fixed Unix socket/context, minimal env, typed args and no shell | SEC-05 |
| T06 | Executor follows a mutable tag or wrong platform image | High | Immutable repository digest, platform match, independent registry verification, no silent substitution | SEC-06 |
| T07 | Source or running target changes between GUI preview and execution | High | Executor-owned fresh preflight, registration/config/actual fingerprints, expiry and terminal preview | SEC-07 |
| T08 | Two invocations or external Compose/CI operations conflict | High | Per-project cross-process lock, state checks before mutation, unresolved-operation block; no atomicity claim for external tools | SEC-08 |
| T09 | Process crashes after pull/stage/apply/verify | High | Journal intent before each side effect, durable state and manual reconciliation without auto-replay | SEC-09 |
| T10 | Compose config, Docker/registry output or receipts expose secrets | High | Never emit raw merged config/env, bounded sanitized errors, restrictive journal/receipt access | SEC-10 |
| T11 | Malicious/untrusted candidate image executes harmful code once deployed | High | Approved existing repo, explicit digest and human review, disclose no image-safety certification | SEC-11 |
| T12 | Oversized/flooded proposals or CLI output exhaust VPS disk/memory | Medium | File count/size/time/output quotas, stream/cap results, cleanup and safe errors | SEC-12 |
| T13 | Backup restore leaves journal/override out of sync with Docker | High | Off-host backup and restore drill, restore generation checks, mandatory reconciliation before new apply | SEC-13 |
| T14 | GET-only monitoring proxy becomes writable or publicly exposed | Critical | Regression checks on allowed endpoints, denied mutations, unpublished ports and isolated network | SEC-14 |
| T15 | A failed health check is reported as success or safe rollback | High | Distinguish running/healthy/readiness, inspect actual image, block ambiguous outcomes, manual recovery only | SEC-15 |
| T16 | Host filesystem source validation is bypassed by trusted-path replacement | Critical | Trusted parent ownership and file stability, safe opens, exact source hash/recheck; reject unsafe races | SEC-16 |

## Required security controls

### Authorization and independence

- A monitor proposal cannot carry arbitrary host file paths, Docker commands, flags, registry credentials or new permissions.
- The executor uses a separate approved registration. Historical monitoring `registrations.enabled` and label opt-in are never promoted to deployment authority.
- Local preview and `apply`/`recover` commands require an interactive TTY and confirmation bound to the validated registration, service, immutable digest and current baseline. A proposal ID is not proof of identity or authorization.
- Do not provide `--yes`, background worker, daemon, SSH command endpoint exposed by the web tier, or privileged web API in beta.6.
- An OS administrator can deliberately invoke the executor; the system does not defend against that administrator being compromised.

### Filesystem isolation and parsing

- Use a root- or trusted-admin-owned ancestor. Mount only the isolated proposal directory read-write into the monitoring container; mount receipts read-only. Never mount the executor state parent into the monitor.
- Use distinct UID/GID ownership appropriate to the deployed host. Document the actual privileges/`umask`; do not depend on mode bits alone to resist an already privileged container.
- Accept exact schema versions; reject unknown fields, duplicate JSON keys, malformed UTF-8, unexpected types, oversized nesting, extra trailing JSON and noncanonical identifiers.
- Open proposals relative to the trusted inbox directory using safe, no-symlink semantics; do not parse paths supplied in proposals as filesystem locations. Validate file type, ownership, link count and size from the opened descriptor.
- Prevent name-based TOCTOU by securely opening trusted sources and validating stable identity/content. If the Compose CLI cannot safely consume the validated inputs without a race, require trusted non-writable source owners and fail closed when stability is not assured.
- Protected registrations, SQLite/SQLite WAL files, override directories, binaries and receipt-output directories must be unmodifiable by the monitor.

### Compose and Docker execution

- Resolve the local Docker Engine explicitly. Sanitize inherited `DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`, `COMPOSE_FILE`, `COMPOSE_PROFILES`, `COMPOSE_PROJECT_NAME` and other Compose/CLI-changing variables.
- Use a validated, trusted absolute Docker executable and Compose plugin; invoke fixed arguments via Go `exec.CommandContext`, never a shell. Bound execution time and captured output. Reject version drift outside the tested support matrix.
- Reconstruct the approved source model in the correct order, interpolation context, working directory and profiles. Validate both the source-only/committed and candidate-effective configurations; the selected image field is the sole authorized semantic change.
- Eligibility is an allowlist. Initially reject untested `build`, privileged service settings, Docker socket mounts, Compose hooks, ambiguous includes/extends, complex dependencies, replicas and other constructs that might cause extra mutation.
- Resolve immutable image identity by approved repository and verified target platform. A remote digest difference is not equivalent to a safe upgrade.
- Before Docker mutation, validate actual current project/service/container state against the newly displayed approval. A project-level lock prevents competing DockLantern executors; it **cannot** prevent an external administrator/CI job changing Compose concurrently.
- No `down`, volume removal, prune, automatic build, unrelated service recreation or image-adoption side effect.

### Journal, recovery and information disclosure

- Journal phase intent **durably before** pull, staging, replacement and override promotion. Because SQLite, filesystem renames and Docker Engine cannot participate in one atomic transaction, treat crash gaps as explicit reconciliation cases.
- The committed override remains unchanged until verification. A failed post-apply operation can leave actual state different from committed desired and must block new deployments pending reconciliation.
- Recovery is a distinct, locally reviewed and confirmed operation; it is best-effort image restoration, not rollback of persistent application state.
- Store minimal redacted metadata; never place raw Compose config, resolved environment values, auth headers, secrets or unbounded child stdout/stderr into proposals, receipts or GUI views.
- Protect and restore both monitoring and executor SQLite consistently (including SQLite WAL/sidecars or backed-up snapshots). On restore, do not assume journal/actual image agreement.

## Verification gates before executable beta.6 code is enabled

| Test | Expected outcome |
|---|---|
| SEC-01 — Forged monitor proposal; monitor attempts executor launch | No Docker write, unauthorized operation or executable control path |
| SEC-02 — Traversal, symlink/hardlink, rename, duplicate JSON keys, malformed/nonregular/oversized proposal | Fail closed without reading/writing protected paths |
| SEC-03 — Attempt from monitor UID/container to edit registrations, override, journal, receipt | OS and mount permissions deny writes |
| SEC-04 — Malicious effective Compose configuration or image-only diff violation | Service not eligible; no Docker mutation |
| SEC-05 — Poison `PATH`, `HOME`, Docker/Compose env and plugin context | Correct trusted executable/daemon selected or operation rejected |
| SEC-06 — Tag moved, digest mismatch, platform mismatch, inaccessible IPv6 registry | Exact approved digest only or no apply |
| SEC-07 — Source/env file or target changed since GUI proposal or CLI preview | Revalidation fails; fresh approval required |
| SEC-08 — Parallel executor and external Compose interference | One DockLantern operation proceeds; conflicts detected or marked uncertain |
| SEC-09 — Inject crash at each durable phase boundary including after Docker replacement | No blind replay; accurate reconciliation workflow |
| SEC-10 — Secret-bearing Compose input and failing child output | No secret appears in UI, receipts, test artifacts or logs |
| SEC-11 — Operator shown exact existing repository and immutable image, with explicit trust disclaimer | No unexplained upgrade or signature guarantee |
| SEC-12 — Repeated large files, output flood, slow registry, disk exhaustion | Bounded resources and safe failure |
| SEC-13 — Restore old journal/override backup while Docker has newer image | Reconciliation required before apply |
| SEC-14 — Proxy GET success/POST denied, no published socket-proxy port, isolated network | Monitoring boundary unchanged |
| SEC-15 — No HEALTHCHECK, unhealthy healthcheck, restart loop, successful image replacement | Verified outcomes accurately distinguish running vs healthy vs uncertain |
| SEC-16 — Source file or ancestor replaced during validation/Compose execution | Operation rejected unless trusted source stability can be demonstrated |

All mutating tests run only on disposable Docker-enabled hosts, with explicit environment guards and isolated random project names. A separate operator decision is required before any test-VPS execution. Passing these tests does not certify production security.

## Residual risks and release blockers

**Residual risks:** rootful Docker daemon authority; trusted host/Compose-owner compromise; malicious image content; noncooperating external Compose actions; application migrations/volume side effects; human operator approval error; untested Compose behavior; inability to atomically coordinate Docker/filesystem/SQLite.

**Release blockers:** no proven monitor/executor privilege separation; untrusted Compose source ownership; unsupported/changed Docker Compose CLI semantics; unresolved crash reconciliation; raw-secret disclosure; no off-host backup **and completed restore drill**; missing disposable-host adversarial tests; registry image identity ambiguity; or altered GET-only proxy permissions.

## Review questions before implementation

1. Which host administrator UID/GID will own the executor state and trusted Compose roots, and how will rootless/user-namespace installs be handled?
2. Which Docker Engine and Compose CLI versions/platforms are supported for the beta.6 release?
3. What concrete Compose feature allowlist and stable-source validation method can be demonstrated on Linux?
4. What limits apply to pending proposals, CLI output, receipt retention and audit-log rotation?
5. What operator-visible behavior is acceptable when a target has no Docker HEALTHCHECK?
6. How will backup restore generation and current Docker state be reconciled after disaster recovery?

These are implementation-contract and acceptance decisions, not permission to enable privileged execution.

**Governance:** This document is a proposed security baseline. A docs-only PR may be reviewed independently. No implementation, issue creation, merge, release publication or VPS deployment is authorized by this document.
