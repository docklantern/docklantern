# DockLantern v0.1.0-beta.3 — Implementation Plan

**Status:** Approved product scope; implementation not started  
**Date:** 2026-10-25  
**Release theme:** Zero-touch monitoring onboarding and a consistent, service-focused dashboard  
**Release posture:** **Monitoring-only**; no Docker pulls, updates, deployment authorization, or container restarts initiated by DockLantern.

## Goals and non-goals

**Primary success criterion:** After installing DockLantern once, an operator opts a Compose service into monitoring by adding only `com.docklantern.monitor: "true"` to that service's Compose labels and redeploying **that application service**. On the next inventory scan, DockLantern enrolls and monitors the service automatically. No edit to DockLantern's Compose, additional filesystem bind mounts, `-roots`, DockLantern restart, or manual GUI registration.

**Other goals:** A persisted default polling interval; optional interval labels; live, durable GUI overrides; a shared sidebar on every HTML page; a clear image/service/event status model; an upgrade that preserves all beta.2 SQLite data; fewer redundant registry requests.

**Out of scope:** Automatic or manually triggered container deployment, Compose execution, host-side executor, Docker event-stream subscription, multi-host management, standalone non-Compose discovery, arbitrary Compose editing in UI, and extensive notification integrations. These require separate authorization/security review and future scope.

**Key architecture boundary:** Monitoring may be authorized by an opt-in label or an authenticated GUI action using verified Docker metadata. **Any future deployment remains subject to separate explicit registration, source path/interpolation validation, and a restricted executor boundary.** Labels do not grant deployment rights. See [ADR 0007](../architecture/decisions/0007-label-enrollment-monitoring-only.md), which refines the monitoring portion of ADR 0003 without removing the deployment safety gate of ADR 0005.

## Work breakdown and order

