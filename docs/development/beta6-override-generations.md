# Beta.6 Phase 3c-a — Protected override generation metadata and promotion intent

**Status:** CI-only foundation proposed under [B6-03 (#83)](https://github.com/docklantern/docklantern/issues/83), after Phase 3b [PR #102](https://github.com/docklantern/docklantern/pull/102). **This does not create or promote a real Compose override.** No executor CLI, monitoring application, production filesystem, Docker Engine or VPS is changed by the PR.

## Separate durability domains

The existing SQLite `override_records` table is an append-only **committed override history**. It remains untouched: a staged generation or promotion attempt may **never** be inserted as if it were committed. SQLite's `PRAGMA user_version` migrates transactionally from **v1 to v2** and adds a separate immutable `override_generation_events` table with strongly bounded event kinds, project/operation IDs, generation and previous committed fingerprint, candidate digest, manifest fingerprint and event time. Its UPDATE/DELETE triggers reject direct audit-history rewrites. The migration checks the prior v1 tables/triggers and journal application ID before altering an existing database; malformed or unknown versions fail closed, without resetting operations.

Each operation-specific generation has a strict append-only progression:

```text
stage_intent (durable SQLite)
     ↓
private immutable generation metadata file (atomic no-overwrite)
     ↓
staged (durable SQLite)
     ↓
promotion_intent (durable SQLite)
     ↓
[NO FILE PROMOTION OR COMMIT-COMPLETE IMPLEMENTATION]
```

The manifest contains **only** schema version, opaque project and operation IDs, monotonically selected generation, previous committed generation/hash, and approved immutable candidate image digest. It has no Compose YAML, shell commands, env files, source paths, raw error text or credentials. Its SHA-256 identifies exact bytes, but it is **not an operator approval, OCI verification, or Docker deployment permission**.

**An existing, independently protected committed base generation must already be present** in `override_records`; this PR does not invent generation 0, infer an empty desired state, or provision an initial committed override. The stage API compares base generation/hash, operation ownership and candidate digest against protected journal rows. A pending stage cannot be superseded or reused, including after a restart.

## Private host-only metadata filesystem

The staging library requires a host **root EUID**, a live held root-private per-project `ProjectLock`, an existing journal `STAGE` phase intent **from that same live Store session**, a prior committed generation and a pre-provisioned executor-only directory:

```text
/var/lib/docklantern/executor/overrides/<registered-project-id>/
  g000000000000000001.json
```

All ancestor directories must be root-owned and non-group/world-writable; the final `overrides/` and per-project directories must be private root-owned. Opens traverse with descriptor-relative `openat(O_DIRECTORY|O_NOFOLLOW)`; all generation files are root-owned `0600` single-link regular files. A random private temporary file is written and synchronized, atomically installed with Linux `renameat2(RENAME_NOREPLACE)`, and the directory fsynced. A file with the same generation name is **never replaced**, including when it is a symlink. The exact new file is reopened with `O_NOFOLLOW`, metadata/inode stability is checked, and its contents must hash to the prior durable intent before the `staged` event is committed.

If an error or process interruption occurs at **any** cross-domain point, the prior intent (or the staged event) remains durable, and new same-project work stays blocked. It cannot be marked successful or retried blindly; the operator must eventually reconcile filesystem, journal, committed desired, and independently inspected actual Docker state.

## Promotion history without promotion authority

After a separate `STAGE` phase completion and recorded `VERIFY` and `COMMIT` journal intents (currently journal-only/fake tested), the library may append a **promotion_intent** record bound to the original operation, previous generation/hash, immutable file digest and held project lock. It does **not** rename a real Compose override, change `override_records`, publish a receipt, execute Docker, verify image health, or mark an operation `succeeded`.

No `promotion_completed` kind or production API exists in v2. `Store.Complete(..., "COMMIT", ...)` rejects a proposed completion when the operation has any uncommitted override generation events. `InspectOverrideGeneration` always reports `Committed=false` and `NeedsReconciliation=true`. Phase 3b's `ReconcileReadOnly` now treats **any** v2 generation event as unfinished and reports the sanitized reason `override_generation_incomplete` rather than comparing it as committed desired. Even a prior exact fake Docker match cannot clear the block.

## CI boundaries, review and remaining gates

Pure SQLite tests verify v1→v2 transactional migration without losing journal rows, damaged-schema refusal, immutable event triggers, unknown event types, typed manifest validation, and the unchanged v1 committed-history semantics. Root-owned disposable CI fixtures exercise project lock ownership, trusted directories, no-overwrite/symlink/hardlink refusal, stage/promotion intent progression, duplicate-replay rejection, and injected crashes after SQLite stage intent, after private-file fsync/rename and after recorded staged event. The interrupted journal is closed and reopened to ensure uncertainty cannot disappear.

**Known limits:** There is no independent OCI/Compose verification, safe Compose YAML serialization, committed override promotion, supported Docker version qualification, real actual-state observation, or operator-confirmed deployment. The future implementation must design a transactional **journal/file/actual-state** reconciliation and a separate, independently verified promotion-completion gate. It must not assume SQLite transactions and filesystem renames are atomic together.

Phase 3c next includes sanitized idempotent receipt publication and WAL-consistent backup/controlled restore with generation divergence tests. B6-04 still gates the true real-state authorization path, B6-05 gates Docker mutations, and B6-08 gates off-host backup and host acceptance. Live `hello-go` group-writable parent `0775` remains ineligible.

**Governance:** This PR itself does not create production override directories, journal files or Compose overrides. Do not merge, publish, install, change real host permissions, deploy an executor, enable Docker writes or tag a release without explicit separate operator approval.
