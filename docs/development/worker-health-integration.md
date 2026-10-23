# PR #19 — Disposable end-to-end monitoring flow and worker health

This change verifies the actual monitoring data path and adds **read-only operational visibility**. No Docker image pulls, Compose deployments, restarts or other mutating Docker actions are enabled.

## End-to-end CI

The new `monitoring-flow-integration` GitHub Actions job runs **only** on disposable CI runners with `DOCKLANTERN_INTEGRATION_TEST=1`. It starts a BusyBox service using Docker Compose, then exercises:

1. Real Docker CLI discovery of its Compose-labeled container and persistence to a temporary SQLite inventory.
2. Explicit monitoring registration and a watched image/platform policy.
3. Two deterministic, fake registry manifest observations, producing a digest transition.
4. Transactional `remote_digest_changed` event recording.
5. Signed HTTPS webhook delivery to a local ephemeral test server and no duplicate delivery after acknowledgement.
6. Read-back of event delivery and persisted worker health.

The fake OCI registry resolver and local webhook fixture guarantee **no real registry changes or internet webhook requests**. The test uses the Docker daemon only on the disposable CI runner. Compose cleanup runs on success and on failure.

Run locally **only on a disposable development Docker host**:

```sh
DOCKLANTERN_INTEGRATION_TEST=1 bash tests/integration/run-monitoring-flow.sh
```

## Worker status

The following workers record their last pass in the existing SQLite database:

- `registry_monitor` — periodic read-only registry scheduler
- `docker_discovery` — periodic inventory scan
- `webhook_notifier` — outbound event worker

Workers record start time, last heartbeat, last success, last error time, last batch count and an intentionally generic error status. Error details, tokens, webhook URLs and credentials are **never** stored. The authenticated dashboard displays these statuses in **Background worker health**, and the authenticated JSON endpoint is `GET /api/v1/workers`.

These are **last-observed records**, not real-time liveness probes. A stopped or crashed process can leave its last healthy heartbeat visible; operators must compare its timestamp with the expected polling interval. The existing public `/healthz` endpoint checks only the HTTP server and intentionally does not expose worker data.

## Failure handling and limits

- Registry worker writes heartbeat outcomes for each pass; normal remote-registry errors are persisted with per-service backoff as before. An SQLite heartbeat-persistence failure terminates the worker, rather than silently continuing.
- Webhook worker records pass outcomes; a worker-level database error causes the worker to stop with a logged error. Delivery failures remain in the outbox for retry.
- Docker discovery records failed and successful scans, preserving the previous inventory when discovery fails.
- A single scheduler and notifier per SQLite database is still required; distributed leases and process restart supervision are not implemented. The example Compose deployment uses `restart: unless-stopped` for process exits, but a failed goroutine may not exit the whole HTTP server. A follow-up supervisor/liveness milestone should address this before beta.
- No production VPS infrastructure is touched by the workflow.

## Release-readiness follow-ups

Consider startup configuration validation, an authenticated readiness endpoint with heartbeat freshness thresholds, a process supervisor for goroutine exits, crash-safe database backups, documented rollback testing, and stricter worker leases before enabling automated update execution.