| Phase | Issue | Deliverable | Dependencies |
| --- | --- | --- | --- |
| 0 | [#31](https://github.com/docklantern/docklantern/issues/31) | Schema/versioned migration, legacy ownership, SQLite fixture, backup guidance | ADR 0007 |
| 1 | [#29](https://github.com/docklantern/docklantern/issues/29) | Docker label parsing, one policy per Compose project/service, idempotent reconciliation | #31 |
| 2 | [#30](https://github.com/docklantern/docklantern/issues/30) | Persisted global default; GUI overrides, reset, pause, safe live rescheduling | #31, #29 |
| 3a | [#28](https://github.com/docklantern/docklantern/issues/28) | Shared sidebar/layout for Overview, Settings, Events | Independent; can start first |
| 3b | [#32](https://github.com/docklantern/docklantern/issues/32) and [#26](https://github.com/docklantern/docklantern/issues/26) | Service-focused status, explicit registry event attribution and delivery wording | #28, #29, #30 |
| 4 | [#33](https://github.com/docklantern/docklantern/issues/33) | Registry request deduplication, bounded work and preserved per-service state | #29 |
| 5 | [#34](https://github.com/docklantern/docklantern/issues/34) | Integration, migration, security, deployment docs, beta release gates | All above |

[#27](https://github.com/docklantern/docklantern/issues/27) is the parent product requirement for label-based onboarding and GUI interval behavior. #28 is the parent GUI layout requirement. Prefer reviewable, narrowly scoped PRs; implementation order may be adjusted when tests reveal a dependency.

## Label and enrollment contract

Minimal service configuration:

```yaml
services:
  web:
    image: example/hello-js:latest
    labels:
      com.docklantern.monitor: "true"
      # com.docklantern.interval: "10m"  # Optional
```

Labels are on **the monitored service**, not on DockLantern. Changing service labels requires that service's normal Compose recreate/redeploy; GUI or global setting changes do **not** require any container restart.

1. Supported `com.docklantern.monitor`: `true`, `false`, or absent. Invalid input is a visible error and must not activate monitoring. Default: **not monitored** unless explicitly GUI-enrolled or preserved legacy manual registration.
2. `com.docklantern.monitor=false` prevents monitoring even if a GUI user previously enabled it; show the conflict/inactive reason. `true` opts in. If the `true` label is removed and there is no separate explicit GUI enrollment, disable checks but retain history; a prior GUI *interval override* alone is not enrollment.
3. `com.docklantern.interval` is optional. Parse durations using a deterministic grammar compatible with Go `time.ParseDuration` (`60s`, `5m`, `1h`); restrict to 60–86,400 seconds. If omitted, inherit the stored global default, initially `300s`.
4. Effective interval precedence: **per-service GUI override > label > global default**. GUI has a separate **Reset to label/default** operation. Expose effective value and source through the backend and UI.
5. A persisted user pause stops scheduled checks regardless of label/default interval. Manual pause does not change label or GUI enrollment source. Show a clear reason for inactive/paused/error states.
6. Identity is Compose `project + service`, not container ID. Replicas share a monitor; conflicting labels/images/platforms within a service cause a visible validation state, not arbitrary enrollment.
7. Only reconcile a **successful, complete discovery snapshot**. Do not disable policies after transient proxy failures. Missing/stopped services must not keep generating unexpected new checks; retain state/history for reappearance. Repeated identical scans are idempotent.
8. Preserve beta.2 manually registered policies as **legacy manual** enrollment, including interval, paused state, check history and registration records. Clearly distinguish them from new GUI or label-owned enrollments and offer a non-destructive adoption path.
9. `image` and `platform` must come from verified Docker container/image metadata and canonical OCI reference parsing. Fail closed if platform cannot be determined or image reference is unsupported; don't guess architecture.

## Storage, scheduling and event contract

- Persist a schema version and perform additive, idempotent migrations, including the new global default, enrollment provenance, GUI override, and service discovery/reconciliation state.
- Migrations must preserve beta.2 registrations, policy IDs/keys, image platform digest baselines, events, delivery state, notification prefs, backoff state, and unknown-but-valid user data. No destructive rebuild of the SQLite volume.
- The worker uses persistent settings on each due-policy scan; applying a new GUI interval/global default reschedules affected policies promptly and safely without restarting DockLantern. Guard in-flight check results against policy changes (existing compare-and-set design).
- Registry identity is a canonical image reference + platform (+ applicable credential isolation). Deduplicate safe, bounded requests for shared image/platform observations while preserving per-service polling, backoff, pause, last-success and error reporting.
- A `remote_digest_changed` event describes **a remote image/platform digest movement**, not a proven newer version and not a deployment. Existing storage emits this at image scope, so the UI must no longer render empty project/service as `/ `. If showing affected services, list the actual watchers accurately instead of falsely assigning one.
- Notification `pending (0)` is distinct from registry-check status; label delivery status clearly, especially when no webhook sender is configured.

## GUI structure

- Share one server-side Go `html/template` layout for all HTML pages. Keep the DockLantern brand/sidebar visible on Overview, Settings and Events; add active link and `aria-current`; links to Overview sections should use `/#section`, not page-relative anchors.
- Preserve responsive CSS and accessibility, including keyboard focus, main landmark, mobile navigation, readable tables, and reduced-motion behavior.
- Service view/row shows project, service, image/tag, actual container health, enrollment source and inactive reason, polling source/effective interval, last/next check and digest/registry assessment.
- Admin can change global default, GUI enrollment (for an unlabeled Compose service), pause/resume and GUI interval override/reset without editing Compose source files.
- Keep service status terminology precise: **first seen**, **remote digest changed**, **unchanged**, **check failed**, and **running container image assessment** are not interchangeable.

## Migration and operational safety

1. Before upgrading a real beta.2 deployment, capture a consistent SQLite backup using existing supported backup instructions and keep the previous image tag available.
2. Upgrade on a test fixture first. Beta.3 migrations may not be reversible by downgrading the binary; restore the pre-upgrade DB snapshot for rollback. Do not promise transparent downgrade.
3. Verify existing `hello-py` and `hello-js` registration records, schedules, events and remote digest baselines remain intact.
4. Label-only services must not require source mounts or `-roots`. Keep the old `compose.projects.yaml` overlay supported for existing legacy registrations/possible future *deployment* validation, but remove it from recommended monitoring onboarding docs.
5. Do not change a live VPS, reconfigure its Traefik, pull a beta.3 image or recreate any service as part of planning or development. Deployment needs separate operator approval.

## Test plan and release gates

- Unit: label grammar, precedence, state transitions, replica conflict, canonical platform/image identity, pause/resume, idempotent reconciliation, scheduling next-due logic, stale/in-flight checks, and migration.
- HTTP/template: shared shell on `/`, `/settings`, `/events`, correct active navigation and working links, CSRF-protected settings POSTs, valid/invalid interval input, Events query filtering and pagination.
- Docker-proxy integration: bounded GET-only discovery; no extra API methods or privileges; safe behavior on transient errors and service recreation.
- Registry: digest sequence **first_seen → changed → unchanged**; one digest-change event, no duplicate, correct platform/auth isolation, bounded duplicate requests, sane retry/backoff.
- Persistence: migrate representative beta.2 SQLite dataset twice idempotently; prove no lost rows/events and settings persist across restart.
- Acceptance in test Compose stack: deploy labeled `hello-js` after DockLantern is already healthy, without changing its Compose config; monitor 5m default, label 10m, GUI 2m, reset to label; confirm DockLantern does not restart target containers.
- CI: Go unit/integration/security and multi-platform checks where feasible; build/pin beta.3 only after CI and review. Separate manual smoke test on an authorized VPS after a backup; record exact outcomes in release notes.

A beta.3 release is **not ready** until [#34](https://github.com/docklantern/docklantern/issues/34) has evidence for these gates. Clearly disclose residual limitations.

## Ship / defer

**Ship:** #27, #28, #29, #30, #31, #32, #33, #26, and #34, subject to passing test gates.  
**Defer:** automatic updates, container replacement, verified semantic upgrade detection, host executor, multi-host, Docker-event push discovery, extensive notification integrations.

## Existing merge baseline

- [Application PR #25](https://github.com/docklantern/docklantern/pull/25): merged CSRF/referrer-policy fix, expected in next built beta image.
- [Deployment PR #2](https://github.com/docklantern/docklantern-deploy/pull/2): merged reproducible beta.2 deployment and legacy source overlay. Update the deployment repo's **recommended** onboarding only after beta.3 implementation is validated.

## Implementation handoff

Start with #28 (independent layout) or #31 (migration/schema foundation). Use feature branches/PRs, review test evidence per step and **do not deploy** until explicit approval. Update the ADR and this plan only when an intentional architectural decision changes.
