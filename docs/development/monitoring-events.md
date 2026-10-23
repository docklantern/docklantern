# PR #17 — Monitoring events and signed webhook notifications

This release adds **monitoring-only** event history and opt-in outbound HTTPS webhooks. It does **not** run Docker pulls, restarts, Compose updates or mutation APIs.

## What creates events

- `remote_digest_changed` — a previously checked image tag resolves to a different OCI platform manifest digest. This is **not** proof of a newer release, nor confirmation of a running-container update.
- `registry_failure` — the first unsuccessful registry check in a service's current failure incident. Repeated failed attempts, including 429 retries, do not generate duplicate failure events.
- `registry_recovered` — a previously failing service's successful check after one or more failed attempts.

The image digests, image, platform, project/service and UTC timestamps are stored without secrets. Digest transitions are inserted in the same SQLite transaction as the check result. Failure/recovery events are inserted in the same SQLite transaction as the changed retry state. All events are retained across restarts. The first successful digest observation is a baseline, not a change alert.

Read event history in the authenticated dashboard or via `GET /api/v1/events` (latest 100) or `docklantern-inventory -mode events`.

## Configure the optional notifier

For a local instance or deployment, set:

- `DOCKLANTERN_WEBHOOK_URL` — a trusted **public HTTPS** endpoint. URL userinfo, fragments and query strings are rejected.
- `DOCKLANTERN_WEBHOOK_HMAC_KEY_FILE` — path to a secret file containing a random key of at least 32 characters.

Run the standalone worker via `docklantern-inventory -mode notify -db /data/docklantern.db`, or add `-run-notifier` to the HTTP server command. The existing `-run-monitor` flag still controls the registry scheduler separately.

For Traefik Compose deployments, from `deploy/traefik`:

```sh
openssl rand -hex 32 > secrets/webhook_hmac_key.txt
chmod 600 secrets/webhook_hmac_key.txt
# Add DOCKLANTERN_WEBHOOK_URL=https://hooks.example.org/docklantern to .env
docker compose -f compose.yaml -f compose.notifications.yaml config
docker compose -f compose.yaml -f compose.notifications.yaml up -d --build
```

If using Docker discovery too, be aware that Compose overlays override `command`. The example notifier overlay runs registry monitoring but not periodic discovery; specify the required flags together when combining overlays.

**No webhook is sent unless the notifier is explicitly started with a URL and signing secret.** Pending events remain in SQLite and can be delivered later. If a webhook is first configured long after monitoring began, consider the existing event backlog before enabling it.

## Delivery contract

A JSON POST uses `type: docklantern.monitoring.v1` with an `event` object and includes:

- `X-DockLantern-Event-ID: <integer>`
- `X-DockLantern-Signature: sha256=<HMAC-SHA256 hex of the exact UTF-8 JSON request body>`

Verify the signature using your shared key with a constant-time comparison and deduplicate on event ID. Delivery is **at least once**: if a webhook succeeds but the database update fails or the process crashes before it commits, the notification can be delivered again.

HTTP 2xx acknowledges a delivery. Network errors and non-2xx responses retain the pending event and schedule bounded exponential retry, from one minute up to one hour. Failed webhook attempts do not alter Docker inventory, registry digest history or polling schedules. The dashboard shows delivery state and retry counts.

## Security

Webhook requests require HTTPS, follow **no redirects**, respect TLS validation, never use HTTP proxy environment variables and reject connections to loopback, private, link-local or non-global IP destinations after DNS resolution. This prevents using the notifier as an obvious internal Docker/metadata endpoint client. Protect the HMAC key; it is only read from a file and never stored in SQLite. Registry error response bodies and passwords are not recorded in event history.

Only run **one notifier instance** against the SQLite database. A distributed delivery lease, host-wide rate coordination, indefinite delivery retention controls and a production alert-management UI are future work. Do not expose the Docker socket proxy publicly.
