# Roadmap (Draft)

Roadmap stages are exploratory and carry no delivery-date commitment.

## Stage 0 — Architecture validation

Validate Compose labels, registered file lists, interpolation, relative bind mounts, Traefik networks and managed override locations. Threat-model Docker and private registry access. Decide executor, language, license and CI choices.

## Stage 1 — Monitoring

Project discovery/enrollment, OCI manifest checks and auth, configurable polling, pauses/check-now, dashboard and check history.

## v0.1.0-beta.3 — Monitoring UX and scale (next planned beta)

See the [implementation plan](beta3-plan.md) and tracking [issue #27](https://github.com/docklantern/docklantern/issues/27). Required beta.3 outcomes:

- Zero-touch, explicit opt-in Compose label monitoring with no edits to the DockLantern deployment or Compose source mounts.
- Live GUI interval overrides, label interval fallback and persistent global defaults.
- Persistent shared sidebar across all HTML pages and service-centric status views.
- Safe migration of beta.2 SQLite registrations/history; no unauthorized deployment.
- Efficient/deduplicated OCI registry checks, unambiguous event statuses, and automated end-to-end gates.

Beta.3 is **monitoring-only**. Deployment control remains future work.

## Stage 2 — Controlled deployment

Compose override generation, validation, image pulls, manual/opt-in automatic deployment, journaling, readiness checks and failure recovery.

## Stage 3 — Cooperative management

Drift detection, manual non-destructive adoption, deployment groups, logs/history, user-facing onboarding and documentation.

## Future

Standalone Docker updates, semantic tag policies, notification channels, stronger rollback, and multi-host management.
