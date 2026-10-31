# Beta.5 release preparation and operator gates

**Status: DRAFT / PRE-PUBLICATION.** Proposed version `v0.1.0-beta.5`, awaiting explicit operator approval. There is **no implied authorization** to create a tag, publish any image, upgrade SQLite, restart the app, or deploy it on the VPS.

**Trackers:** [umbrella #63](https://github.com/docklantern/docklantern/issues/63), [phase-4 acceptance #68](https://github.com/docklantern/docklantern/issues/68), [release gate #69](https://github.com/docklantern/docklantern/issues/69). Use the [detailed test-VPS acceptance runbook](beta5-test-vps-acceptance.md) for operational evidence.

## Implemented phases and source provenance

| Phase | Implementation | Status |
| --- | --- | --- |
| 0 — beta.4 release docs | [PR #70](https://github.com/docklantern/docklantern/pull/70) | Merged |
| 1 — persistent watched reference | [PR #71](https://github.com/docklantern/docklantern/pull/71), SQLite v4 | Merged |
| 2 — Check Now scheduling | [PR #72](https://github.com/docklantern/docklantern/pull/72), SQLite v5 | Merged |
| 3 — Service/Image Details UI | [PR #73](https://github.com/docklantern/docklantern/pull/73) | Merged |
| 4 — cross-phase regression, audit trigger, acceptance runbook | [PR #74](https://github.com/docklantern/docklantern/pull/74) | Merged; **VPS acceptance pending** |

PR #74's merge commit `8b5eb568297562e6630a62596a5fd58b1968c725` passed [Go POC](https://github.com/docklantern/docklantern/actions/runs/41237468844) and [Beta security audit](https://github.com/docklantern/docklantern/actions/runs/41237468751). This is **baseline evidence only**; after any new merge, verify the exact final release commit again.

## Gate 1 — finalize release inputs (no publication)

- [ ] Operator confirms proposed version `v0.1.0-beta.5` and the intended monitoring-only feature scope; do not move or reuse earlier tags.
- [ ] Review the [version-matched release notes](../releases/v0.1.0-beta.5.md) for factual claims, user-facing limitations and upgrade guidance.
- [ ] Confirm the **exact** accepted final `main` commit SHA and tag-target ancestry. Verify no PR remains pending that changes the candidate artifact.
- [ ] Pass Go `go test -race ./...`, `go vet ./...`, all integration/release-input jobs and the three security audit jobs on **that exact commit**. Record workflow run links and any explicit, operator-approved exceptions.
- [ ] Confirm source-controlled release workflow requires the version-matched nonempty notes, an ancestor of main and pinned Docker base manifests. A tag push will **automatically publish** image tags and create a GitHub prerelease after checks; **never push a tag as a harmless preview**.
- [ ] Confirm the current release notes and image registries still refer to beta.4.1; no beta.5 images or GitHub release have been published prior to approval.

## Gate 2 — database safety and test-host baseline (operator-run only)

- [ ] Obtain explicit operator approval before any VPS-touching backup, migration, container recreation or smoke test.
- [ ] Verify currently active Compose files, actual DockLantern image ID/digest and binary version; capture read-only baseline for the three monitored hello services' immutable container IDs and images.
- [ ] Capture current SQLite `PRAGMA user_version` **without modifying the live DB**; expected beta.4.1 is v3 but **verify** on the host.
- [ ] Capture a consistent DockLantern application-backed SQLite backup **before** first startup with the candidate v5 binary; run `PRAGMA integrity_check` on the backup and store it separately. Explicitly decide off-host backup custody and restore-drill requirements. A prior on-VPS-only beta.4.1 backup does not prove recovery.
- [ ] Document an executable rollback plan restoring **both** the previous beta.4.1 image/digest **and** the compatible v3 SQLite database. A blind image-only rollback against v5 DB is not allowed.
- [ ] Confirm GitHub/Docker Hub/OCI registry availability from the **IPv6-only** host and existing restricted read-only Docker proxy routing. No Docker API writes, public socket, or Compose permissions are added.

## Gate 3 — separate release publication approval

- [ ] **Explicit operator authorization** to push the new version tag and publish the multiarch release images and GitHub prerelease. Merely reviewing/merging documentation or a PR does not grant this approval.
- [ ] Verify the tagged SHA equals the approved final-main commit; verify `linux/amd64` and `linux/arm64` image manifests and the published GHCR and Docker Hub immutable index digests (record exact resolved values, **do not invent them**).
- [ ] Confirm the version embedded in the published image matches the tag; preserve historical beta.4 and beta.4.1 artifacts.

## Gate 4 — separate test-VPS deployment approval and acceptance

- [ ] **New explicit operator approval** to deploy to the test VPS after approved release publication, with verified backup/rollback plans.
- [ ] Apply only the approved DockLantern image/Compose change, without automatic updates to hello-py, hello-js or hello-go. Deployment-repo digest-pin changes need their own reviewable PR and merge approval.
- [ ] Confirm `/readyz`, actual running binary version and image digest, then record all UI/auth routes, per-service tagged watch override/reset, Check Now (service and shared-image group), correct deferred/backoff messages and exact observation evidence.
- [ ] Confirm history and legacy monitoring policies survive SQLite v3→v5 migrations. Check that request cooldown survives restart.
- [ ] Verify before/after target-container IDs and immutable image IDs are unchanged; no DockLantern-driven pulls, Compose rewrites or target restarts.
- [ ] Capture mobile/tablet/desktop and keyboard smoke results, API compatibility/security, IPv6-only registry egress, short idle resource observations and representative page latency.
- [ ] Record any unperformed test explicitly. Obtain operator acceptance of any remaining backup, restore, performance or coverage risks.

## Gate 5 — project closure

- [ ] Close [phase-4 issue #68](https://github.com/docklantern/docklantern/issues/68) only with recorded VPS acceptance or explicit approved deferrals.
- [ ] Close [phase-5 issue #69](https://github.com/docklantern/docklantern/issues/69) and [umbrella #63](https://github.com/docklantern/docklantern/issues/63) only after approved publication, deployment disposition and completed acceptance documentation.

**Invariant:** DockLantern remains monitoring-only. The feature does not request Docker API write privileges, pull or redeploy target services, execute Compose overrides, rotate registry credentials per service or infer semantic software update availability from digest movement.
