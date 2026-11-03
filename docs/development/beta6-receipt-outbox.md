# Beta.6 Phase 3c-b — Sanitized receipt outbox and idempotent publication

**Status:** Proposed CI-only library under [B6-03 issue #83](https://github.com/docklantern/docklantern/issues/83). No monitor web integration, production journal/receipt directory, Docker action, host mount, release, installation, or VPS change is included.

## Authority and report format

The executor derives a **version 1** JSON `Receipt` from its separate, protected SQLite operation journal and its immutable phase events. The canonical serializer and strict `ParseReceipt` only accept the documented fields: `schema_version`, `operation_id`, `proposal_id`, `registration_id`, `service`, `phase`, `outcome`, immutable `candidate_digest`, `updated_at`, and fixed `message_code`. The payload limit is **8 KiB**. Arbitrary strings, raw errors, environment variables, Docker/Compose output, source paths, credentials and extra/duplicate JSON keys are rejected. The serializer produces deterministic bytes; the journal records their exact SHA-256.

The **only currently permitted outcomes** are `pending` or `needs_reconciliation`. Supported message codes are `operation_pending`, `interrupted`, and `verification_incomplete`. Existing `succeeded`, `recovered` or other terminal database claims are **never published** because Phase 3 lacks actual Docker/Compose/OCI verification and the independently approved terminal success transition. A receipt **cannot authorize execution, certify the actual container state, close a journal operation, release a lock or promote an override**.

## Fixed outbox and strict filesystem isolation

`Store.PublishReceipt(ctx, heldProjectLock, operationID, at)` is host **root-EUID only** and uses the fixed `/var/lib/docklantern/handoff/receipts` directory. The caller cannot supply a destination path, Docker host, raw receipt, arbitrary status or monitor-controlled UID. The operator must separately provision the directory; this PR does not create a production mount or modify the monitor. The only overrideable directory parameter is **unexported** for isolated root-owned CI fixtures.

All ancestors are opened with descriptor-relative `openat(O_DIRECTORY|O_NOFOLLOW)` and must be root-owned, non-group/world-writable. The final outbox directory must be root-owned `0755` for sanitized reads. The writer requires a held root-only `ProjectLock` whose project matches the protected operation; no lock may be supplied by the monitor. Final JSON files are root-owned, single-link regular **0644**, under `<opaque-operation-id>-v1.json`. The monitoring container must receive **only this directory as a read-only mount**, and cannot write its contents or any executor journal, registration or private override. Provisioning and read-only mount acceptance remain separate security/deployment gates.

## Non-atomic SQLite/file crash boundary

The publisher uses the pre-existing **append-only SQLite `receipt_publications`** table; no migration or monitoring database writes are needed. Every publication is:

1. Derive a bounded deterministic receipt from a consistent journal snapshot and validate its operation/project identity against the held lock.
2. Persist a `pending` publication event **before file creation**. A replay of the same version/hash is idempotent; conflicting hashes, incomplete metadata or unsupported terminal outcomes fail closed.
3. Recheck the current journal snapshot. Write a random private temp, fsync content, set its sanitized final `0644` mode, atomically install using Linux `renameat2(RENAME_NOREPLACE)`, fsync the directory, then reopen and verify the exact inode, owner, one-link mode, bounded canonical JSON and SHA-256.
4. Recheck that the journal still derives the exact same bytes, then append one `published` event. Repeating this call verifies the existing final file and returns the previous publication without rewriting it or adding journal rows.

A crash before pending leaves no file. A crash after pending but before rename leaves pending evidence. A crash after file rename but before the `published` event leaves a final immutable receipt **plus pending journal state**; retry verifies its exact bytes and inserts the missing durable event, never overwriting it. A crash after the `published` event is idempotently verified on retry. Any changed journal, wrong hash, symlink/hardlink/FIFO, missing/inconsistent file, or failed fsync is an **ambiguous/fail-closed publication**, not an operation success. An already-published receipt is immutable; if the operation later advances, the changed snapshot cannot silently replace version 1.

The journal-only `ReceiptState` helper reports `unknown`, `pending` or `published` **as a journal record state, not a filesystem verification or successful deployment**. In particular a missing/stale browser-visible receipt must be shown as unknown/pending; actual operation state must be independently reconciled using the private executor journal and real Docker inspection when that later phase is authorized.

## Bounded history, retention and failure behavior

Publication enforces **1,000 final receipts maximum**, bounded enumeration of at most **1,024 directory entries**, **8 KiB** each, and only known final/temp filename patterns. Capacity exhaustion fails closed with a durable `pending` record; the writer never rotates, deletes or hides earlier results to make room. A random leftover temp from a crash is not interpreted as a published receipt, and unknown filenames fail new publication. **The 90-day ordinary GUI retention target is not implemented in this first slice** because Phase 3 cannot yet prove whether an older operation has safely terminated. Automatic cleanup would risk hiding an unresolved operation. Retention/pruning requires a separate reviewed implementation backed by durable terminal-state evidence; until then the bounded outbox may fill and must refuse new publications instead of deleting audit history.

## CI tests and remaining gates

Pure SQLite tests check strict canonical JSON and unknown/duplicate-key rejection, no secret-bearing fields, pending/uncertain code mapping, idempotent `pending→published` event recording, immutable conflicting hashes and refusal to expose fabricated `succeeded` outcomes. New disposable root-owned CI fixtures test no-overwrite private `0644` files, replay after crashes at all three boundaries, symlink/hardlink/FIFO/ownership/mode and parent-path attacks, a full 1,000-file outbox, and a changed journal snapshot between reservation and publication.

**Not delivered:** Actual monitoring GUI integration or read-only mount, executor installation, real Docker/Compose/OCI evidence, authorized file promotion, durable successful-operation finalization, receipt expiry cleanup, WAL backup/restore, off-host backup or deployment. Phase 3c-c separately handles consistent WAL backup/controlled restore; B6-04 handles actual-state preflight/terminal authorization; B6-05 Docker mutations and B6-08 real-host/offsite acceptance remain blocked. Existing `status`, `reconcile`, `apply`, `recover` CLI commands remain disabled and beta.5 monitoring stays unchanged.

**Governance:** No PR merge, release, installation, production directory/mount provisioning, VPS permission changes, real Docker operation or deployment without a fresh explicit operator approval.
