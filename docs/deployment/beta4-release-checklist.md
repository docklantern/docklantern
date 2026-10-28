# v0.1.0-beta.4.1 — Release validation and operator acceptance

**Status: IN PROGRESS — v0.1.0-beta.4.1 is the selected release candidate; it is NOT tagged, published, or approved for a release deployment. PR #58 was separately authorized and successfully deployed to the test VPS.**  
**Tracker:** [#51](https://github.com/docklantern/docklantern/issues/51)  
**Scope:** monitoring-only image-first GUI. **Do not tag, publish or deploy on the basis of CI alone.**

The beta.3 predecessor gate [#34](https://github.com/docklantern/docklantern/issues/34) is closed. Its operator-deferred host safety checks should still be exercised during beta.4 acceptance where practical.

### Existing beta.4 tag and release-candidate provenance

- **Preserve the existing `v0.1.0-beta.4` tag and registry artifacts:** the tag points to `fede82cef7f7fd802397be5121cde471f8a21fa1` before PRs #58–#60. Its [versioned release run](https://github.com/docklantern/docklantern/actions/runs/40418851278) successfully published GHCR and Docker Hub images but skipped creating a GitHub prerelease. **Do not move, reuse, or overwrite** that tag.
- The candidate is `v0.1.0-beta.4.1`, untagged and unpublished as of this review. Matching notes are [here](../releases/v0.1.0-beta.4.1.md). Merged PR #60 ensures missing notes fail verification before any new image publication.
- After PRs #59/#60, main commit `4de9b833b04140230e72aa81356345f3b00fb99b` passed [10/10 Go jobs](https://github.com/docklantern/docklantern/actions/runs/40628465289) and [3/3 security jobs](https://github.com/docklantern/docklantern/actions/runs/40628465280). Reverify CI for the exact final merge/tag target.
- **Separate explicit operator approvals** are required for (1) release tagging/image publication and (2) subsequent digest-pinned release deployment. The local PR #58 test deployment is not beta.4.1 publication.

## Gate 1 — Code and regression checks

| Verification | Automated or source evidence | Outstanding |
| --- | --- | --- |
| Shared image/platform identity and grouping | `internal/presentation/inventory_test.go`; same summaries feed Images/Overview | Explicit credential-scope limitations documented; see Gate 5 |
| Remote change and conservative deployment status | `internal/presentation/overview_test.go`, `inventory_test.go`, `internal/updates` tests | Browser and real registry check on approved host |
| Long polling interval freshness | `TestRegistryFreshnessRespectsMonitorIntervals` | Inspect a 6–24h interval image if available |
| Images filtering, digest evidence and image-only history | `internal/settings/images_experience_test.go`, `internal/store/image_history_test.go` | Browse actual images and compare counts |
| Services grouping, controls and label opt-out | `internal/settings/services_experience_test.go`, `internal/management/services_workspace_test.go` | Browser smoke of pause/resume/override/reset; verify no restarts |
| Activity, webhook delivery and cursor paging | `internal/management/activity_settings_test.go` | Browser observation with real history |
| Settings, separate retention and notification preferences | `internal/management/activity_settings_test.go`, `notification_settings_test.go` | Validate confirmation/prune behavior against noncritical data only |
| Existing API and auth regressions | `internal/management` and `internal/api` packages | API compatibility check on approved deployment |
| Docker read-only proxy and Compose integration | `.github/workflows/go-poc.yaml`; integration tests | Confirm live proxy is private/GET-only |
| No database schema additions | Source review of phase-7 changes; migration tests | Record deployed schema before/after and backup behavior |

### Required automated gates

Verify the **exact final merged commit** passes:

- `go test -race ./...` and `go vet ./...`
- Existing Go POC workflow jobs (including image-proxy, Compose, install/upgrade, zero-touch onboarding and monitoring).
- Required security audit: reachable Go vulnerabilities and container/proxy scanning; review any unresolved HIGH/CRITICAL alerts.
- Existing release-input verification and image-build constraints.
- API/HTML authentication, origin checks, CSRF, HTML escaping and no Docker mutation capabilities.

CI results on a PR commit are not sufficient to certify a different final merge commit without reviewing its associated checks.

## Gate 2 — Browser and accessibility acceptance (human/operator)

Review at desktop (~1440px), tablet (~800px) and mobile (~390px). Capture screenshots or brief notes; do not guess visual quality from passing Go template tests.

- [ ] Five main destinations work, exactly one sidebar item active; direct detail URLs and breadcrumbs render correctly.
- [ ] Overview counts match Images (same watched-image identity) and Images to Review links to real details.
- [ ] Images search/filter/sort handles empty matches, different platforms, mixed/unknown assessments, and long image names.
- [ ] Image Details shows full digest evidence and paged image-scoped history without attributing shared-image events to a random service.
- [ ] Services groups each project/service once despite multiple replicas; undiscovered/unmonitored and disabled-by-label states are clear.
- [ ] Service Details forms work with real authentication and CSRF; override/reset, pause/resume and explicit `monitor=false` behave correctly.
- [ ] Activity date groups and filters work; previous/observed digests and expandable delivery state are readable.
- [ ] Settings shows global Monitoring, Notifications, Retention and Advanced; old direct service-prefill deep links still work.
- [ ] Keyboard-only navigation reaches every relevant link, filter, button, form and native disclosure control with visible focus.
- [ ] Mobile view has no clipped navigation/forms; tables scroll horizontally when necessary and do not conceal required actions.
- [ ] High-contrast/status colors remain readable, especially failures, Unknown and disabled states.

### PR #58 test-VPS acceptance — confirmed by operator (2026-10-27)

[PR #58](https://github.com/docklantern/docklantern/pull/58) merged at `5d1bc5fa2af472cd604e69ac248fd3b86e0fe706`, with [10/10 Go checks](https://github.com/docklantern/docklantern/actions/runs/40601983136) and [3/3 security checks](https://github.com/docklantern/docklantern/actions/runs/40601983487). The operator authorized a **test-VPS-only** upgrade, verified the pre-upgrade SQLite snapshot (`integrity_check: ok`), and built/deployed `docklantern:beta4-pr58-5d1bc5fa` on amd64. Embedded version: `v0.1.0-beta.4+5d1bc5fa`.

- [x] Dashboard typography/contrast, sidebar build version, Docker-observed image + inventory timestamp versus separately persisted registry watch reference accepted by operator.
- [x] Desktop, mobile (~390px), tablet (~800px), keyboard navigation and focus visibility spot-checked and accepted; this is not a formal accessibility certification.
- [x] Verified proxy has no published host port, private internal Docker discovery network, no direct app socket mount, writable SQLite volume, read-only legacy hello-js/hello-py mounts and administrator secret. Live `GET /containers/json` returned **200** and `POST /containers/create` returned **403**.
- [x] Readiness PASS. Three idle app+proxy samples over ~24 seconds showed **32.53–32.73 MiB combined memory** and near-zero CPU. This is not a sustained/load or page-latency benchmark.
- [x] Pause/resume tested on two hello services; operator confirmed monitoring active afterward. All three hello containers were running with zero restart counts in paused-state snapshot and no diff was reported across snapshots (no observed recreation/restart).
- [x] Earlier beta.2 image display was traced to a legacy persisted watch policy and corrected; it was not proof of a stale running container.
- [ ] Record or explicitly defer remaining targeted edge-case checks (mixed platform, stale/failing registry checks, explicit label opt-out, override/reset, API compatibility) in #51.
- [ ] Confirm protected **off-host** backup custody and restore/rollback drill disposition; capture page latency if required for final sign-off.

Safe read-only host diagnostics are in the [running-image reference guide](../operations/beta4-running-image-reference.md). The assistant cannot certify live host or visual acceptance from passing CI alone.

## Gate 3 — VPS operating and data checks (only after explicit deployment authorization)

The test-VPS checks below applied to the approved PR #58 local build, **not** a final digest-pinned release deployment:

- [ ] Confirm the published beta.3 deployment is healthy; record the current pinned image digest, exact Compose overlays, proxy configuration, and SQLite schema.
- [x] Pre-upgrade SQLite backup completed and integrity returned `ok`. **Still pending:** off-host copy and a recorded restore/rollback drill or an explicit operator-approved deferral.
- [ ] Verify the deployment Compose config with the exact active overlays, network aliases and Traefik routing.
- [x] Private GET-only Docker proxy enforcement, network isolation, no app Docker socket mount and read-only legacy binds checked on test VPS (GET 200 / POST 403). This does not test every endpoint.
- [x] Idle CPU/memory and `/readyz` baseline recorded; **page response time, SQLite file size and sustained/load behavior remain unmeasured**.
- [ ] Only after `v0.1.0-beta.4.1` publication is separately approved/completed, review a digest-pinned deployment PR and obtain **new explicit deployment approval**; recreate DockLantern only.
- [ ] Check authenticated Overview, Images, Image Details, Services, Service Details, Activity, Settings, `/readyz`, selected `/api/v1/*` endpoints and unchanged monitored target containers.
- [ ] Compare CPU/memory/page responsiveness with beta.3. Log any unacceptable slowdown and revert the app using the previous verified image if needed.

Never run disposable integration scripts against the actual VPS. Follow the separately managed [deployment repo](https://github.com/docklantern/docklantern-deploy) for controlled installation.

## Gate 4 — Data and compatibility

- No SQLite table/index migration is intended for beta.4; preserve current data directory/volume and WAL ownership.
- Existing enrolled services, GUI/label precedence, pause/resume state and last-known OCI digest observations must survive upgrade.
- Event history is immutable observation data except explicit delivered-only pruning. A changed registry digest is not proof of a newer release.
- Pending webhook events survive manual retention pruning.
- Service/registry history, settings form targets, and JSON API paths must retain backward compatibility.

## Gate 5 — Registry authorization-scope limitation

The beta.4 UI and SQLite historical key currently group by **canonical OCI reference + platform**. The monitor process has only a **single configured credential host/account tuple**; the in-memory registry cache additionally isolates anonymous and authenticated lookup results by effective scope.

**Beta.4 does not support multiple independent per-service credential scopes for the same image**, and persisted history cannot distinguish a credential-scope change over time. Do not claim scope isolation across a rotated credential identity. Before deploying under a new registry account with different visibility, verify/re-baseline observations; do not interpret change events across account contexts as unambiguous release changes.

Track any future credential-scope-aware persistence as an architectural feature with its own migration and security acceptance, rather than a silent expansion in beta.4. Review [#45](https://github.com/docklantern/docklantern/issues/45).

## Gate 6 — Explicit release authorization and rollback

Only after Gates 1–5 are reviewed:

- [ ] Resolve/document known exceptions and security findings on [#51](https://github.com/docklantern/docklantern/issues/51).
- [ ] Obtain explicit operator approval **to create the new `v0.1.0-beta.4.1` tag and publish** its GHCR/Docker Hub images and GitHub prerelease from an accepted final-main commit. Never reuse or move the existing `v0.1.0-beta.4` tag.
- [ ] Verify actual GHCR/Docker Hub manifest digests and `linux/amd64`, `linux/arm64` support; record SHA, provenance and scan evidence.
- [ ] Obtain **separate explicit approval to deploy**, review a digest-pinned deployment PR and preserve current Traefik/target-service configuration.
- [ ] If monitoring/GUI regression occurs, stop the new app, restore prior pinned digest and verify existing SQLite schema compatibility. Do not downgrade blindly after any future schema migration; restore a matching snapshot if schema changes occurred.
- [ ] Record post-deploy smoke evidence, operator disposition and final release notes.

**Current state:** Operator dashboard, mobile/tablet/keyboard, read-only socket proxy, idle resource, backup integrity, and basic pause/resume checks are recorded as accepted above. Off-host backup/restore disposition, remaining edge-case/API and page-latency evidence, and release approval still require sign-off in #51. **No `v0.1.0-beta.4.1` tag, prerelease, image publication or release deployment is authorized.**
