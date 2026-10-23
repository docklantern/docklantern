# PR #18 — Notification preferences, event search and retention

This milestone improves **monitoring-only** notification administration. No image pulls, Compose restarts, Docker mutation endpoints or automated deployment controls are introduced.

## Notification preferences

Authenticated administrators can visit **Settings → Notification preferences** to select which event types are delivered by the already opt-in webhook worker:

- Remote digest changes
- Initial registry failures
- Registry recovery
- Prolonged registry outages, generated once after the configured number of consecutive failed checks (default: 3; range: 2–100)

Webhook URL and HMAC signing key remain **outside SQLite**, configured using the existing `DOCKLANTERN_WEBHOOK_URL` and `DOCKLANTERN_WEBHOOK_HMAC_KEY_FILE` variables. Merely saving settings never starts the notifier. No secrets are revealed in the settings page or stored in event history.

Disabled event types continue to be recorded, but the webhook worker excludes them from its due queue. If re-enabled later, historical pending events may be sent. This deliberate backlog behavior must be considered before re-enabling a notification category.

A prolonged outage event is emitted once during each continuous failure incident, when that incident reaches the configured failure threshold. Subsequent failures do not generate duplicate outage events. An eventual successful check creates the existing recovery event. Threshold changes during an ongoing failure incident do not retroactively create alerts that have already been missed.

## Search and pagination

- `GET /events` is an authenticated, server-rendered event explorer with kind, delivery status and project/service/image text filters, and keyset pagination by immutable event ID.
- `GET /api/v1/events/search?kind=...&delivery=...&search=...&before=...&limit=...` uses the same input validation and is protected by the existing administrator authentication.
- Search text uses an escaped SQL `LIKE` expression; wildcard input isn't treated as a pattern.
- Existing `GET /api/v1/events` remains backwards-compatible.

## Safe retention

Retention defaults to **30 days** and can be changed from 1–3650 days. Deletion is never automatic or triggered by a GET request: the Settings page requires a separately confirmed CSRF-protected POST, or an operator can run:

```sh
docklantern-inventory -db /data/docklantern.db -prune-events
```

Only **delivered** events older than the configured retention are removed. Pending events are retained even if old, preserving undelivered notifications. Back up the SQLite data volume before applying retention changes, and consider pausing or draining notifications before intentional deletion.

## Security and limits

- Settings and history are behind the existing Basic-auth guard and HTML autoescaping; mutation forms use the existing HMAC CSRF check and 32 KiB size limit.
- The HTTPS webhook sender and SSRF guard introduced in PR #17 are unchanged.
- One notifier worker per SQLite database is still required.
- No separate destination-specific preferences or secret rotation UI. No email transport is included.
- Digest changes still do **not** imply newer releases; no deployment action is enabled.

## Tests

SQLite tests cover persistent preferences, muted delivery categories, escaped search and invalid filters, retention that preserves pending events, exactly-once prolonged outage event creation per failure incident, and recovery. HTTP tests cover authenticated event exploration, CSRF-protected settings and explicit retention confirmation.
