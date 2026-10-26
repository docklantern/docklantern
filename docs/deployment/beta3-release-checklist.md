# v0.1.0-beta.3 — Release acceptance and operator handoff

**Status: release validation in progress. This document is not a release approval.**  
**Release tracker:** [#34](https://github.com/docklantern/docklantern/issues/34)  
**Scope:** monitoring-only; never automatically pull, start, stop or replace other containers.

## Evidence matrix

| Gate | Automated evidence | Additional approval needed |
| --- | --- | --- |
| Zero-touch label onboarding | `tests/integration/run-beta3-onboarding.sh` builds the real app/proxy, starts DockLantern first, then recreates a real Compose fixture with labels and waits for periodic discovery. Verifies policy without project registration, DockLantern config, roots or source mounts; verifies no unintended container recreation. | Real VPS smoke test only after backup and deployment authorization |
| Default and label/GUI priority | `internal/store/monitor_admin_test.go`, `internal/store/reconcile_test.go`, `internal/management/monitoring_settings_test.go` | Confirm GUI interactions on real TLS origin after approved deployment |
| Explicit opt-out, removed labels, replica conflicts and failure recovery | `internal/store/reconcile_test.go`, `internal/discovery/labels_test.go` | Smoke test in representative Compose stacks |
| Shared GUI navigation and service/event status | `internal/management/navigation_test.go`, `internal/dashboard/services_test.go`, `internal/management/service_events_test.go` | Browser review on desktop and mobile widths |
| OCI digest movement, events and retry scope | `internal/monitor/cache_test.go`, `internal/store/registry_test.go`, `internal/management/service_events_test.go`, `internal/store/backoff_test.go` | Confirm public and authenticated registry behavior with authorized credentials |
| Legacy beta.2 upgrade and rollback | `internal/store/migrations_test.go`, `internal/store/monitor_admin_test.go`; `run-beta-install.sh` checks backup/restore in an isolated container | Consistent backup and **verified restore** before live database migration |
| Read-only Docker security | `tests/integration/run-discovery-proxy.sh` and beta.3 onboarding test ensure POST fails; `go test -race ./...` | Verify real host's private proxy network and socket scope |
| Release image/security | `scripts/verify-release-inputs.sh`, `go vet ./...`, security-audit workflow, tag-gated multi-arch Docker Buildx release workflow | Review signed-off commit, scan results, Docker Hub/GHCR artifact digests, image provenance, supported platforms |
| Deployment Compose and onboarding docs | [docklantern-deploy](https://github.com/docklantern/docklantern-deploy) Compose validation workflow and separate beta.3 documentation PR | Do **not** change the pinned beta.2 image until beta.3 is published and approved |

**Automated gates passing is necessary but not sufficient.** The release tracker remains open until CI evidence, published artifact verification, and an authorized deployment smoke test are recorded.

## Beta.2 → beta.3 operator checklist (do not run as part of CI)

- [ ] Review the published beta.3 release notes, immutable image digest, and security scan.
- [ ] Record the currently deployed app image digest and existing optional `compose.projects.yaml` overlay; never silently drop legacy mounts from an already registered deployment.
- [ ] Produce a **consistent** SQLite backup using the existing app's `-backup` mode. Confirm `PRAGMA integrity_check` returns `ok` on a copy and save it off-host.
- [ ] Retain the previous beta.2 image tag and the pre-migration database snapshot. A binary rollback on the already migrated SQLite schema is **not** guaranteed.
- [ ] Validate the local Compose config using all the same overlays as the current deployment. Update only DockLantern's image to the approved pinned beta.3 release; do not change Traefik or other containers.
- [ ] Verify authentication, `/readyz`, Overview, Services, Settings, Event History, and original beta.2 registrations/history after the upgrade.
- [ ] On an independently deployed `hello-py` or `hello-js` service, add `com.docklantern.monitor: "true"`, with an optional `com.docklantern.interval`. Recreate **only that target service** to apply labels.
- [ ] Confirm DockLantern discovers the newly labeled service on its next scheduled scan, creates one policy and displays correct enrollment source. Ensure DockLantern itself did not restart and no new Compose source mounts or registration were needed.
- [ ] In the GUI, test global default, override, reset, pause, resume and explicit `monitor=false`; verify changes do not restart a container. Check actual registry observation and event wording.
- [ ] Confirm Docker proxy still denies POST and exposes no host port, and DockLantern has no Docker CLI/socket mount or runtime deployment credentials.
- [ ] Record test evidence and unresolved limitations on [#34](https://github.com/docklantern/docklantern/issues/34); only then mark beta.3 ready.

## Known limitations / deferred

- Registry digest movement establishes only a changed **remote tag**, not semantic version direction or a newer available release.
- Monitoring uses polling rather than instant Docker events; no multi-host coordination or distributed leases.
- No automatic image pulls, Compose execution, container restarts, policy-driven deployments, or arbitrary source editing.
- Single HTTP Basic auth account behind trusted HTTPS; no team-wide RBAC.
- The in-memory registry cache is per running process; SQLite stores persistent schedules/history. Transient registry failures are backed off independently by service.
- Label changes require recreating the **target application container**, not DockLantern.
- The example deployment uses an existing Traefik, private network and an optional IPv6 egress network supplied by the host.

## Rollback

An approved rollback requires: stop DockLantern, **restore the verified pre-upgrade SQLite snapshot as a whole**, move aside any old `-wal`/`-shm` journal files, switch to the previously recorded beta.2 image and start exactly one app process. Never downgrade the binary in-place against a version-3 schema and assume it is safe. The [deployment operations guide](https://github.com/docklantern/docklantern-deploy/blob/main/docs/operations.md) has the backup/restore procedure.

## Release image contract

`v0.1.0-beta.3` is the intended tag; it **does not yet exist** merely because this plan describes it. The `.github/workflows/release.yaml` workflow publishes multi-architecture `linux/amd64` and `linux/arm64` images only after a reviewed main commit is tagged, tests pass and the security audit succeeds. Do not create the tag or change production deployments without explicit authorization.
