# DockLantern v0.1.0-beta.6 — gated test-VPS acceptance runbook (B6-08)

**Status: PREPARATION ONLY — NO-GO for VPS executor installation, live Compose changes, or release.** Issue [#88](https://github.com/docklantern/docklantern/issues/88). Completing this document does not satisfy the gates.

This runbook covers the existing IPv6-only Linux/amd64 GCP test VPS, currently running beta.5 monitoring with separate `hello-py`, `hello-js` and `hello-go` Compose projects. The only contemplated beta.6 execution target is a **separately approved, disposable/test `hello-go/web` image**, not DockLantern, Traefik, the socket proxy or application data. The monitor remains GET-only.

## Operator decision gates

| Gate | Requirement | Status |
| --- | --- | --- |
| G0 — Reviewed source | Merge/security checks on the **exact final main commit**; inspect SEC-01–SEC-16 evidence, high-severity vulnerability findings and CI failure history | **Pending** |
| G1 — Baseline | Separate approval to collect read-only host state (Docker/Compose versions, service IDs and digests, ownership, network/proxy restrictions) | **Pending approval** |
| G2 — Backup + restore | Separate approval to create consistent monitoring SQLite and executor-state backups, copy and verify **off-host**, and demonstrate restoration into a separate disposable environment; never restore over live state for a drill | **Blocking; not performed** |
| G3 — Host CLI install | Approve the exact audited `linux/amd64` executable SHA-256, trusted installation path, ownership, and protected directory/mount layout | **Pending approval** |
| G4 — Qualification | Approve protected registration and trusted source root, independently inspect exact proposal and candidate digest/platform via host CLI; no Docker write | **Pending approval** |
| G5 — Live test apply | **Separate approval** for the exact service, immutable candidate digest and one-time host-local TTY `APPLY`; record before/after observations | **Pending approval** |
| G6 — Evaluate | Operator inspects journal status, receipt vs Docker actual state, sibling invariants, monitoring GUI, and any failure/uncertainty | **Pending** |
| G7 — Publication | Independently approve beta.6 release notes and tag/publish, knowing a GitHub tag automatically publishes images | **Pending approval** |

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

### Acceptance evidence record

| Field | Operator-entered evidence |
| --- | --- |
| Final reviewed main SHA / CI run IDs | Not recorded |
| Audited executor SHA-256 + binary source | Not recorded |
| VPS observed Engine/API/Compose + host architecture | Not revalidated for beta.6 |
| Off-host backup location/hash and independent restore report | Not performed |
| Trusted Compose root/source owner/mode check | Pending; prior 0775 ancestor blocker |
| Approved proposal ID, target repository/platform digest | Not approved |
| Target/sibling before and after snapshots | Not performed |
| Journal phase/outcome and sanitized Activity evidence | Not performed |
| Operator approval for each G0–G7 action | Not granted |
| Release decision | **NO-GO** |
