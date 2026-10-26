# Beta.3: Bounded shared OCI registry checks

**Implementation:** [Issue #33](https://github.com/docklantern/docklantern/issues/33). This is monitoring-only; it does not grant any Docker write permissions.

## Why share checks?

Different Compose services often watch the same tagged OCI image on the same platform. The beta.2 monitor made a full remote manifest request for every due service, even if another watcher had just checked that exact image. This is especially expensive for multi-platform manifest lists and Docker Hub authentication.

The beta.3 monitor uses a small **in-process cache** around its existing read-only OCI resolver:

- Identity is the **canonical OCI image tag + platform (OS/architecture/variant) + effective authentication scope**.
- Anonymous and authenticated observations can never share a cache key. The authentication scope is represented by an opaque, SHA-256-derived fingerprint; neither credentials nor fingerprints are persisted to SQLite or written to event logs.
- Successful lookups are reused for at most **45 seconds**, with at most **128 entries**. Distinct tags/platforms/registries/credential scopes always get their own observations. Cache eviction is bounded.
- Unknown/custom resolvers that cannot report an effective credential scope are deliberately **not cached**.
- All OCI calls are still **sequential** with the existing per-request timeout, due-policy batch size (20 in the app), and worker polling interval. This provides a strict upper bound on outbound concurrency, suitable for small single-vCPU hosts.
- `HTTP 429` responses respect `Retry-After` as a **registry-host + credential-scope cooldown**, also applying to different watched tags on that host. A 429 without a supplied delay uses a one-minute cooldown. `503` with `Retry-After` is also honored; without one, that image error is briefly cached.
- Cooldowns are in memory and capped at 64 scopes. SQLite independently persists each **service's exponential retry/backoff**, so an application restart does not reset policy backoff. It is safe to use an in-process (rather than persistent) response cache because SQLite keeps the authoritative digest baseline.
- An invalid result, unauthorized request, malformed manifest, or canceled request is not reused as a successful observation.

## Events and per-service state

Each eligible service still has its **own** monitoring policy and next due time, last success, failure counter, pause flag, and notification preferences. A cached success is passed through the same `RecordRegistryCheck` / `FinishMonitorCheck` flow as a new lookup. Registry digest changes continue to generate **one image/platform-scoped event** for each observed movement, even when several watchers share the same remote result. A remote change does not imply an available newer release.

Cached rate-limit responses also flow through per-service error/backoff bookkeeping, but the monitor avoids making additional HTTP requests while a scoped host cooldown is active. In-flight policy generation checks continue preventing older results from replacing newer GUI settings.

## Operational limitations

This POC currently uses one scheduler process per host; cache entries **do not cross independent DockLantern processes**. Restarting the monitor clears the in-memory cache but does not discard SQLite schedules, failure backoff, or historical registry events.

Newly registered watches may still become due together; sequential processing, batch-size limits, bounded response reuse and registry cooldowns prevent unconstrained outbound bursts. Event-driven Docker discovery, distributed leases, user-configurable cache durations, and registry-wide global quotas are future work.

## Tests

The registry cache tests verify bounded memory, TTL expiry, registry/tag/platform isolation, credential scope fingerprints, host cooldown and remaining Retry-After, plus SQLite integration for three services monitoring the same image (one paused), the `first_seen → changed → unchanged` sequence, exactly one digest-change event, and separate per-service persistent failure/backoff state.
