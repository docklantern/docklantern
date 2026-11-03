# Beta.6 Phase 3a — Protected executor SQLite journal foundation

**Status:** CI-only, no Docker/Compose mutations, proposal for review under [B6-03 (#83)](https://github.com/docklantern/docklantern/issues/83). **No real host journal, release, deployment or VPS directory has been created.** This slice is a prerequisite of Phase 3, **not** its final acceptance.

## Independent authority and protected filesystem

The new `internal/executorjournal` package is independent of the existing monitoring SQLite database and the monitor's GET-only Docker proxy. The only production journal opener is `OpenHost()`; it requires a root effective UID and uses the fixed directory:

```text
/var/lib/docklantern/executor/journal/operations.sqlite
```

A host administrator must provision the parent directory and root-owned ancestors beforehand; neither CLI nor monitoring web app calls `OpenHost` in this PR. The reader/writer walks each ancestor via `openat(O_NOFOLLOW|O_DIRECTORY)` and checks root ownership, no group/world writable ancestor, and a private (`0700`) final journal directory. It creates the database using a private exclusive `0600` file open only if absent; existing database and WAL/SHM sidecars must be singly linked, root-owned, regular and mode `0600`. The opened file descriptor is checked against the SQLite pathname inode on completion. No web/monitor writable mount is involved. Root-only disposable CI fixtures live beneath a private `/root` test tree, not at production paths.

SQLite uses `WAL`, `synchronous=FULL`, foreign keys and an explicit busy timeout; one connection per store avoids connection-scoped PRAGMA drift. SQLite durability **does not** make Docker operations, managed overrides and journal commits atomic. Those are still future gated components.

## Migration v1 and durable tables

The transactional `PRAGMA user_version=1` migration creates these independent tables:

- `operations`: opaque operation/project/registration/service/proposal identities, exact proposal fingerprint, immutable candidate digest, timestamps, current phase and outcome. A partial unique project index blocks a new operation while an existing operation is `pending` or `needs_reconciliation`.
- `phase_events`: append-only ordered per-operation sequence, strictly bounded phase/kind/code enums, and timestamp. SQLite **triggers forbid UPDATE/DELETE**, including direct SQL attempts.
- `baselines`: structured actual/desired fingerprints and readiness grades; immutable-by-trigger, with no production writer until independent actual-state checks exist.
- `override_records`: protected append-only generation/promotion-history schema; **no override files or promotions** implemented.
- `receipt_publications`: structured version/hash/publication-state history; **no receipt outbox or publisher** implemented.

A newly initialized database is migrated transactionally. Unknown future versions, unrelated SQLite schemas, absent expected tables and failed integrity checks fail closed; the code does not downgrade, reset, delete, or silently adopt incompatible state. The schema models eventual terminal states, but **Phase 3a exposes no method that can mark an operation `succeeded`, `recovered` or safe for execution**.

## Ordered and conservative phase intents

`Begin` records validated, bounded, non-path operation identity and metadata; it reserves one unresolved operation per project. `Intent` appends a durable journal record before each step and `Complete` appends a separate fact afterward. Phases must be traversed in order:

```text
PREPARE → AUTHORIZE → PREFLIGHT → PULL → STAGE → APPLY → VERIFY → COMMIT
```

Duplicate, skipped, reordered or replayed intents/completions are rejected. **A completion also requires the same live Store session that recorded its matching intent**; reopening after a crash discards this in-memory completion eligibility, even though the earlier intent remains durable. This does not replace future cross-process project locks. Arbitrary raw errors, credentials, Compose paths, child stdout or JSON are never stored in event codes; uncertainty uses a strict small allowlist (`interrupted`, `state_ambiguous`, `external_change`, `storage_uncertain`).

When a process stops after intent but before completion, `Inspect` conservatively exposes **`needs_reconciliation`** even when the operation row is still `pending`. `MarkUncertain` appends an explicit durable marker and locks the operation in `needs_reconciliation`; it cannot be reset to pending or silently replayed by any Phase 3a API. Other operations in the same project remain blocked. Operations in unrelated projects may create separate journal records, but **cross-process OS project locks** are deferred to Phase 3b.

**Critical boundary:** No Docker actual-state read, Compose invocation, OCI resolution, image pull, override stage/promotion, recovery, production operator approval or CLI action occurs in this package. Appending `completed` is bookkeeping, **not evidence of a verified Docker deployment**. Even a completed `COMMIT` stays `pending` and blocks new work until future independently verified finalization is designed and accepted.

## CI evidence and deferred gates

Go fake/SQLite tests inject interruption after each of the eight phase intents, close and reopen the database, confirm the uncertain status remains visible and same-project replay is rejected. Other tests verify SQLite user-version migration, WAL/FULL pragma, immutable phase history, foreign keys, strict phase progression, immutable operation identity, invalid metadata, sanitized errors, same-project denial vs independent projects, and future-version/foreign-DB failure. A separate root-owned disposable CI fixture checks private file modes/ownership, hardlink/symlink and writable ancestor rejection, restart survival and future version refusal.

This is a **3a schema and operation-event foundation** only. Phase 3b/3c still require project-level cross-process file locks, inspect-only fake-backed `status`/`reconcile`, verified baseline recording, stable override generations, sanitized idempotent outbox receipts, crash+WAL backup/restore and ambiguity tests. Phase 4 must validate actual Docker/Compose source/OCI evidence, including previously deferred Phase 2 operator confirmation; Phase 5 Docker mutations remain unapproved. Existing `register` and informational `preview` remain unchanged; `status`, `reconcile`, `apply` and `recover` remain unavailable.

**Operator governance:** No PR merge, release, executor installation, root directory provisioning, test-VPS action, Docker permission expansion or Docker mutation without separate explicit approval. The live hello-go Compose parent mode `0775` remains ineligible; nothing here changes host permissions or that condition.

## Follow-up v2 generation staging foundation

The separate [Phase 3c-a override generation PR](beta6-override-generations.md) migrates the executor journal from v1 to v2 **without deleting v1 operations**, and adds immutable `override_generation_events` for stage and promotion intent. Protected 0600 metadata manifests do **not** contain Compose configuration, and **no promotion completion or Docker write** is enabled. Prior v1 statements above describe the original Phase 3a checkpoint, not a bypass of the v2 migration or its reconciliation blocks.

## Phase 3c-b — Receipt outbox follow-up

The [sanitized receipt publication foundation](beta6-receipt-outbox.md) uses the existing append-only `receipt_publications` table for deterministic, bounded v1 pending/uncertain status. It **does not** enable verified success, monitor UI integration, receipt pruning, or Docker writes. The pending and published events are two independent, idempotent journal/file durability boundaries.
