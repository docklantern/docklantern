# DockLantern Roadmap

Roadmap stages are exploratory and do not commit to delivery dates. **Monitoring enrollment and automatic deployment authorization are independent.**

## Stage 0 — Architecture validation (initial exploration)

Compose discovery/labels, OCI registry auth, image/platform handling, registered paths, and Docker/socket threat modeling established the monitoring-only baseline.

## Stage 1 — Compose-first image monitoring

### v0.1.0-beta.3 — Released

[Published beta.3](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.3) provides opt-in monitoring with `com.docklantern.monitor=true`, interval labels and GUI overrides, global defaults, pauses, persistent SQLite history, read-only image status, conservative deployed-image evidence, and deduplicated OCI registry checks. An explicit `monitor=false` label blocks monitoring enrollment.

Beta.3 is published; [issue #34](https://github.com/docklantern/docklantern/issues/34) was closed after operator acceptance with some host checks explicitly deferred. Publication does not mean future betas are already deployed.

### v0.1.0-beta.4.1 — Image-first Management Experience (released)

**Development branches/PRs merged:** [#44](https://github.com/docklantern/docklantern/pull/44), [#52](https://github.com/docklantern/docklantern/pull/52), [#53](https://github.com/docklantern/docklantern/pull/53), [#54](https://github.com/docklantern/docklantern/pull/54), [#55](https://github.com/docklantern/docklantern/pull/55), [#56](https://github.com/docklantern/docklantern/pull/56), [#57](https://github.com/docklantern/docklantern/pull/57), [#58](https://github.com/docklantern/docklantern/pull/58), [#59](https://github.com/docklantern/docklantern/pull/59), [#60](https://github.com/docklantern/docklantern/pull/60), [#61](https://github.com/docklantern/docklantern/pull/61).

- Five workspaces: Overview, Images, Services, Activity, Settings.
- Canonical image/platform grouping and dedicated Image Details showing digest evidence and related services.
- Compose project-grouped Services and Service Details with existing monitoring controls.
- Consistent registry and deployed status semantics, including conservative Unknown assessment.
- Image-centric Overview review queue and date-grouped Activity.
- Simplified global Settings plus Advanced / Legacy Configuration.
- Automated compatibility and security gates passed; operator desktop/tablet/mobile/keyboard smoke checks accepted, with deferred tests recorded as exceptions.

**Release status:** [v0.1.0-beta.4.1](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.4.1) was published on October 28, 2026 from final source commit `358f0d7e90082164b23c9df1c0b66dec48651f85`. On October 31 the operator reported the released image running on the test VPS with embedded version `v0.1.0-beta.4.1` and `/readyz` PASS. The deployment repository [PR #5](https://github.com/docklantern/docklantern-deploy/pull/5) was subsequently merged with the published image digest. The older `v0.1.0-beta.4` tag/images remain preserved (earlier commit, no GitHub prerelease), and PR #58 was a separate local test build. [Release gate #51](https://github.com/docklantern/docklantern/issues/51) is **closed with accepted exceptions**: on-VPS-only SQLite backup, no full restore drill, and deferred exhaustive edge-case/API/latency checks. Multi-credential persisted observations remain unsupported; future work is tracked in [#62](https://github.com/docklantern/docklantern/issues/62).

Beta.4 adds **no** Docker write permission or deployment executor.

### v0.1.0-beta.5 — Actionable image monitoring (released and test-VPS accepted)

[v0.1.0-beta.5](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.5) was published October 31, 2026 and is reported healthy on the IPv6-only test VPS. [Umbrella #63](https://github.com/docklantern/docklantern/issues/63), [regression/acceptance #68](https://github.com/docklantern/docklantern/issues/68), and [release gate #69](https://github.com/docklantern/docklantern/issues/69) closed after operator acceptance. The release adds watched-tag override/reset, durable background Check Now and clearer monitoring diagnostics. Accepted **test-only** limitations include on-VPS-only verified schema-v5 backups, no restore drill, untested older v3 snapshot recovery, and deferred extended security/performance checks. This is not production certification. Credential-scope persistence [#62](https://github.com/docklantern/docklantern/issues/62) remains separately deferred.

Beta.5 preserves monitoring-only behavior, with no automatic/manual Compose updates or new Docker write privileges.

## Stage 2 — v0.1.0-beta.6 Controlled Compose deployment (architecture accepted; not implemented)

A [gated beta.6 implementation and acceptance plan](beta6-plan.md) proposes phases 0–6, prerequisite security checks, a separate executor release artifact and independent operator approvals. It is a **planning draft**, not authorization to create issues, implement, merge, tag or deploy.

[ADR 0008](../architecture/decisions/0008-on-demand-compose-executor.md) establishes an **on-demand host-native Go CLI executor**, independent host-local operator approval, explicit executor-owned project/service registration, immutable candidates, controlled image-only Compose overrides, a durable journal and manually approved recovery. Monitoring remains GET-only and cannot invoke the executor. This is a design decision, not authorization to implement, merge, publish or modify the VPS.

Initial beta.6 scope: one explicitly registered project/service, public prebuilt OCI image with verified digest/platform, GUI proposal + terminal confirmation, scoped Compose replacement, verification and sanitized Activity receipt. Defer automatic updates, private registry deployment, broad Compose compatibility, multi-service management and automatic rollback. Require executor-specific threat modeling, off-host backups with restore drill, disposable integration tests and separate operator release/VPS gates.

## Stage 3 — Cooperative management (future)

External drift detection, non-destructive adoption, deployment groups, logs/history, larger-scale onboarding and operational tooling.

## Longer-term exploration

Semantic version/update policies, per-service credential-scope-aware observation persistence, richer notification channels, stronger rollback, Docker event-based discovery and multi-host management.
