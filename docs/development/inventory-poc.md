# PR #7 — Monitoring-only discovery and registration

This milestone introduces a **second Go command** at `cmd/docklantern-inventory` without modifying the existing Compose execution POC.

## Behavior

- Docker container discovery uses read-only `docker ps -aq` and `docker inspect` operations, including Compose labels, image identity, state and health.
- `register` requires explicit allowlisted source paths, validates them with the existing POC validator, and stores the registration in SQLite with `enabled=false`.
- SQLite persists registered Compose projects and the latest discovered container snapshot. An empty snapshot removes stale inventory entries.
- An HTTP API exposes only `GET /healthz`, `GET /api/v1/projects`, and `GET /api/v1/containers`. There are no mutation endpoints, update controls, or automatic deployments.

## Example (development Docker host)

```sh
go run ./cmd/docklantern-inventory -mode discover
go run ./cmd/docklantern-inventory -mode scan -db /tmp/docklantern-inventory.db
go run ./cmd/docklantern-inventory -mode serve -db /tmp/docklantern-inventory.db -listen 127.0.0.1:8787
curl http://127.0.0.1:8787/api/v1/containers
```

Register with `-mode register -registration /absolute/path/registration.json -roots /absolute/allowed/root`. Paths must exist and remain within allowlisted source roots; registration does **not** enable deployments.

## Security and limitations

**Do not expose this API publicly.** Authentication and authorization are not implemented. Listener deliberately rejects non-loopback hostnames. It may still expose internal Compose paths and image names to local callers. No background refresh: invoke `-mode scan` manually. Database lives on a user-chosen writable path and should be on persistent storage.

Discovery relies on host Docker CLI availability and direct daemon privileges. It is not Docker socket privilege separation. No registry polling, scheduler, update actions, drift reconciliation, API pagination, or secret storage is implemented.

SQLite driver is CGO-free `modernc.org/sqlite`, pinned in `go.mod`. Dependency footprint and licensing should be reviewed before release.
