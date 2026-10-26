# DockLantern Roadmap

Roadmap stages are exploratory and do not commit to delivery dates. **Monitoring enrollment and automatic deployment authorization are independent.**

## Stage 0 — Architecture validation (initial exploration)

Compose discovery/labels, OCI registry auth, image/platform handling, registered paths, and Docker/socket threat modeling established the monitoring-only baseline.

## Stage 1 — Compose-first image monitoring

### v0.1.0-beta.3 — Released

[Published beta.3](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.3) provides opt-in monitoring with `com.docklantern.monitor=true`, interval labels and GUI overrides, global defaults, pauses, persistent SQLite history, read-only image status, conservative deployed-image evidence, and deduplicated OCI registry checks. An explicit `monitor=false` label blocks monitoring enrollment.

Beta.3 is published; [issue #34](https://github.com/docklantern/docklantern/issues/34) was closed after operator acceptance with some host checks explicitly deferred. Publication does not mean future betas are already deployed.

### v0.1.0-beta.4 — Image-first Management Experience (release validation)

**Development branches/PRs merged:** [#44](https://github.com/docklantern/docklantern/pull/44), [#52](https://github.com/docklantern/docklantern/pull/52), [#53](https://github.com/docklantern/docklantern/pull/53), [#54](https://github.com/docklantern/docklantern/pull/54), [#55](https://github.com/docklantern/docklantern/pull/55), [#56](https://github.com/docklantern/docklantern/pull/56).

- Five workspaces: Overview, Images, Services, Activity, Settings.
- Canonical image/platform grouping and dedicated Image Details showing digest evidence and related services.
- Compose project-grouped Services and Service Details with existing monitoring controls.
- Consistent registry and deployed status semantics, including conservative Unknown assessment.
- Image-centric Overview review queue and date-grouped Activity.
- Simplified global Settings plus Advanced / Legacy Configuration.
- Tested compatibility, security, accessibility and operator release gates.

**Not yet released or deployed.** See [beta.4 release checklist](../deployment/beta4-release-checklist.md) and [issue #51](https://github.com/docklantern/docklantern/issues/51). The current image/platform persisted observation identity is not credential-scope-aware; multiple simultaneous credential scopes for one tagged image require future design work.

Beta.4 adds **no** Docker write permission or deployment executor.

## Stage 2 — Controlled deployment (future; not authorized)

Potential work: deliberate executor design, Compose override generation, validation and image pulls, opt-in/manual deployment, journaling, readiness checks, rollback and failure recovery. This requires its own threat model and user authorization; monitoring enrollment alone is never sufficient.

## Stage 3 — Cooperative management (future)

External drift detection, non-destructive adoption, deployment groups, logs/history, larger-scale onboarding and operational tooling.

## Longer-term exploration

Semantic version/update policies, per-service credential-scope-aware observation persistence, richer notification channels, stronger rollback, Docker event-based discovery and multi-host management.
