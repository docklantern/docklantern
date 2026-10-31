# v0.1.0-beta.4.1 — Release validation and operator acceptance

**Status: COMPLETED WITH ACCEPTED EXCEPTIONS — v0.1.0-beta.4.1 was published and subsequently operator-reported running on the test VPS.** Earlier checklists below retain historical unverified/deferred items; **unchecked does not mean passed**, and the operator explicitly accepted those gaps for this beta.
**Tracker:** [#51](https://github.com/docklantern/docklantern/issues/51) — closed as completed on October 31, 2026  
**Scope:** monitoring-only image-first GUI. Any **future** release tag, publication or VPS deployment requires separate explicit operator approval.

The beta.3 predecessor gate [#34](https://github.com/docklantern/docklantern/issues/34) is closed. Its operator-deferred host safety checks should still be exercised during beta.4 acceptance where practical.

### Existing beta.4 tag and release-candidate provenance

- **Preserve the existing `v0.1.0-beta.4` tag and registry artifacts:** the tag points to `fede82cef7f7fd802397be5121cde471f8a21fa1` before PRs #58–#60. Its [versioned release run](https://github.com/docklantern/docklantern/actions/runs/40418851278) successfully published GHCR and Docker Hub images but skipped creating a GitHub prerelease. **Do not move, reuse, or overwrite** that tag.
- [v0.1.0-beta.4.1](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.4.1) was tagged and published on October 28, 2026 from commit `358f0d7e90082164b23c9df1c0b66dec48651f85`, using [version-matched release notes](../releases/v0.1.0-beta.4.1.md). Release workflow PR #60 required nonempty version-specific notes before publication.
- Exact final release commit `358f0d7e90082164b23c9df1c0b66dec48651f85` passed [10/10 Go jobs](https://github.com/docklantern/docklantern/actions/runs/40629747695) and [3/3 security jobs](https://github.com/docklantern/docklantern/actions/runs/40629747668). [Release workflow 40630568959](https://github.com/docklantern/docklantern/actions/runs/40630568959) succeeded and logged GHCR and Docker Hub pushes for linux/amd64 and linux/arm64 with manifest index digest `sha256:54dcdfc315cf55738e26084baa2aff97dc8ea15fc3dcca8e895af922f58a0092`.
- The operator separately approved release tagging/publication and later reported beta.4.1 deployed on the test VPS. The PR #58 local test deployment was a different build; separate approvals remain mandatory for future release/deployment operations.

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

### Historical PR #58 test-VPS acceptance — confirmed by operator (2026-10-27)

[PR #58](https://github.com/docklantern/docklantern/pull/58) merged at `5d1bc5fa2af472cd604e69ac248fd3b86e0fe706`, with [10/10 Go checks](https://github.com/docklantern/docklantern/actions/runs/40601983136) and [3/3 security checks](https://github.com/docklantern/docklantern/actions/runs/40601983487). The operator authorized a **test-VPS-only** upgrade, verified the pre-upgrade SQLite snapshot (`integrity_check: ok`), and built/deployed `docklantern:beta4-pr58-5d1bc5fa` on amd64. Embedded version: `v0.1.0-beta.4+5d1bc5fa`.

- [x] Dashboard typography/contrast, sidebar build version, Docker-observed image + inventory timestamp versus separately persisted registry watch reference accepted by operator.
- [x] Desktop, mobile (~390px), tablet (~800px), keyboard navigation and focus visibility spot-checked and accepted; this is not a formal accessibility certification.
- [x] Verified proxy has no published host port, private internal Docker discovery network, no direct app socket mount, writable SQLite volume, read-only legacy hello-js/hello-py mounts and administrator secret. Live `GET /containers/json` returned **200** and `POST /containers/create` returned **403**.
- [x] Readiness PASS. Three idle app+proxy samples over ~24 seconds showed **32.53–32.73 MiB combined memory** and near-zero CPU. This is not a sustained/load or page-latency benchmark.
- [x] Pause/resume tested on two hello services; operator confirmed monitoring active afterward. All three hello containers were running with zero restart counts in paused-state snapshot and no diff was reported across snapshots (no observed recreation/restart).
- [x] Earlier beta.2 image display was traced to a legacy persisted watch policy and corrected; it was not proof of a stale running container.
- [x] Remaining targeted edge-case checks (mixed platform, stale/failing registry checks, explicit label opt-out, override/reset and API compatibility) were **explicitly accepted as beta exceptions** in [#51](https://github.com/docklantern/docklantern/issues/51), not represented as completed tests.
- [x] The operator explicitly accepted an **on-VPS-only** SQLite snapshot (integrity check `ok`) and deferred off-host custody, a full restore drill and representative page latency. Neither disaster recovery nor benchmark performance was established.

Safe read-only host diagnostics are in the [running-image reference guide](../operations/beta4-running-image-reference.md). The assistant cannot certify live host or visual acceptance from passing CI alone.

## Gate 3 — VPS operating and data checks (only after explicit deployment authorization)

The older performance and proxy spot checks below were performed on the separate PR #58 test build. Final released beta.4.1 operator evidence is distinguished below; those earlier checks are **not** a sustained benchmark of the release:

- [ ] Full before/after baseline for beta.3, SQLite schema and complete production-equivalent proxy configuration was not independently recorded for final release deployment; accepted as a beta limitation.
- [x] Pre-upgrade consistent SQLite backup passed integrity checking. The operator approved **deferring** an off-host copy and complete restore drill; the backup remained on the VPS.
- [x] Operator reported active Compose files as `compose.yaml` and `compose.discovery.yaml` (old `compose.pr58.yaml` inactive); the full effective configuration, networking and Traefik mapping were not independently re-audited for the release.
- [x] Private GET-only Docker proxy enforcement, network isolation, no app Docker socket mount and read-only legacy binds checked on test VPS (GET 200 / POST 403). This does not test every endpoint.
- [x] Idle CPU/memory and `/readyz` baseline recorded; **page response time, SQLite file size and sustained/load behavior remain unmeasured**.
- [x] The operator reported installing and running published beta.4.1. **After** that deployment, [deploy repo PR #5](https://github.com/docklantern/docklantern-deploy/pull/5) aligned its image pin to the published digest; [merged-commit Compose CI](https://github.com/docklantern/docklantern-deploy/actions/runs/41230929425) succeeded. PR #5 was **not** a prerequisite applied before the operator-led VPS deployment.
- [ ] On the released build, operator confirmed Docker image ID matching the release index digest, embedded version `DockLantern v0.1.0-beta.4.1`, `/readyz` PASS and all three hello services running. The hello-js restart was **intentional**. A complete authenticated page/API sweep and before/after target IDs were **not** independently verified.
- [ ] Beta.4.1 versus beta.3 sustained resource usage, response latency and full rollback testing were **not** measured; explicitly accepted for this test beta.

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

Credential-scope review issue [#45](https://github.com/docklantern/docklantern/issues/45) was closed after documenting the unsupported beta.4 case. Scope-aware persistent observations require a future architecture, migration and security review under [#62](https://github.com/docklantern/docklantern/issues/62), separately from planned [beta.5 #63](https://github.com/docklantern/docklantern/issues/63).

## Gate 6 — Explicit release authorization and rollback

Only after Gates 1–5 are reviewed:

- [x] Operator reviewed and documented accepted exceptions in [closed issue #51](https://github.com/docklantern/docklantern/issues/51).
- [x] Operator explicitly approved creating `v0.1.0-beta.4.1`; final-main tag published both registry images and the GitHub prerelease. Earlier `v0.1.0-beta.4` tag and image artifacts were preserved.
- [x] The completed release workflow logged the same multiarch index digest at GHCR and Docker Hub for `linux/amd64` and `linux/arm64`, with successful security jobs. **Direct independent registry-resolved inspection was not separately recorded**.
- [x] The operator reported running beta.4.1 on the test VPS. The deployment repo was then aligned to the published digest in merged [PR #5](https://github.com/docklantern/docklantern-deploy/pull/5); this does **not** mean PR #5 preceded the actual VPS update or that full effective-config validation was performed.
- [ ] If monitoring/GUI regression occurs, stop the new app, restore prior pinned digest and verify existing SQLite schema compatibility. Do not downgrade blindly after any future schema migration; restore a matching snapshot if schema changes occurred.
- [x] Operator recorded post-deploy binary version, image index ID, `/readyz` PASS and all three hello workloads running; accepted limitations and [final release gate #51](https://github.com/docklantern/docklantern/issues/51) were documented.

**Final disposition (2026-10-31):** The published beta.4.1 release is operator-reported ready on the test VPS, and [release gate #51](https://github.com/docklantern/docklantern/issues/51) is **closed with explicitly accepted beta exceptions**. Earlier PR #58 acceptance established browser/keyboard and read-only proxy spot checks on a **different** local test build. An off-host backup and full restore drill were **deferred**. Exhaustive registry edge cases, full API/browser sweep, sustained latency/performance, complete config re-audit and before/after target container identity verification were **not demonstrated**, and must not be called passed. [Deployment repo PR #5](https://github.com/docklantern/docklantern-deploy/pull/5) is merged; [beta.5 planning](https://github.com/docklantern/docklantern/issues/63) is future work requiring separate approvals.
