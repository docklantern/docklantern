# Beta.6 accelerated delivery — release candidate track

**Date:** 2026-11-03  
**Decision:** user requested a faster beta.6 development cadence and fix-forward remediation on the test VPS, while retaining explicit PR merge, release and deployment approvals.

## Objective

Reach an operator-approved `v0.1.0-beta.6` candidate that can update exactly one already-approved public-OCI, linux/amd64 prebuilt Compose service using a separately installed host-native executor. The monitor remains GET-only and never launches an executor.

## Change in engineering cadence

- Replace the single-issue sequence of small Phase 3c PRs with **larger, end-to-end feature PRs**, each containing implementation, negative/positive tests, security checks and operator documentation.
- Develop **B6-04 read-only preflight and B6-06 GUI/Activity in parallel**. Begin design and guarded-disposable-fixture development for B6-05 alongside those tracks, but **do not execute Docker mutations on a real/VPS host** before required checks and separate operator approval.
- **Constrain the first beta.6 executor** to the registered single-replica hello-style public Docker Hub linux/amd64 service. Unsupported Compose constructs, private registries, missing OCI evidence, drift and ambiguity fail closed; they are not expanded into extra beta.6 features.
- Add the SEC-01–SEC-16 mapping/test evidence to each PR as it lands. One final exact-commit audit aggregates results and checks the standalone executor binary.
- Prefer a **single beta.6 release candidate and a fix-forward acceptance cycle** on the VPS. Do not automatically roll back or infer the application database/volume can be restored from an image-only operation.
- A fixture passing is not equivalent to real Docker integration, and a test-VPS pass is not permission for deployment elsewhere.

## Delivery batches and exit criteria

| Batch | Issues | Deliverable | Exit |
|---|---|---|---|
| A — read-only deployment preflight | #82, #84 | trusted source snapshot/recheck, normalized image-only diff, immutable OCI digest/platform evidence, read-only actual-container baseline | guarded Compose fixtures and negative cases; no CLI mutating path |
| B — on-demand executor loop | #83, #85 | host TTY approval bound to a fresh preflight, per-project lock + journal intent, targeted pull/apply/verification, durable outcomes; manual recovery is another approved operation | disposable-host end-to-end tests including interrupted/failed apply; sibling containers unchanged |
| C — monitoring/UI integration | #86 | image-first proposal review, eligibility, sanitized Activity receipts, no web execution bridge | accessibility and beta.5 monitoring regressions |
| D — release closure | #83, #87, #88 | minimum operational backup/restore and manual recovery runbooks, full security matrix, executor artifact audit, off-host restore drill, candidate notes | no unanswered critical authorization/data-loss risk; exact source/build attested |
| E — test-VPS acceptance | #88 | separately approved read-only host baseline and backups, then candidate install and one hello-service test | verified target image, unaffected sibling services and proxy; operator go/no-go |

## Non-negotiable gates

- Never merge a PR, publish a tag/image/release or install/update anything on the VPS without express operator approval.
- No executor CLI or Docker socket in monitoring container; no GUI execution bridge.
- No Docker writes without fresh independently verified Compose/OCI/actual state, trusted registration, interactive host-local approval, project lock and durable pre-side-effect journal intent.
- No silent replay/automatic rollback. Ambiguous outcomes block new work and require deliberate manual reconciliation.
- Only isolated disposable Docker fixtures mutate prior to a separately authorized VPS acceptance test.
- Require at least one independently held, verified off-host backup and **completed restore drill** before enabling real-host executor operations. Preserve live journal, WAL/SHM and override/receipt evidence; stale restores cannot silently become authoritative.
- Fix-forward on the VPS means diagnosis plus new reviewed/approved image and an additional explicitly approved deployment, **not** permission to bypass security gates or silently change services.

## Scope cuts

Defer private registry credentials, multi-service/group updates, scheduled/silent updates, generalized Compose feature support, self-update/Traefik updates, automatic recovery and image rollback of application data. Do not reopen them simply because they are adjacent to the first update path.

## Remaining B6-03 work

The protected journal, crash tests, maintenance lease, WAL backup, quarantine, reference and inverse inventory are available in merged PRs #105–#110. Rather than extend Phase 3c indefinitely, close remaining safety integration while implementing B6-05/B6-08. The Phase 3 issue remains open until its real-state reconciliation and operational restore evidence are complete.

## Release readiness is separate from development speed

A candidate may be built and exercised before all noncritical cosmetic issues are fixed. It must **not** be declared release-ready while required SEC suites, CLI authorization, journal uncertainty, filesystem/actual-state consistency or off-host restoration are unresolved. All exceptions need a documented owner and disposition.
