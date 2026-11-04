# Beta.6 Phase 3c-c — inverse protected artifact inventory

**Status:** Review-only, root CI fixture. No operational restoration or Docker execution.

Following merged PR #109, the private `auditRestoreInventoryAt` runs the previously reviewed forward reference audit, then walks the separately protected override and receipt directories in bounded, nofollow-validated form. It compares **every** generation metadata filename and final receipt filename against a validated candidate snapshot's journal references. A later generation, later project directory, unknown receipt, noncanonical name, leftover staging temp or unexpected filesystem entry is **not** ignored and results in `needs_reconciliation`. Missing referenced files also remain blocked.

Even a complete bidirectional match returns `rows_match_unqualified`, `BlocksNewWork=true`. Neither this function nor its callers can replace SQLite, delete files, declare a Compose operation successful, publish authorization or clear a pending journal operation.

Tests use disposable root-owned CI directories to verify a matching inventory remains blocked, later generations/other projects/receipts, dangling temporary files and unsafe symbolic links are refused.

## Remaining hard gates

This is a read-only snapshot of directory listings, **not** an atomic snapshot of other host filesystems. Any eventual operational caller must first prove *exclusive maintenance quiescence*, rule out all uncooperative writers and preserve the old journal and WAL/SHM. The inventory does not examine live Docker/Compose containers, actual image digests, desired Compose sources, published GUI state, or host privilege boundaries. Those require distinct offline real-state verification and operator approval. Restore activation, capacity/retention, crash-safe activation/rollback and B6-08 off-host restore drill are not implemented here.

No host provisioning, executor CLI exposure, release, deployment, or Docker mutation. B6-03 issue #83 remains open.
