# v0.1.0-beta.4 — Release validation and operator acceptance

**Status: IN PROGRESS — beta.4 is running as an unreleased test-VPS build; merged PR #58 is not yet deployed there. The beta.4 release is not approved or published, and no release deployment is authorized.**  
**Tracker:** [#51](https://github.com/docklantern/docklantern/issues/51)  
**Scope:** monitoring-only image-first GUI. **Do not tag, publish or deploy on the basis of CI alone.**

The beta.3 predecessor gate [#34](https://github.com/docklantern/docklantern/issues/34) is closed. Its operator-deferred host safety checks should still be exercised during beta.4 acceptance where practical.

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

### PR #58 acceptance on the existing beta.4 test VPS

**Checkpoint (2026-10-27):** [PR #58](https://github.com/docklantern/docklantern/pull/58) was merged at `5d1bc5fa2af472cd604e69ac248fd3b86e0fe706`. The exact merged commit passed [10/10 Go checks](https://github.com/docklantern/docklantern/actions/runs/40601983136) and [3/3 security checks](https://github.com/docklantern/docklantern/actions/runs/40601983487). The operator confirmed the misleading beta.2 image reference was an old persisted monitoring policy and corrected it in Settings. **The test VPS has not yet received PR #58.** The following manual checks remain open:

- [ ] Obtain **separate explicit approval** before changing or recreating the test-VPS DockLantern container to include PR #58. A merged PR does not grant deployment approval.
- [ ] Record active Compose overlays and the current `docker inspect` container image reference, immutable image ID and created time; compare `docklantern-inventory -version` with the sidebar's **Running app** label after the approved test upgrade. The binary build version is not the Docker image tag.
- [ ] On desktop (~1440px), tablet (~800px) and mobile (~390px), inspect sidebar, tables, badges, filters, digests, helper text and activity timestamps for readable size and contrast, and test keyboard-only navigation and visible focus.
- [ ] On Services and Service Details, separately confirm the **Docker-observed container image**, latest inventory observation timestamp, and persisted **registry watch image**. They must not be presented as interchangeable; no registry watch policy is proof of a running image.
- [ ] Verify Overview/Images/Services/Activity/Settings and both details pages with real but noncritical monitoring data; compare status/count behavior with registry observations, including mixed platforms, failing/stale checks and explicit `monitor=false`.
- [ ] Check unchanged IDs, image IDs and start times for unrelated monitored containers; verify monitoring form changes do not cause pulls/restarts/recreates.
- [ ] Attach operator screenshots/notes, sanitized versions/observations, resource baseline and any accepted deferrals to [#51](https://github.com/docklantern/docklantern/issues/51). Never paste secrets or complete environment-bearing Compose output.

Safe read-only host diagnostics are in the [running-image reference guide](../operations/beta4-running-image-reference.md). The assistant cannot certify live host or visual acceptance from passing CI alone.

## Gate 3 — VPS operating and data checks (only after explicit deployment authorization)

Before changing the running DockLantern image (identify the test VPS's actual current beta.4 candidate; published beta.3 is the release/rollback baseline, not necessarily the live image):

- [ ] Confirm the published beta.3 deployment is healthy; record the current pinned image digest, exact Compose overlays, proxy configuration, and SQLite schema.
- [ ] Create/locate a consistent SQLite backup; confirm integrity and how to restore. In a small beta with recreatable test services, a **full restore drill may be explicitly deferred by the operator** (record that decision).
- [ ] Verify the deployment Compose config with the exact active overlays, network aliases and Traefik routing.
- [ ] Verify Docker socket proxy has no published host port, is reachable only over the intended private network and denies POST; no writable socket reaches the application.
- [ ] Record resource baseline: application resident memory, idle/load CPU, page response time and SQLite file size.
- [ ] Only after beta.4 images are **approved/published**, upgrade by reviewed digest-pinned deployment PR; restart/recreate **DockLantern only**, not monitored services.
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
- [ ] Obtain **explicit approval to publish/tag** `v0.1.0-beta.4`; the GitHub tag/build may then publish GHCR and Docker Hub multi-platform images through the standard controlled workflow.
- [ ] Verify actual GHCR/Docker Hub manifest digests and `linux/amd64`, `linux/arm64` support; record SHA, provenance and scan evidence.
- [ ] Obtain **separate explicit approval to deploy**, review a digest-pinned deployment PR and preserve current Traefik/target-service configuration.
- [ ] If monitoring/GUI regression occurs, stop the new app, restore prior pinned digest and verify existing SQLite schema compatibility. Do not downgrade blindly after any future schema migration; restore a matching snapshot if schema changes occurred.
- [ ] Record post-deploy smoke evidence, operator disposition and final release notes.

**Current state:** Automated checks can be performed by CI in this repository. Real-browser visual assessment, the operator's VPS, release tagging/publishing, deployment and post-deploy sign-off are **not complete** solely because this document exists.
