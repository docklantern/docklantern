# Clean VPS first-release acceptance checklist

**Plan only.** Use the [RC runbook](first-release-rc-acceptance.md) for the
acceptance contract and [existing VPS results](first-release-existing-vps-results.md)
for the prior evidence. A clean VPS is a separate approval gate, not an
upgrade of the existing host.

## Before host provisioning approval

- Select a reviewed commit and paired monitor/executor artifacts. Prefer
  transferring the already tested pair if unchanged executable sources and
  verified hashes are available; use the reviewed corrected systemd template
  separately. If artifacts are rebuilt, record the new commit, CI and exact
  hashes, verify pairing and repeat the applicable acceptance gates.
- Confirm Linux amd64, rootful local Docker 29.8.1/API 1.56 and Compose 5.5.1
  availability; do not bypass exact compatibility checks. Select host, region,
  network/IPv6 routing, SSH access, TLS and budget before creating resources.
- Review the deployment repository's host prerequisites, receipt overlay,
  secret handling and immutable image pin. Do not inadvertently run its
  older beta default in place of the RC.
- Establish backup custody and restoration evidence for both SQLite stores
  and protected source/overrides, or explicitly approve a narrow disposable
  host waiver. Never describe such a waiver as production readiness.

## Separately approved host setup and monitor installation

1. Establish SSH/OS/Docker, DNS/TLS and external networks. Verify host and
   container registry access; restrict the discovery proxy to GET-only,
   internal networking, no exposed port, no monitor raw socket.
2. Transfer and independently verify the paired RC artifacts and checksums.
   Record monitor image ID, executor SHA-256 and versions. Install only with
   approval. Fresh monitor volume and fresh executor state; do not copy
   previous VPS data into the test.
3. Provision protected directories and the sanitized outbox separately:
   root-owned 0755 outbox with trusted ancestors, files 0644, no symlinks.
   See the [deployment receipt checklist](https://github.com/docklantern/docklantern-deploy/blob/main/docs/rc-receipt-overlay.md).
   Review permissions before starting any timer.
4. Use a local RC image overlay and receipt overlay last; compare the full
   effective Compose configuration, preserve read-only discovery and
   existing security settings. Verify GUI auth, health, inventory and
   receipt availability semantics; an empty outbox is not a success receipt.
5. Provision a disposable single-service project outside user-writable
   ancestors. Record image, service, platform, source and siblings.
   Register interactively with explicit standing policy, persistent override
   and appropriate health or approved running-only verification. Install
   verified service/timer templates; run systemd-analyze verify. Leave timer
   disabled until separate approval.

## Separately approved live tests

- Establish a new baseline (remote digest, actual image/container,
  registration, journal, backups, protected sibling IDs). Enable only the
  selected instance with explicit approval.
- Publish two distinct future hello application image revisions, each with
  specific source/tag and publication authorization. Verify versioned tag
  equals watched tag, distinct index/platform manifests, timer-triggered
  update, actual image/container, journal COMMIT, override generation,
  sibling IDs and positive receipt. Verify fresh GUI inventory and Activity
  for both. Check an unchanged tick after each.
- Under separate scope approval, pause only that timer, ensure executor idle,
  recreate/restart only the disposable target through default-file Compose,
  compare protected state and siblings, resume timer on success, check
  unchanged reconciliation and GUI. Verify monitor restart/recreation
  does not grant execution authority or affect the target.
- Review RC5 negative cases against exact-commit CI and isolated fixtures.
  For any uncovered destructive tests, require isolated fixtures and separate
  approval; record actual results, not presumed coverage.

## Signoff

Record dates, source/CI/artifact hashes, platform/runtime, digest and container
identities, journal and receipt evidence, backup/restore limitations, proxy
restrictions, all exception decisions and negative-test results. No unresolved
unsafe state or unexplained missing receipt is acceptable. Obtain operator
signoff before release tag creation, publishing images or production rollout.
Website work follows first-release acceptance.
