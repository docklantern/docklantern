# PR #9 — Service monitoring policies and scheduler

**Scope:** Monitoring only. No pulls, Compose deployment, authenticated web UI, or automatic update actions.

## Configuration

Register a Compose project using the existing `register` command before configuring a watched service. The new `set-policy` mode takes a JSON file:

```json
{
  "project": "myapp",
  "service": "web",
  "image": "ghcr.io/org/web:latest",
  "platform": "linux/amd64",
  "interval_seconds": 90,
  "paused": false
}
```

```sh
go run ./cmd/docklantern-inventory -mode set-policy -db ./docklantern.db -policy-file policy.json
go run ./cmd/docklantern-inventory -mode policies -db ./docklantern.db
go run ./cmd/docklantern-inventory -mode check-due -db ./docklantern.db
go run ./cmd/docklantern-inventory -mode monitor -db ./docklantern.db -poll-every 10s -check-timeout 20s
```

`GET /api/v1/monitor-policies` lists configuration and next/last checks. This API remains **unauthenticated and localhost-only**. Policy updates are possible through the local administrative CLI only, not an HTTP endpoint.

Policies require a registered service. Checks are read-only OCI manifest requests and are **not** correlated to the currently running container's immutable image identity. Successful checks update the existing registry digest history; failures record a bounded error and reschedule, without erasing the last successful digest. Paused policies are excluded from the scheduler. Settings are durable in SQLite; the process can restart and resume checking due policies.

## Credentials

Optional environment variables `DOCKLANTERN_REGISTRY_CREDENTIAL_HOST`, `DOCKLANTERN_REGISTRY_USERNAME`, and `DOCKLANTERN_REGISTRY_TOKEN` must be supplied together. A token is used **only** when the watched image's canonical registry host matches the specified host. No credentials are persisted; secret storage and credentials for multiple registries remain future work.

Avoid publishing registry credentials in shell histories or command arguments. The existing registry resolver additionally restricts HTTPS token challenge destinations. The monitoring process must not be exposed to untrusted users, who could otherwise abuse its access to registries.

## Scheduling behavior and limits

- Per-service intervals from 60 to 86,400 seconds; the scheduler polls the SQLite due queue at a configurable short tick.
- One worker processes a bounded batch sequentially, each check with a timeout. Failed requests are rescheduled without tight retry loops.
- Policies changed/paused while a check is in flight are protected by a compare-and-set update on the current image/platform/due time.
- Only run **one** scheduler instance against this SQLite database; distributed locks, registry rate-limit backoff, multi-host and HA are out of scope.
- No automatic Docker pulls or updates are performed. The existing executor is unchanged.

Future work includes authenticated settings UI, safe credential references and rotation, jitter / explicit 429 backoff, deployed-image digest correlation, and a unified long-running application command.
