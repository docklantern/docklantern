# Beta.5 Phase 2 — Check Now request contract

**Tracks:** [#66](https://github.com/docklantern/docklantern/issues/66), after the watched-image persistence work [#65](https://github.com/docklantern/docklantern/issues/65).
This phase adds backend request endpoints only. **Service Details and Image Details buttons and visible confirmation messages belong to phase 3 [#67](https://github.com/docklantern/docklantern/issues/67).**

## Behavior

An operator may ask DockLantern to check a known registry watch before its next normal interval. **Check Now is never a pull/update action.** It does not inspect or mutate Docker at request time, perform a network registry call from a web handler, or bypass existing OCI registry cooldowns.

The handler:
1. Requires HTTP Basic authentication under the existing `auth.Guard`, an accepted Origin and valid CSRF token; rejects unsupported content types and oversized forms using the existing `Handler.prepare` guard.
2. Resolves a **previously persisted** monitoring policy from a project/service identity or a canonical image/platform ID. A client cannot submit a new arbitrary network destination.
3. In one SQLite transaction, applies exactly the eligibility predicate already used by the normal monitor worker: no paused, explicitly opt-out, invalid-discovery or unenrolled watch. Legacy registered monitoring retains existing eligibility.
4. Does **not** move backoff schedules forward when `consecutive_failures > 0`. Normal monitor worker and cached OCI resolver still enforce rate-limit backoff, request timeouts, credential scoping and the 45-second deduplicating cache.
5. Schedules eligible, healthy, not-yet-due policies by setting `next_check_at` to current UTC time. Requests are picked up at the **next background monitor tick** (10 seconds by default, configurable), not synchronously. Generation/CAS changes guard against stale in-flight results.
6. Enforces **at least one minute** between manual requests for the same canonical image/platform, including requests targeting different services sharing the image; any recent completed attempt for the group also imposes that cooldown. If **any** service watching that image/platform is still in persistent retry backoff, the whole manual group request is deferred. Initial already-due work is left unchanged when no cooldown/backoff applies. A single image request schedules at most 100 eligible watches, to avoid unbounded SQLite work.
7. Records a durable `last_manual_request_at` alongside the existing policy schedule. Repeated POSTs, restarts and concurrent request attempts cannot continually invalidate work; accepted work remains queued in SQLite if the monitor worker is stopped and will run when it resumes. A request is not a guarantee of immediate success.
8. Performs no notification delivery directly. The existing monitor's registry observation and event pipeline continues normally after the requested check is run.

## Endpoints and response contract

**No new front-end controls are included in phase 2.** Phase 3 can submit ordinary form POSTs to:

- `POST /settings/monitoring/check-now/service`: `csrf_token`, `project`, `service`.
- `POST /settings/monitoring/check-now/image`: `csrf_token`, `image_id` produced by `presentation.ImageID` for a known image/platform grouping.

An accepted, already-due, or deferred authenticated request redirects with HTTP 303 to its corresponding Service or Image Details page with `?check=<state>`. The state codes are:

| State | Meaning |
| --- | --- |
| `scheduled` | At least one eligible future check was moved into the due queue; it has **not yet completed** |
| `already_due` | Worker already has due work; no schedule modification required |
| `deferred_backoff` | Existing failure/retry schedule is preserved; retry is not forced |
| `deferred_recent` | A recent check or manual request is too new; cooldown is preserved |

Bad image/service identifiers return HTTP 400, valid but ineligible or unknown watched identities return HTTP 409, unauthenticated requests return HTTP 401, and invalid Origin/CSRF returns HTTP 403. An unexpected SQLite failure returns a generic HTTP 500, without internal paths, Docker metadata or credentials.

An Image Details request targets all eligible watched services sharing the exact canonical image and platform, **not** all containers on the Docker host. If any service in the image/platform group is still backed off, the manual request is **deferred for the entire group** to avoid rate-limit bypass; when no backoff applies, already-due watches remain unchanged and `scheduled` means at least one not-yet-due watch was queued. No platform or credential scopes are merged; the current single-process registry-credential limitation remains [#62](https://github.com/docklantern/docklantern/issues/62).

## Persistence and rollback

Transactional schema **v5** adds `monitor_policies.last_manual_request_at TEXT NOT NULL DEFAULT ''`. Earlier policies and event history are untouched, and existing v4 databases migrate with the same fail-closed version guard. This is forward-only: a future VPS upgrade must first create a **consistent, verified SQLite backup**, and rolling back to beta.4.1/v3 or a phase-1/v4 build requires restoring the matching older database snapshot.

No deployment, release, registry image publication or database migration on the VPS is authorized by this PR. PR merge must receive separate operator approval. The production image remains beta.4.1 until an independently approved release and deployment.
