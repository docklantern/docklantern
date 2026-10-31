# Beta.5 Phase 4 — Regression and test-VPS acceptance

Tracking [#68](https://github.com/docklantern/docklantern/issues/68) under [beta.5 roadmap #63](https://github.com/docklantern/docklantern/issues/63). **This is a plan, not a record of VPS tests performed.**

## Automated acceptance (disposable CI only)

CI is the required test boundary for Docker/Compose actions. It runs on disposable GitHub-hosted Docker runners; do not run the destructive integration scripts against a shared Docker daemon or the test VPS.

- [ ] On the **exact final candidate commit**, `go test -race ./...` and `go vet ./...` pass, including beta.5 cross-phase watched-tag/Check Now regression tests.
- [ ] The Go POC workflow completes all jobs: unit, Compose/discovery proxy, monitoring flow, packaging, beta installation/backup, onboarding, and release-input checks.
- [ ] The Beta security audit completes all jobs on the exact candidate commit: pinned-toolchain `govulncheck`, application-image Trivy, and patched Docker proxy-image Trivy. Phase 4 changes the workflow trigger so new Go-source PRs receive a security audit before merge.
- [ ] Manually review that the Docker proxy grants only necessary **GET** endpoints, no public listener and no POST/PUT/DELETE Docker API; DockLantern does not mount a direct Docker socket. Existing disposable proxy tests must pass.
- [ ] Verify API, Origin/CSRF, HTML escaping, pause/opt-out, shared-image/platform isolation and watch persistence tests have passing evidence. Explicit multi-credential scopes remain deferred to [#62](https://github.com/docklantern/docklantern/issues/62).
- [ ] Release Docker build inputs and CI support both `linux/amd64` and `linux/arm64`. **Do not tag, publish or deploy** as part of this acceptance PR.

## Test-VPS readiness — not yet authorized

The operator's test host is IPv6-only, has Cloudflare/Traefik, a restricted Docker socket proxy, and three hello services (Python, JavaScript and Go). Running beta.4.1 is **not** evidence of beta.5 acceptance. The operator must separately approve any on-host backup, migration, test, restart or deployment; merely merging this documentation or CI PR is not authorization.

Before any future approval, record:

| Field | Baseline / evidence |
|---|---|
| Current running image/version | Operator capture of immutable digest, container image ID, binary version |
| Application Compose overlay set | Expected `compose.yaml` + `compose.discovery.yaml`; verify actual active files |
| Target services | Record container IDs and image IDs for hello-py, hello-js, hello-go |
| Current SQLite | Schema version, database location from active Compose, available free space |
| Backup and rollback | Consistent backup, `PRAGMA integrity_check` result, protected copy and tested restore plan |
| Security boundary | Socket proxy GET-only permissions, no public Docker API, unchanged Traefik routing |
| Registry reachability | IPv6-only egress to configured registries; no assumption that GitHub/GHCR is IPv6-reachable |
| Performance baseline | Idle CPU/RSS, sample page latency, normal registry check cadence |
| Operator approvals | Separate sign-off for backup/migration, test-VPS deployment and test actions |

### Read-only baseline commands (operator-run when approved)

Run from the DockLantern deployment directory, after confirming the active project and overlays. No credentials or private Compose environment should be pasted into public reports.

```sh
docker compose -f compose.yaml -f compose.discovery.yaml ps
CID="$(docker compose -f compose.yaml -f compose.discovery.yaml ps -q docklantern)"
docker inspect "$CID" --format 'Image={{.Config.Image}} ImageID={{.Image}} Created={{.Created}}'
docker exec "$CID" docklantern-inventory -version
# Inspect exact running container IDs/images for the three hello services
# using their actual Compose project directories or read-only docker inspect.
# Avoid docker compose up, pull, restart, down, prune or volume operations.
```

The Docker `exec` command above starts a short read-only version process inside the **existing** container; do not run a migration against the live database during baseline capture. Do not assume the SQLite file is safe to copy directly while active; DockLantern's online `-backup` CLI is the supported consistent backup mechanism, and must be run only after explicit approval.

### Proposed post-upgrade smoke matrix — requires separate approval

- [ ] A **consistent verified SQLite pre-upgrade backup** is captured before the forward-only v3→v4→v5 migration; rollback reverts both image **and matching database snapshot**. The beta.4.1 on-VPS-only backup did not demonstrate off-host recovery.
- [ ] On the actual beta.5 image, `/readyz` and all authenticated Overview, Images, Image Details, Services, Service Details, Activity, Settings routes pass through Traefik/Cloudflare; confirm legacy API compatibility, Origin and CSRF rejection, no credential/path leakage.
- [ ] On a safe test service, change its watched tag to a valid tag **in the same repository**; verify the deployed Docker image reference and container ID remain unchanged; check observed/watch provenance, history links and shared-image/platform grouping.
- [ ] Request **Check Now** on Service Details and Image Details. A 303 `scheduled` redirect only means *queued*. Verify later observation through the background worker, cooldown, `Retry-After`, and no duplicate event generation; do not force registry traffic while a backoff is active.
- [ ] Reset watch target to the Docker-observed tag and check that enrollment, pause, label opt-out, interval and previously recorded image-scoped history are preserved.
- [ ] Browser test desktop/tablet/mobile widths; keyboard navigation, focus, labels, readable typography, responsive forms and accessible confirmation notices.
- [ ] Compare target hello-service **container IDs before/after**; no target containers should be recreated or restarted by monitoring operations.
- [ ] Measure bounded idle memory/CPU and several authenticated page latencies on the actual low-resource IPv6-only host. Capture concrete samples, not anecdotal pass/fail claims.
- [ ] Verify credentials/registry errors are not leaked in activity logs or HTML. Confirm read-only socket proxy restrictions still apply.
- [ ] Record any deferred edge cases, restore drill, external backup or load checks explicitly, along with operator approval of the remaining test-host risk.

## Hard release gates

At this stage **no VPS checks have been performed**, no database backups or migrations have been made, and no publication or deployment is authorized. Close #68 only after both the code/CI evidence and the operator's accepted VPS/deferral disposition have been recorded. Phase 5 [#69](https://github.com/docklantern/docklantern/issues/69) separately governs tagging/publishing and later deployment, each requiring explicit approval.
