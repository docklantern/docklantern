# Beta.6 Phase 3c-c — fenced host journal Store lifetime

**Status:** PR-only, root-owned CI-fixture implementation. No production executor/restore CLI integration, release, VPS provisioning or Docker mutations.

`OpenHost` now must acquire a shared `_maintenance.lock` lease **before** opening, creating or migrating the root-private SQLite journal. It retains that lease until the returned `Store.Close` closes SQLite, including WAL/shm-related connections. A host maintenance holder with the exclusive lease prevents any cooperating new `OpenHost` call; a live `Store` (and all its journal writes/override generation/receipt publishing methods) excludes a cooperating exclusive holder.

An unsuccessful host open releases the lease. A missing or unsafe lock directory fails closed **before** creating a journal. No production directories are created by this PR; provisioning `/var/lib/docklantern/executor/locks` and `/var/lib/docklantern/executor/journal` remains an independent operator action.

Root-only test fixtures verify two simultaneous shared Store sessions, exclusive contention while live, release on Close, refusal to open while exclusive is held, refusal of unsafe/missing lock directory, and release on unsupported future-schema open failure.

## Still not a restore authorization

The lock is advisory, not a process killer or OS-wide filesystem freeze. Existing internal `openProtectedJournal` and `openSQLite` remain private for test-only internal fixture paths and do not imply host fencing. Any future executor that uses those private paths without the shared lease would violate the host protocol. Production CLI entrypoints **still do not call** `OpenHost`, so no operational behavior is enabled by merging this PR.

The exclusive holder is not yet bound to a reviewed offline restore workflow and does not validate override generations, published receipt filesystem evidence or real Docker state. Backup/restore quarantine and history comparisons remain evidence only. No journal replacement or automatic recovery is enabled. Future implementation must verify every production writer path uses `OpenHost`/shared lease, preserve existing journal + WAL + SHM before activation, fail closed on external divergence, and independently authorize offline restore.

B6-03 #83 remains open. B6-04 real-state read-only qualification, B6-05 mutation boundary, and B6-08 off-host restoration remain separate.
