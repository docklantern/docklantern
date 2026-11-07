# DockLantern v0.1.0-beta.6 — gated test-VPS acceptance runbook (B6-08)

**Status (2026-11-07): disposable-VPS G5/G6 execution and monitoring-RC validation complete; G7 tag/publication remains UNAUTHORIZED.** Issue [#88](https://github.com/docklantern/docklantern/issues/88). The operator waived off-host backup custody and restore testing **only for this disposable test host**. This evidence does not close untested production-readiness or issue #88 acceptance criteria.

This runbook covers the existing IPv6-only Linux/amd64 GCP test VPS, currently running beta.5 monitoring with separate `hello-py`, `hello-js` and `hello-go` Compose projects. The only contemplated beta.6 execution target is a **separately approved, disposable/test `hello-go/web` image**, not DockLantern, Traefik, the socket proxy or application data. The monitor remains GET-only.

## Operator decision gates

| Gate | Requirement | 2026-11-07 test-VPS outcome |
| --- | --- | --- |
| G0 — Reviewed source | Exact source/CI/security review | **CI PASS** at `f26d2c9aaf62494700433a77e74e02a64c1a4621`: 17/17 Go/integration and 4/4 security jobs; reviewed executor artifact hash recorded. The new docs merge commit will require fresh release-head checks. |
| G1 — Baseline | Read-only Docker, Compose, image/container, mount/network and proxy inventory | **PASS** for the recorded disposable VPS baseline. |
| G2 — Backup + restore | Off-host custody and independent restore | **EXPLICITLY WAIVED FOR THIS DISPOSABLE VPS ONLY**; local schema-v5 monitoring snapshot integrity verified. No off-host custody or independent restore drill. **Production blocker remains.** |
| G3 — Host CLI install | Separately authorized audited executor, protected host layout | **PASS** for installed executor `/usr/local/libexec/docklantern-executor` with SHA-256 recorded below. |
| G4 — Qualification | Protected registration and independently qualified immutable proposal | **PASS**; registered `hello-go-beta6/web`, candidate digest, source and running baseline matched. |
| G5 — Live test apply | One separately authorized target-only host-local TTY apply | **PASS**; operation `apply-be7c0588e024e4648dee3885` committed, no second apply. |
| G6 — Evaluate | Docker identity, protected journal, siblings, monitoring UI and Activity | **PASS for exercised scope**: replacement running, journal succeeded, six sibling IDs unchanged, beta.6 RC GUI 7/7 and receipt-unavailable fail-closed display. **Receipt-mount/positive Activity integration NOT TESTED.** |
| G7 — Publication | Independent release-head review and explicit tag/image publication approval | **PENDING / NOT AUTHORIZED**. No `v0.1.0-beta.6` tag or published release image. |

**Stop at any failed gate.** G0–G4 are not an implicit grant for G5 or G7.

## G1: read-only baseline (only after G1 authorization)

Record UTC time, shell account, commands/outputs and the current deployed DockLantern image reference and digest. The following commands are observational; run against the already-known deployment directories after confirming paths, with no Compose file edits:

```sh
date -u +%FT%TZ
uname -s -m
id
docker context show
docker version
docker compose version
docker ps --no-trunc --format '{{.Names}}|{{.ID}}|{{.Image}}|{{.Status}}'
docker inspect docklantern-docklantern-1 \
  --format 'ImageRef={{.Config.Image}} ImageID={{.Image}} Started={{.State.StartedAt}}'
docker inspect docklantern-docker-socket-proxy-1 \
  --format 'ImageRef={{.Config.Image}} ImageID={{.Image}} Ports={{json .HostConfig.PortBindings}}'
```

Record separate `docker inspect` snapshots of the selected hello service and **all siblings within its Compose project**: container IDs, image IDs/references, health, labels, mounts and networks. Save evidence only in operator-controlled storage; inspect output for secrets before sharing. Check Traefik reachability, IPv6 registry access, GET-only socket proxy and current monitoring GUI without changing images. The existing no-POST proxy rule must remain unchanged.

**Source-owner blocker:** Phase 0 recorded a `0775` ancestor under `~/deployments/hello-go`; the beta.6 executor rejects group-writable source ancestry. Confirm the *current* full path, all ancestor owners and modes, regular files, and trusted .env inputs. **Do not chmod, chown, migrate or register sources automatically.** Choose and separately authorize a compliant host-owned source layout before G4. Legacy `/settings/projects` registration is monitoring-only, not executor authority.

**Runtime blocker:** the last reported VPS combination was Engine/Client 29.8.1, API 1.56 and Compose v5.5.1, Linux x86_64. Recheck it; exact matching version strings do not alone prove CI host parity.

## G2: off-host backup and witnessed restore drill (only after G2 authorization)

Before copying any files, document a tested **consistent SQLite snapshot** procedure: the monitoring DB is live and may use WAL, so copying `docklantern.db` alone is not a reliable backup. Use the application's supported SQLite backup path or an independently validated SQLite online-backup method. Capture executor journal, registrations, committed desired generations and protected overrides only if they exist; **absence of uninstalled executor state is not a successful executor restore test**. Never expose raw `.env`, registration secrets or image credentials in logs.

1. Inventory snapshot source, SQLite schema version, journal/WAL status, source owners/modes and exact service/image version; take a consistent backup without stopping or editing unrelated containers.
2. Produce cryptographic SHA-256 manifests, transfer encrypted/access-restricted backup material to an **independent off-VPS destination**, and verify its copied hashes. Backups remaining in `~/deployments/docklantern/backups` do **not** satisfy off-host custody.
3. On a separate disposable system/path, restore **from the off-host copy**, verify SQLite `PRAGMA integrity_check` and application-visible data, ownership and journal/override consistency. A successful integrity check alone is insufficient.
4. Exercise a deliberately stale protected-journal/override snapshot against newer **simulated/disposable** Docker evidence; ensure it blocks new apply and requests operator reconciliation, not silent success or replay. Do not create drift in the live project.
5. Record timestamps, method, source/copy/restored hashes, destination custody (without credentials), restoration results and operator sign-off. Retain a verified beta.5 monitoring rollback image **plus its matching pre-upgrade DB**.

If the executor is not yet installed, perform the executor-specific restore component using a disposable protected-state fixture first and repeat relevant checks after an approved installation.

## G3–G4: artifact and trust acceptance (separate approvals)

Inspect the final-commit [B6-07 evidence](../security/beta6-release-evidence.md) and CI artifact `beta6-executor-linux-amd64-security-evidence`; match `source_commit` and `executor.sha256` to the binary *actually proposed for installation*. CI artifacts are ephemeral and **not published releases**. Do not install a mismatched or unaudited rebuild.

Before any root-only CLI use, verify root ownership of trusted binary, source ancestors, registration files, journal/locks and mount boundary. The monitor may only have a constrained proposal handoff and **read-only** sanitized receipts; no executor or raw Docker socket in the monitor image. If source inputs cannot be rendered from stable protected snapshots, deny registration/qualification.

The independently host-created proposal is versioned, limited-life, untrusted data; the GUI **cannot** mint qualified proposals. Treat registry watch tags and digest differences as hints, not deployment permission. Use `docklantern-executor version`, `diagnose`, `preview`, `qualify` only as allowed by the CLI's actual usage and the specific host approvals; never paste unverified CLI flags or host paths from monitoring metadata.

## G5–G6: one authorized immutable hello service change

Before G5, record the exact **approved Docker Hub repository, linux/amd64 platform digest, proposal ID, registered project and service**, as independently qualified by the root executor; ensure the selected fixture is volume-free and not host-critical. Confirm the original `hello-go` target image and its siblings from current observations, no external Compose writer and required readiness policy (with HEALTHCHECK or explicitly authorized `allow_running_only`).

After **new, explicit G5 approval**, invoke only the root-host executor's interactive `apply <proposal-id>` from a genuine local/SSH controlling terminal. The command must revalidate the trusted registration, OCI/Compose/source/actual baseline, display a nonce-bound exact-image challenge, and block on any drift. Never use a web POST, cron task, unattended flag, or direct Compose change as a substitute.

On return, do not infer success from exit code alone: compare observed target image **ID/digest**, running/health classification, unchanged sibling IDs/images/labels/mounts/networks and committed journal record. Compare any sanitized Activity receipt to actual Docker state and timestamp. Capture the operator's observations and failures, not raw secret-bearing output.

On any failure or interruption **STOP**: the first beta.6 CLI does not implement `recover` or mutating `reconcile`. `status <operation-id>` is read-only. Do not run a second `apply`, manually change the journal, or assume rollback. Any repair/restore needs a new, separate approved plan. Application volumes/database effects are never rolled back automatically.

**Managed override limitation:** the host executor records desired-image generations in protected SQLite and supplies the image override through a private Compose stdin stream. User-authored YAML remains untouched; an independent Compose up using only original YAML may revert the container image. Record this prominently during acceptance.

## G7: release decision

Review a final CI run and binary hash from the **same final source commit**, SEC-01–SEC-16 results, off-host restore evidence, actual VPS outcome and unresolved issues. The [beta.6 release-note draft](../releases/v0.1.0-beta.6.md) is not a release authorization. Release workflow is tag-triggered and publishes monitoring images to GHCR/Docker Hub; a GitHub release can be created automatically. Do **not** create a tag or start that workflow without new approval.

### Acceptance evidence record — witnessed 2026-11-07

The following is a dated **operator-reported disposable-VPS test record**, not CI's claim of universal production support. All G5 authorization was specific to one disposable `hello-go-beta6/web` update; no further applies, automatic recovery, release tags, or unrelated deployments were authorized.

| Evidence | Recorded observation |
| --- | --- |
| Reviewed source | `f26d2c9aaf62494700433a77e74e02a64c1a4621` (PR #133); at the time of the test this was `main`. |
| CI/security at reviewed source | [Go POC: 17/17 jobs passed](https://github.com/docklantern/docklantern/actions/runs/41907889810); [Beta security audit: 4/4 jobs passed](https://github.com/docklantern/docklantern/actions/runs/41907889788). These precede the documentation-only release-prep merge. |
| Installed executor | `/usr/local/libexec/docklantern-executor`; SHA-256 `1bd2a8afa155e87306af3849a346fd4d6b4bf1fd864ba7e5413b3e2cd81657bb`. Host: Ubuntu 24.04, Linux/amd64, Engine 29.8.1/API 1.56/Compose 5.5.1. |
| Trusted registration | `hello-go-beta6`, `web`, `single-service-prebuilt-v1`; registration SHA-256 `652427a71f4999d2f97175c4d026d4ed11434f6ae6f6e119f9cdc7f2330b7efe`; running-only readiness explicitly accepted (no HEALTHCHECK). |
| Fresh G5 proposal | `hello-go-beta6-g5-73429d9499`, expires `2026-11-07T20:27:03Z` (not reusable); proposal SHA-256 `8c11042db252b1579c1ef464140b9e0386d2da345a43859c83f4528363138131`; preview and independent `qualify` succeeded. |
| Candidate and source | `docker.io/davidslusser/hello-go`, tag hint `1.0.5`; OCI index `sha256:2784c9e97340a33997905ad651c034fe11dba9f3a1ab8a2218ccffc093c1a603`; Linux/amd64 manifest `sha256:31d32eff8c58b8f7e9e0cc52a1e3d2d3b3a7b6198e3e114255e4fd8d16f0ca19`; Compose source `sha256:01a7d61008d714c634c0460921454b1d497b3dc3a1b45206feae6435276e74a6`. |
| Target before / after | Container `efeb1a37fa84a6c3ac788857b7fd9b675de552f312b100abc6341f92ab70b723` (image `sha256:88750fab0c36e4cd289ef3fd030c1328db890e15be7be203f129a8c73bcee392`) replaced by `176ec0b62eb93a3e8b2411283089c0af17ab4c944cc31af6ea1d2dda1f635629`, running `linux/amd64` at the approved digest reference. |
| Interactive apply and journal | Host-local challenge completed; `Verified immutable update committed for operation apply-be7c0588e024e4648dee3885`. Read-only `status`: phase `COMMIT`, outcome `succeeded`, `Blocks new work: false`. |
| G5 sibling check | Original `hello-go`, `hello-js`, `hello-py`, beta.5 monitoring, socket proxy and Traefik: **six pre/post container IDs identical and running**. The executor's own commit verification also checked sibling image/label/network/mount invariants at apply time. |
| Monitoring snapshot | `docklantern-pre-beta6-20261107-202748.sqlite` (80 KiB, root-owned `0600`); Python SQLite `PRAGMA integrity_check=ok`, `user_version=5`; SHA-256 `2d2a62a7bda5d0f113fa2633b858955165186df108307002cfc55abdb8df9100`. **Local VPS only**; neither off-host copy nor a restoration drill was performed. |
| Beta.6 monitoring candidate | Local workstation build from `f26d2c9` (not the exact published CI image); version `v0.1.0-beta.6-rc.f26d2c9`, `linux/amd64`, Docker archive config SHA-256 `78d36f8549313b8a726c4b2eb895dac42a57efdff5ea80b7d50a7e37890efc25`, transferred gzip archive SHA-256 `3d0326c1a1fc36793dd82dfae06dde485c3456c84efecca05149c31de8a9b232`. VPS containerd image ID `sha256:ac669e7338270b247a4a7dfd191931e58e5d97e3492f5ac618e8d179b95bfd4f`; archive config, layers, platform and tag independently matched. |
| Beta.6 monitor replacement | Separately authorized `docklantern`-only Compose RC override; new `docklantern-docklantern-1` ID `3a21f262f9c71e93f76786a834c37df32138f781157354754f0181dcfb806df6`. Image `docklantern:beta6-rc-f26d2c9`; CLI version matched; `Running=true`, `Health=healthy`. Existing Compose mounts, secrets, command, networks and proxy policy were preserved. |
| Monitoring siblings | Disposable `hello-go-beta6`, original `hello-go`, `hello-js`, `hello-py`, socket proxy and Traefik: **all six container IDs unchanged and running** after monitoring-container replacement. |
| GUI | **Seven of seven operator checks passed**: overview, images, services, settings and watched-tag retention, Deployment Activity access, correct unknown/unavailable Activity state without receipt mount, and navigation/style. |
| Positive receipt/Activity integration | **NOT TESTED**. The new beta.6 monitor has no authorized read-only receipt outbox mount; successful G5 host journal state is **not** displayed by the GUI and must not be inferred from the unknown Activity state. |
| Recovery/retention | No interrupted/uncertain update, live `recover`, manual repair or independent restore was exercised. The first beta.6 executor has no automatic rollback or `recover`. No release publication or production certification is implied. |

### Residual risks and follow-up hold points

- **Disposable-test waiver only:** no off-host backup, independent restoration drill, or production-grade recovery proof. Do not mark the corresponding B6-08/#88 criteria complete or claim production readiness.
- **No positive Activity evidence:** validating a read-only receipt mount and displayed committed operation requires a new, independently approved host configuration; the verified `unknown/unavailable` path is the only UI Activity acceptance here.
- **Compose override persistence:** executor G5 recorded desired generation in protected SQLite and passed its image override via stdin. An unmanaged `docker compose up` against the *test service's* original YAML may revert that service. The beta.6 monitor RC also uses a local three-file Compose override; omit neither that overlay nor any existing configuration while managing the monitor.
- **Release is separate:** beta.6 has **not** been tagged or published. After this docs PR is reviewed and merged, revalidate CI/security evidence and final source SHA on the **new release head**, review the intended final release notes, and obtain an independent explicit G7 authorization *before* creating `v0.1.0-beta.6`. The tag triggers image publication to Docker Hub/GHCR. Subsequent publication verification and any migration to the published image require their own approval.
- **Recovery is separate:** no automatic retry or rollback following uncertain executor state. Preserve the beta.5 monitoring image and verified pre-upgrade SQLite backup; a restore or any new Compose mutation would require separate approval.
