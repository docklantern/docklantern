# PR #16 — Freshness, registry retry behavior and conservative classification

This milestone is **monitoring-only**. It does not pull images, restart containers or authorize deployments.

## Evidence freshness

The authenticated dashboard and `GET /api/v1/image-assessments` now use `AssessFresh`:

- Inventory observations must be no more than **5 minutes** old.
- Last successful OCI registry checks must be no more than **2 hours** old.
- Missing/unparseable timestamps and observations beyond five seconds into the future are rejected.
- A stale registry observation produces `stale_registry`; a stale container snapshot produces `stale_inventory`. They are **never classified as** `current` or `different`.
- Multiple replicas require every container to have fresh inventory provenance.
- The inventory's existing `observed_at` SQLite field is now exposed in the in-memory/JSON container model, with no destructive data migration.
- Successful remote checks continue to carry their existing `checked_at` timestamps. Failed checks do not replace the most recent successful digest observation.

These bounds are conservative defaults, not promises that an artifact is safe to deploy. The default discovery interval is 60 seconds; operators need to ensure periodic scans and registry checks are enabled for the dashboard to show recent assessments.

## Classification

`current` means **matching observed digest identity**. `different` means distinct observed digests. Neither status means a verified newer semantic version, because OCI registries do not guarantee that mutable tag movement is forward in release order.

Each assessment returns `release_classification: "not_verified"`. Do not display a *Newer release available* badge until there is explicit, trustworthy version-order evidence. The dashboard shows inventory and registry observation times alongside its evidence.

## Rate limiting and retry safety

Registry HTTP 429 and 503 errors produce typed retry hints. The client parses standard `Retry-After` formats (seconds or HTTP dates), caps them at 24 hours, and **never logs response bodies or credentials**.

Monitoring policies now persist a `consecutive_failures` count. Failed requests are retried with exponential backoff (2x, 4x, etc., capped at six hours), unless the registry requests a longer `Retry-After` (up to 24 hours). A successful check resets the failure count and returns to the ordinary policy interval. A compare-and-set update prevents in-flight attempts from overwriting changed/paused policies.

Existing SQLite databases receive a non-destructive additive failure-counter migration.

## Limitations and future work

- There is still one supported scheduler instance per SQLite database. Per-registry shared rate-limit coordination and jitter are future enhancements.
- This does not establish release ordering, verify signatures, or correlate all cross-platform image forms.
- The app does not proactively declare an image update available; it shows conservative digest identity assessments.
- The restricted Docker socket-proxy remains sensitive and is not a sufficient boundary against malicious network members.
- No production VPS was touched; CI uses disposable Docker environments.

## Validation

Tests cover stale/missing/future evidence, freshness boundaries, registry Retry-After parsing, token-safe errors, persistent exponential backoff and success reset, SQLite restart/migration, and existing Compose/packaging/discovery regressions.
