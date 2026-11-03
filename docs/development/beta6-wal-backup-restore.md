# Beta.6 Phase 3c-c — WAL snapshot and quarantined restore foundation

**Status:** Review-only, isolated root CI fixture. No operational CLI, host provisioning, executor installation or real Docker/Compose interactions.

## What this slice does

- Takes a SQLite-consistent independent snapshot with `VACUUM INTO`, including committed WAL transactions (never copying a live `.sqlite` file and assuming that its WAL data is included).
- Uses independently preprovisioned root-owned private 0700 storage, nofollow ancestor traversal, owner/mode/single-link checks, random filenames, `fsync`, and `RENAME_NOREPLACE`. A candidate snapshot is **not trusted** until exact SHA-256, manifest metadata, SQLite `integrity_check`, foreign-key check, application ID, schema version and protected table/trigger structure all validate.
- Keeps a manifest separately from the backup. A crash before manifest publication leaves an orphan; no caller may interpret an orphan as a usable backup. No deletion/rotation or silent repair.
- Stages a requested validated snapshot into an **empty**, separate private quarantine. Staging never replaces the active executor journal, cannot clear pending operations, never unlinks live WAL/SHM, and exposes no write/mutation authority.
- Fails closed for corruption, unsafe directories or files, occupied quarantine, duplicate/noncanonical backup identifiers, unsupported schema and interrupted snapshot/staging.
- Uses root-only test fixtures beneath `/root` with an explicitly enabled fixture environment flag.

## Limitations and why restore activation remains blocked

This is a **foundation**, not the full Phase 3c-c completion gate. No exported production backup/restore command exists. A Store-local mutex only serializes writers using *that Store instance*; it is not a global host-wide barrier against other executor instances. For production activation, a protected global backup/maintenance lock, executor quiescence, bounded disk/capacity policy and transactional recovery coordination require a separate review.

A valid SQLite snapshot is **not** evidence of current desired or actual Docker state. A restored older DB can lack later override staging, journal events, published receipts and actual Docker changes. This library intentionally stops at quarantine rather than swapping it into `operations.sqlite`. An independently approved offline activation must preserve old live files, rule out active connections and WAL/SHM, validate histories against separately protected generations/receipts, and require read-only external reconciliation. Missing histories or mismatched committed generations must remain blocked, not be inferred safe from an old journal.

No test here qualifies real Docker/Compose/OCI observations. The Phase 3b `ReconcileReadOnly` fake evidence remains informational and blocks new work even if it matches. B6-04 terminal real-state authorization, B6-05 Docker write boundary and B6-08 off-host restore drill remain separate hard gates.

## Approval

Do not merge, enable a host-local command, provision a production backup directory, restore a VPS database, deploy, publish release/tag or perform Docker mutations without distinct operator approval. Keep [B6-03 #83](https://github.com/docklantern/docklantern/issues/83) open until offline activation and divergence acceptance tests are separately completed.
