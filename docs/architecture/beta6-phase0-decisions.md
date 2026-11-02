# Beta.6 Phase 0 — Executor Security and Compatibility Decisions

**Status:** Proposed Phase 0 decision record; review/approval pending. No executor has been implemented or deployed.  
**Tracking:** [#80 — B6-00](https://github.com/docklantern/docklantern/issues/80) under [beta.6 umbrella #79](https://github.com/docklantern/docklantern/issues/79)  
**Architecture:** [ADR 0008](../architecture/decisions/0008-on-demand-compose-executor.md)  
**Security:** [Executor threat model](../security/beta6-executor-threat-model.md) and [deployment contract](../architecture/beta6-deployment-contract.md)  
**Planning:** [Beta.6 phases and acceptance](../product/beta6-plan.md)

## Decision summary and explicit outstanding gates

These are deliberately conservative first-release decisions for **one host-native manually operated executor on a local Linux Docker Engine**. The choices are a proposed support boundary, **not** claims that a version or configuration has been tested. All work stays behind independent implementation/merge/VPS approvals.

| Area | Proposed initial beta.6 decision | Evidence / remaining gate |
|---|---|---|
| Host and Engine | Linux **amd64**, local rootful Docker Engine using a fixed Unix socket; no remote, rootless, Windows, macOS, or Swarm support | Capture actual test-VPS Engine/API and Compose versions **read-only**, then validate the exact combination in disposable CI before claiming support |
| Compose | **Compose V2 plugin**, host-installed trusted binary and plugin; explicit project/options; no Compose V1, SDK, or loose minimum-version promise | Pin/test an exact plugin version per supported matrix row; reject untested versions |
| Operator | Local/SSH authenticated host administrator via trusted `sudo` and interactive TTY; no executor setuid binary, docker-group requirement, web launcher, or `--yes` | Verify real OS user/UID/GID and authorized invocation in disposable boundary tests |
| Monitor | Remains monitoring-only with private GET-only Docker proxy and no direct Docker socket or executor in the image | Regression SEC-01/03/14 |
| Files | Root/trusted-admin-owned ancestors; three nonoverlapping handoff/trusted-state mounts; protected source inputs, stable Compose execution | Owner/mount validation and TOCTOU fixture SEC-02/03/16 |
| Service eligibility | One service, one replica, prebuilt public immutable image, approved same-repository/platform, no privileged/host-sensitive Compose features | Exact feature-schema and command tests SEC-04/06/11 |
| Image identity | Pin/reverify repo + digest + platform; differentiate OCI index/platform manifest/local image config ID | Registry/Engine matrix incl. IPv6 failure SEC-06 |
| Verification | `verified_healthy` only with HEALTHCHECK success; optionally `verified_running_only` on approved no-HEALTHCHECK service | Make the weaker grade explicit in operator confirmation and receipts; SEC-15 |
| Journal | Protected SQLite and stable override generations, journal intent before side effects, manual inspect-only reconciliation | Crash/restore fault injection SEC-08/09/13 |
| Backups | Encrypted/access-controlled **off-host** monitor/executor backup plus witnessed restore before live executor testing | Beta.5 same-VPS backups are insufficient; SEC-13 |

**Unresolved data, not a hidden default:** exact Engine and Compose V2 plugin versions installed on the target VPS; permitted host deploy account and ownership of its Compose roots; actual Compose effective-config behavior for the current `hello-go` fixture. Do not close #80 or authorize Phase 1 until these are verified or an explicitly bounded substitute CI support target is approved.

## 1. Supported platform and toolchain contract

### Initial supported configuration

- Linux AMD64 only for the first **host-executor** implementation and release claim. The monitoring image's existing AMD64/ARM64 support is unaffected; host-executor ARM64 may be added only after a dedicated integration and artifact audit.
- One **local rootful** Docker Engine, addressed explicitly at its trusted Unix socket; no user-defined/remote Docker context, daemon TCP endpoint, rootless Docker, alternate runtime or multi-host.
- Host Docker CLI + Compose **V2 plugin**, both installed and secured by the host administrator. The executor uses configured absolute binary paths, inspects executable/plugin identity, and rejects unsupported versions.
- The Go build uses the repository's pinned/reviewed release toolchain, with a separately audited statically linked/bounded-dependency executor artifact. `go.mod` currently declares Go 1.23; this is not proof of an independently patched/acceptable runtime or compiler.
- Compose CLI command construction is typed, with explicit `--project-directory`, `-p`, ordered `-f` and controlled `--env-file` and `--profile` options. No arbitrary appended flags.

### Supported version matrix policy (deliberately not yet populated)

A **version family alone** is not support evidence. Support is exact tested rows of:

| Linux/arch | Engine/server/API | Docker CLI | Compose plugin | Evidence |
|---|---|---|---|---|
| Linux/amd64, disposable CI | **Pending recorded versions** | **Pending** | **Pending** | Exact version output and end-to-end security test run |
| Linux/amd64, approved IPv6-only test VPS | **Not yet captured** | **Not yet captured** | **Not yet captured** | Operator-provided read-only inventory + CI parity review |
| Linux/arm64 | Not in initial executor scope | N/A | N/A | Explicitly unsupported until independently tested |

Before any executable implementation is treated as supported, record the full `docker version` client/server/API details, `docker compose version`, architecture, selected Unix socket, tested lifecycle flags, and relevant actual Compose rendering. Freeze the accepted version(s) in the support document and CI. Upgrades to new major/minor CLI behavior are review-triggering, not automatically allowed.

**Proposed read-only host inventory** (operator-run later if approved; never expose Docker/registry credentials):

```sh
uname -s -m
docker version --format 'Client={{.Client.Version}} Server={{.Server.Version}} ServerAPI={{.Server.APIVersion}}'
docker compose version
docker info --format 'OSType={{.OSType}} Architecture={{.Architecture}}'
```

Do not run a pull, up, install, backup, or other host-changing command just to complete Phase 0. If exact target versions cannot be observed yet, Phase 1 is limited to a separately approved and pinned disposable-host matrix and must **not** claim VPS compatibility.

### Environment isolation

Construct the executor's child process environment from an allowlist rather than inherited shell values. Reject/clear `DOCKER_HOST`, `DOCKER_CONTEXT`, `DOCKER_CONFIG`, `DOCKER_CERT_PATH`, `DOCKER_TLS_VERIFY`, `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`, `COMPOSE_PROFILES`, `COMPOSE_ENV_FILES`, `COMPOSE_PATH_SEPARATOR`, `COMPOSE_REMOVE_ORPHANS`, `COMPOSE_EXPERIMENTAL` and other behavior-changing variables. Use an explicit safe `HOME`/Docker config path with **no inherited credentials**; beta.6 deploys public images only.

Disable implicit project-directory `.env` discovery where supported, and pass explicitly registered interpolation input files. Validate against the actual Compose CLI version: [Compose environment variables](https://docs.docker.com/compose/how-tos/environment-variables/envvars/). Existing `env_file` service references still require independent source-input trust validation.

A poisoned `PATH` must not change the Docker executable or plugin. If plugin identity cannot be verified, disable execution. Do not use a shell for Compose subprocesses.

## 2. Operator identity, storage ownership and execution privileges

### Recommended host layout (path names are defaults, not installed assets)

```text
/var/lib/docklantern/                      host-admin-owned ancestor, not monitor-mounted
  handoff/
    proposals/                            monitor-writable isolated bind mount ONLY
    receipts/                             executor-writable, monitor-read-only isolated bind mount
  executor/                               never mounted in monitor
    registrations/
    journal/
    locks/
    overrides/
    backups/                              protected local staging; not the off-host destination
/usr/local/libexec/docklantern-executor    host-admin-installed executable; never monitor-mounted
```

- Restrict writable mount surfaces to exact proposal or monitor `/data` locations; do **not** mount `/var/lib/docklantern` or its `executor/` subtree into the monitor.
- The monitor container currently runs with a read-only root filesystem and removed Linux capabilities, but its current Compose declaration does **not** set a dedicated nonroot `user`. Do not assume root inside a container is a safe writer to host-backed filesystem. A dedicated monitor UID is a **proposed Phase 1 hardening change**, requiring a compatible `/data` volume permissions/migration test. Mount separation and no host Docker socket remain mandatory regardless of UID.
- Executor private registration, SQLite/WAL, locks, overrides and executable are root/trusted-admin owned and inaccessible for monitor writes. The receipt directory is executor-writable but **read-only from the monitor container**; do not put receipts under a writable parent mounted by the monitor.
- Use host OS account authentication and narrowly controlled `sudo` for interactive executor launch; never install a setuid executable or open a web-triggered sudo bridge. Do not require or recommend adding the web service user to the Docker group.
- A host admin who can invoke a rootful Docker executor already has host-administrative authority. The security goal is to prevent **monitoring compromise alone** from using that authority.
- A Compose source owner may be an approved nonroot deploy user; source files, interpolation inputs and **all parent directories** must be protected against modifications by monitor/untrusted principals. Project roots in a normal operator home directory may be eligible if this can be proven; root ownership is not automatically required.

### Proposal handoff and TOCTOU policy

All inbox file names are executor-generated/validated opaque identifiers plus a fixed extension. Open proposals descriptor-relative to a trusted directory with no symlink following, verify the **opened** file type, size, link count and owner, and parse once. Avoid pathname reopens between preview and approval; compare approved exact-byte hashes. Reject unknown versions, duplicate JSON keys, trailing data, excessive nesting or paths/commands/credential fields.

Compose input stability is harder: `docker compose` normally opens source files itself, so a one-time hash check alone cannot eliminate file-replacement races. The initial implementation must either (a) use protected, stable copies of all supported source/interpolation inputs while preserving original `--project-directory` path semantics and validating references; or (b) prove trusted parent/input ownership and no monitor-writable mutation capability, then recheck just before execution and detect afterwards. Neither option protects against an independently privileged malicious host admin, which is outside our model.

Any unsupported dynamic include, file reference, generated input, remote source, or path whose stability cannot be established makes the project **monitor-only**. Demonstrate relative env files and bind-path behavior in disposable fixtures before enabling that feature.

## 3. Initial Compose compatibility allowlist

**Eligibility is per registered service and its effective project**, not every discovered container. The chosen target must be a prebuilt public image and have a deterministic immutable reference.

| Allowed initially (subject to tests) | Rejected / deferred for beta.6 |
|---|---|
| One selected `web`-like service at **replica count 1**, already observed in an approved Compose project | More than one target replica, multi-service grouped updates, Compose jobs or deployment dependencies that cause side effects |
| Static `image`, `command`, `entrypoint`, `expose`, `ports`, `restart`, environment and `env_file` entries from trusted source | `build`, `develop`, `watch`, executable lifecycle hooks (`post_start`, `pre_stop`, `pre_start`), arbitrary extension behavior |
| Approved deterministic labels (including Traefik), named external networks, static platform and ordinary health checks | `privileged`, `network_mode: host`, `pid: host`, `ipc: host`, Docker/Podman socket mounts, devices, elevated capabilities or host namespace access |
| Explicit ordered local Compose files, controlled interpolation and a tested simple profiles subset | Ambiguous `include`/`extends`, remote Compose sources, unregistered interpolation, unverified profiles/models or dynamic providers |
| Pre-existing approved service-scoped **named volumes** or trusted bind mounts **only after a separate fixture proves safe mount/path parity** | Unexpected writable host binds, sensitive system mounts, externally controlled volumes, new storage resources or any uncontrolled Compose side effects |

For the **first disposable hello-service test**, prefer an image-only **stateless** target with no persistent data; postpone approval of target services with writable persistent data until the data/recovery contract is understood. Named volumes and host bind mounts are therefore **off by default** even if their Compose keys are recognized.

Treat unsupported top-level Compose constructs as a reason to reject the project if they affect project execution. Preserve all existing other-service config exactly; never silently filter unrecognized keys. Check both the committed effective model and candidate effective model, and require the sole difference to be the selected image field.

Docker Compose can execute lifecycle hooks with independent privileges, hence the explicit denylist: [Compose service hooks reference](https://docs.docker.com/reference/compose-file/services/). Compose's configuration renderer may include secret-bearing environment variables; compare normalized structures internally and expose only field names and redacted change summaries.

**Special known fixture risk:** Existing hello Compose files have used `pull_policy: always`. Before accepting hello-go as a beta.6 target, verify that the tested Compose CLI's explicit `up --pull never` behavior (after verified immutable pull) cannot unexpectedly follow a mutable tag or trigger another pull. Reject the fixture until verified. [Compose up options](https://docs.docker.com/reference/cli/docker/compose/up/).

## 4. Image verification and approval

- Registration authorizes an **existing explicit repository** and `linux/amd64` for initial host execution. A watched tag may suggest a candidate but never broadens repository/project permissions.
- Store requested tag (human context), immutable OCI index digest when applicable, immutable **platform manifest digest**, expected platform, and selected local image config ID **as distinct identities**.
- The executor independently verifies exact repository/digest/platform before approval and before pull; no floating tag resolution after approval, cross-repository substitution, or fallback credentials.
- For a platform-specific manifest, preserve the exact digest. If a manifest index points to multiple platform-specific digests, pin and verify the intended one and record the index relationship. Avoid comparing a local image config ID to an OCI manifest digest.
- After pull and after service replacement, inspect the actual container image config ID and available verified provenance; reject ambiguous evidence and reconcile rather than declaring success.
- Docker Hub may be reachable over IPv6 on the test VPS while another registry is not; an unreachable registry fails closed. No silent GHCR fallback or unapproved registry credential use.
- A digest only establishes content identity. It is **not** signature verification, malware scanning or a promise of a newer semantic version. Private registries and signatures enforcement are deferred.

## 5. Verification, readiness and health policy

Define an operation's verified **image outcome** separately from its service readiness grade. A successful `docker compose up` return code is insufficient.

| Condition | Proposed outcome | Operator-visible grade |
|---|---|---|
| Correct candidate image/container running; configured Docker HEALTHCHECK becomes healthy within deadline | `succeeded` | `verified_healthy` — Docker-reported health, not proof of application end-to-end correctness |
| No Docker HEALTHCHECK; explicit registration allows running-only verification; correct image and stable running/restart state over 30s | `succeeded` | `verified_running_only` — weaker than application-ready, disclose in terminal and GUI |
| No HEALTHCHECK and running-only is **not** authorized | `needs_reconciliation` if service replacement occurred | No success claim; operator must inspect |
| Unhealthy, restart loop, wrong image, timeout, crash or source/actual drift after apply | `needs_reconciliation` until inspected | No automatic replay/rollback |
| Failure before replacement with definitive unchanged target state | `failed_no_change` | Original target preserved, with evidence |

Default a new deployment registration to **require HEALTHCHECK**. For disposable hello-go or other intentionally small app lacking one, require a specific `allow_running_only` registration approval and explicit CLI warning; never silently equate `running` with `healthy`.

**Proposed bounded defaults to test** (may be tuned by reviewed support matrix): 15 seconds per OCI manifest metadata request; 5 minutes pull; 3 minutes Compose apply; 2 minutes health verification; 30 seconds running-only observation; ~12 minute total operation deadline. No unattended infinite retries. Any timeout after a possible replacement is **ambiguous until reconciliation**.

## 6. Limits, storage retention and secrets

These are **initial testable policy values**, not implemented settings:

| Item | Proposed limit / policy |
|---|---|
| Proposal JSON | 16 KiB max, strict schema v1, bounded depth/field lengths, **15-minute maximum age** enforced using executor clock |
| Pending proposal inbox | At most 64 entries; excess rejected without evicting an active approved operation |
| Sanitized receipt | 8 KiB max, structured allowlisted fields, atomic write; no raw Compose config/output/secrets |
| Receipt retention | At most 1,000 published receipts and 90 days of ordinary GUI history; truncate only after durable journal confirmed and never silently conceal an unresolved operation |
| Child process captured output | At most 64 KiB in private, ephemeral diagnostic buffer; publish only fixed safe error codes and redacted summaries, not raw output |
| Execution journaling | Durable per-phase transition records and protected WAL; no automatic journal-history deletion in beta.6 |
| Disk exhaustion | Fail closed **before** a new Docker mutation when journal cannot commit; a crash/no-space after replacement triggers reconciliation and preserves existing evidence |
| Registry retries | No implicit credential fallback or unbounded retry; bounded attempts/time and explicit reason |
| Approval | Interactive TTY only, fresh preflight baseline and exact candidate confirmation for each mutation/recovery |

An executor cannot promise no disk exhaustion, but it must not proceed with Docker mutation if durable phase intent cannot be persisted. Never include cleartext secrets, merged env, Docker auth headers or child stdout in monitoring SQLite, proposals, receipts or browser-facing Activity.

## 7. Backup, restore, and operation journal consistency

**Baseline gap:** beta.5's operator-accepted SQLite backups passed integrity checks but remain on the same VPS; off-host replication and a full restore drill were deferred. Phase 0 accepts neither deferral for real beta.6 execution.

### Required pre-VPS gate

1. Produce a **consistent, version-matched** monitoring database backup using its supported SQLite backup method, plus any applicable encrypted config needed for restoration. Record source schema version, image/executor version, digest and timestamp; do not assume older v3 backup is compatible with a v5 monitoring image.
2. Back up the executor's **journal database, WAL-consistent snapshot and committed override generations** plus registration metadata as an internally consistent unit, with file manifest/hashes and a generation marker.
3. Store a protected copy **off the VPS**, with explicit operator custody and access controls. A backup copied to a directory on the same VM is not disaster recovery.
4. Restore both domains in a **disposable environment**, validate SQLite integrity/schema and file permissions, and demonstrate the executor refuses mutations until it reconciles registered sources, committed desired and actual Docker state.
5. Document how to handle a backup restored while Docker still shows a newer container/image; never auto-replay, auto-adopt, or silently mark success. Any missing executor database/journal, mismatched override generation or uncertain baseline requires manual reconciliation.
6. Document recovery when the prior immutable image has been garbage-collected or cannot be fetched: fail explicitly, do not promise rollback of databases, volumes or external writes.

### Recovery consistency policy

SQLite transactions, atomic rename of an override and Docker Engine operations are **separate durability domains**. Record phase intent before mutation; on restart inspect the actual state. If override promotion and journal completion disagree, block new operations pending operator inspection.

Executor `reconcile` is inspect-only. `recover` is a new independently authorized and journaled operation, not a hidden mode of `reconcile`.

No test-VPS backup, restore, executor installation, or update is authorized by Phase 0 planning. The operator must separately approve each real-host action.

## 8. Disposable-host security and integration fixture rules

The existing disposable Compose POC is useful evidence that multi-file overrides, Traefik-style labels, external networks and sibling-container preservation *can* work. It does **not** prove privileged executor authorization, immutable digests, durable journaling or the version matrix.

- Use dedicated ephemeral CI runners or operator-approved disposable VMs; never run mutating integration scripts on the DockLantern VPS or a shared Docker daemon.
- Require both an explicit test environment guard **and** a fresh randomized fixture project/network directory and an unambiguous isolated target marker. Reject ordinary production project names, unsafe cleanup paths and missing test guard.
- Verify Docker endpoint is the approved disposable socket/daemon, not inherited `DOCKER_HOST`/context. Restrict cleanup to fixture-owned containers/networks/temporary files; never issue global prune or project-wide `down` against an unverified project.
- Execute malicious proposal, path replacement, source ownership, CLI environment, OCI digest/platform, concurrent-executor, forced-crash, journal/override restoration, secrets and socket-proxy regression tests.
- Map acceptance evidence to the existing [SEC-01–SEC-16](../security/beta6-executor-threat-model.md) cases. Only tests that intentionally mutate Docker should execute under the disposable guard; parsing/read-only tests remain safe for standard CI.
- Capture exact Engine/Compose versions from the runner and prove that `--project-directory`, `config --format json`, `pull` and scoped `up --no-deps --no-build --pull never --wait` produce the required results **for that pinned tool version**.

## Phase 0 disposition and handoff

### Proposed design decisions ready for review

- Host-native Go CLI with **host admin + interactive TTY** approval, never web execution.
- Linux/amd64 and local rootful Engine only for initial executor claims; exact Engine/Compose V2 versions **must be evidenced** before claiming support.
- Strict monitor/executor filesystem separation and no inherited Docker/Compose environment control.
- Stateful/writable-volume and complex Compose targets off by default; first eligible service should be simple, prebuilt, trusted, single-replica and preferably stateless.
- Exact immutable repository/platform candidate; no auto fallback; no automatic rollback.
- Docker HEALTHCHECK required by default; separately approved `verified_running_only` exception for services without a health check.
- Bounded proposal/receipt/log quotas and deadlines; durable journal retained, failure closes writes.
- Off-host backup **and tested restore** required before VPS executor execution.

### Evidence still required to close issue #80

- [ ] Operator reviews/accepts the above security defaults and any intentional exceptions.
- [ ] Record the actual target Engine/CLI/Compose versions and tested disposable support row (or explicitly bound Phase 1 to a pinned **CI-only** row with VPS support still blocked).
- [ ] Confirm trusted target Compose owner/project-root stability and whether the chosen initial hello service is eligible under the feature denylist and `pull_policy` test.
- [ ] Confirm implementation-time monitor UID/mount permissions and backup custody plan can be tested without changing the current VPS.
- [ ] Review this document against SEC-01–SEC-16, record reviewer disposition and approve a separate Phase 1 start.

**Approval policy:** A documentation-only PR may be proposed under this issue. Merging it, starting Phase 1 implementation, adding Docker write code, installing binaries, taking live backups, changing Compose files or touching the VPS all require **separate explicit operator approval**.
