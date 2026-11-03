# Beta.6 Phase 3b — Project locking and inspect-only fake reconciliation

**Status:** PR for operator review under [B6-03 (#83)](https://github.com/docklantern/docklantern/issues/83). **No production journal/locks directory, executor installation, Docker writes, GUI or VPS actions are created by this change.** Phase 3b remains CI-only until separately accepted.

## Root-owned cross-process project locks

`executorjournal.AcquireProjectLock(projectID)` uses the fixed directory `/var/lib/docklantern/executor/locks`, separate from the monitor-writable proposal inbox and isolated from the monitoring GUI. The root host administrator must provision all ancestors and the private `0700` final lock directory separately. No CLI option, web request, proposal, environment override or caller-selected host pathname is accepted by the production function.

The Linux implementation requires real **root EUID**, validates the opaque project identifier, walks root-owned non-group/world-writable ancestors through descriptor-relative `openat(O_DIRECTORY|O_NOFOLLOW)`, and opens/creates the per-project `<id>.lock` using `0600`, `O_CLOEXEC`, `O_NOFOLLOW` and an exclusive no-clobber create. It verifies opened inode, owner, singly linked regular-file type and exact mode. The file name is rechecked under the already-open trusted directory after acquiring `flock(LOCK_EX|LOCK_NB)`; lock contention fails closed with a distinct sanitized error. Directory/file fsync is required on first creation.

**Never unlink a lock file on release.** Removal/recreation could create two independently flocked inodes for the same project. A lock is held until the descriptor is released or the holding process exits, including abrupt termination. Different projects use distinct independent files. This is an **advisory** cross-process lock over cooperating DockLantern executor instances; it does not stop external Docker CLI/Compose/CI jobs or a compromised host-root administrator. The caller of future execution logic must acquire and hold this lock for the **entire** protected operation. **Phase 3b introduces only the lock primitive, not any executor Docker operation or automatic journal lock acquisition.**

A root-owned disposable CI fixture exercises two independent processes contending for the same project, unrelated project concurrency, release after SIGKILL, nonblocking lock behavior, no-unlink persistence, unsafe symlink/hardlink/FIFO/mode/owner/ancestor rejection, invalid project IDs and goroutine races. No real `/var/lib/docklantern` directory is touched.

## Fake-backed inspect-only reconciliation

`Store.ReconcileReadOnly(ctx, operationID, source, now)` accepts a typed **`ActualReader` interface** and an explicit clock. **There is no Docker/Compose or registry implementation of the interface in the application**; only simulated test evidence is injected in CI. The function cannot invoke a subprocess, contact the Docker socket, modify journal rows, mark completion/success, create/remove a lock, stage/promote an override, write a receipt or call recovery.

The journal reader opens one SQLite read-only transaction that snapshots the operation, immutable phase events, original `baselines` row and latest `override_records` row, while detecting incomplete promotion intents in **any** generation. Phase 3a's `Inspect` and the reconciliation reader share the same snapshot helper. A missing baseline, missing committed override, incomplete/contradictory promotion, unmatched phase intent or explicit `needs_reconciliation` is a block, not a successful recovery.

The injected backend may return bounded identifiers for project/service, complete container/image identity, platform, source and desired-state fingerprints, explicit health grade, committed override generation/hash and inspection time. The tool rejects unknown/partial IDs, mismatched metadata, unknown health grade and evidence older than **10 seconds** (or in the future). An old DB restored against a newer override or actual container triggers a recorded-divergence reason. Backend failures are sanitized into fixed reason codes rather than leaking Docker errors or secrets.

After reading the simulated backend, the function reads the **entire journal view a second time** and rejects a changed event, baseline or override. Its only possible statuses are:

- `needs_reconciliation` with a fixed sanitized reason code for any interruption, missing/stale/ambiguous/divergent evidence or journal change; `BlocksNewWork=true` is a **report**, not a mutation of the SQLite status.
- `observations_match_unqualified` when all simulated evidence matches, still **`BlocksNewWork=true`**. This is **not** `succeeded`, `verified_healthy`, permission to apply, or confirmation that the real Engine/Compose is compatible.

No method clears an unresolved journal operation or permits a mutation. Read-only inspection cannot repair the divergence or silently replay a previous operation. It cannot eliminate a **later** external Docker/Compose change after its final read, so the result cannot serve as deployment preflight. Phase 4 must independently qualify real runtime evidence and immutable OCI identity and Phase 2's final operator confirmation.

Tests use direct SQL to seed **only disposable** journal baseline/override rows, because Phase 3b intentionally has no production write APIs for them. Tests exercise matching-but-blocked reports, changed containers/images/platform/health/sources, stale timestamps, wrong project/service, restored old DB/override generations, incomplete promotions even when a later generation exists, backend errors, interrupted intents, and a journal change during backend inspection. Tests assert the original operation remains pending and cannot begin new work for the project.

## Deferred gates and governance

Phase 3c must still implement independently protected override-generation writers, sanitized idempotent receipt publication, durable backup/restore and post-restore ambiguity tests. B6-04 must validate actual Docker/Compose/OCI state, source stability and the deferred operator confirmation; B6-05 is the separately authorized Docker-write boundary. The running `hello-go` source ancestor remains ineligible due to its observed `0775` mode.

Existing `register` and informational `preview` behavior is unchanged. `status`, `reconcile`, `apply` and `recover` CLI commands **remain disabled**; no real backend is wired. The accepted beta.5 monitoring-only deployment remains untouched.

**Do not merge this PR, publish a release, create host directories/locks, change VPS permissions, install the executor or perform Docker mutations without separate explicit operator approval.** Issue #83 stays open for the remaining Phase 3 acceptance gates.
