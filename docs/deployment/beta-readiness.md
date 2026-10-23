# PR #20 — Beta readiness, supervision, backup and release pipeline

DockLantern remains **monitoring-only**. Do not enable automatic Docker pulls, image updates, Compose recreation, or container restarts through the web interface.

## Liveness and readiness

- Public `GET /healthz` remains a minimal HTTP process liveness endpoint (no private monitoring data).
- Public `GET /readyz` returns only a generic 200 ready or 503 not-ready. It never lists credentials, project names or worker errors. Readiness requires a successful heartbeat for each **configured in-process** worker within its maximum interval. If registry monitoring, discovery and notifications are disabled, readiness is equivalent to the HTTP process being up.
- The dashboard's authenticated `GET /api/v1/workers` retains detailed last-observed worker timestamps for operators.
- The Traefik Compose healthcheck now uses `/readyz` with a 60-second startup grace period. A Docker healthcheck marking a container unhealthy does **not** automatically restart the service under the normal Compose restart policy. Configure an external monitor/alerting system for prolonged unready containers.

Long-running workers run under a fail-fast supervisor. If a worker crashes or exits unexpectedly, the app initiates HTTP shutdown and exits nonzero, allowing `restart: unless-stopped` to recreate the container. Expected cancellation is not a failure. Ordinary registry failures continue being backed off and recorded without taking down the whole process; transient Docker scan failures mark readiness not-ready and subsequent scans can recover.

This is a single-process supervisor, not a distributed scheduler; run only one DockLantern monitoring instance for each SQLite database.

## Consistent SQLite backup

Create a safe backup **while the app is running**, using the same database volume and `VACUUM INTO`:

```sh
docklantern-inventory -db /data/docklantern.db -backup /data/backups/docklantern-2026-10-23.sqlite
```

The destination must not exist. The backup is published atomically with 0600 permissions on the same filesystem, includes all committed WAL transactions and preserves registrations, monitoring observations, policies, events and notification delivery history. Copy the backup securely to separate storage with appropriate encryption/access controls; a backup living in the same named volume is not disaster recovery.

**Restore** (stop all application and worker processes first):

1. Take an independent copy of the current database and accompanying WAL/SHM files for rollback.
2. Verify the backup with `sqlite3 backup.sqlite 'PRAGMA integrity_check;'` and expect `ok`.
3. Stop DockLantern and all processes accessing its database.
4. Move the original database **and its associated `-wal` and `-shm` files** aside together. Never attach stale WAL files to a restored database.
5. Copy the verified standalone backup into the configured SQLite database path, using private owner/mode.
6. Start a **single** DockLantern instance, check `/readyz`, recheck project registrations and policy schedules, and check the authenticated event/outbox views.

Never restore over a live SQLite connection. Test this procedure with disposable data before production use.

## Versioned release workflow

A version tag `vMAJOR.MINOR.PATCH` (optional strict alphanumeric pre-release suffix) triggers `.github/workflows/release.yml`. The workflow validates semver, checks the module graph, runs `go test -race ./...` and `go vet ./...`, then builds a multi-platform `linux/amd64` and `linux/arm64` OCI image with provenance and publishes the **version-only** tag to `ghcr.io/docklantern/docklantern:vX.Y.Z`.

The Go binary embeds its version; use `docklantern-inventory -version`. The release workflow never mutates Docker on a production host and never pushes an unreviewed `latest` tag. Build from immutable release commits, record the resulting image digest and deploy by digest for stronger reproducibility. Base images should be pinned and audited by digest as part of the formal beta release security review.

To publish, merge a reviewed PR, validate CI, and tag the reviewed commit. **Do not create a release tag before approval**. This PR only installs the workflow; it does not publish an image or deploy DockLantern.

## Remaining beta risks

- The SQLite store is single-instance; there are no distributed leases.
- Healthcheck freshness is not a guarantee of downstream registry availability.
- Single-admin HTTP Basic authentication and Docker socket proxy trust still require care in public deployments.
- Release builds use versioned image tags with provenance, but base images have not yet been pinned to immutable digests.
- External alerting for degraded readiness and a separate process supervisor remain operator decisions.
