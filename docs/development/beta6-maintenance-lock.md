# Beta.6 Phase 3c-c — host maintenance lock primitive

**Status:** CI-only primitive after merged PR #106. This is not a live maintenance mode, restore activation, or Docker execution gate.

`AcquireMaintenanceLease(exclusive)` acquires a root-only Linux `flock` from a fixed, preprovisioned root-private directory, on the persistent `_maintenance.lock` inode. Shared leases are intended for future cooperating executor operations; exclusive leases for host-local offline maintenance. Both modes acquire nonblocking, fail-closed locks; exclusive cannot coexist with readers or another exclusive. The file is never removed on unlock. It is opened with nofollow flags and verified root-owned, 0600, singly linked and inode-stable after lock acquisition. An untrusted or writable ancestor is rejected.

Root-only disposable CI tests verify shared/shared compatibility, exclusive conflicts, separate-process conflicts, SIGKILL release, double-release prevention and unsafe inode/directory rejection.

## Critical limits and next gates

This change adds a **primitive only**: existing operation paths are *not yet wired* to hold shared leases, and no production backup or restore command obtains exclusive maintenance. Therefore **an exclusive lease today cannot be interpreted as proving executor quiescence**. A subsequent separately reviewed integration must ensure every journal writer, receipt publisher, override generation writer and operation holds a shared lease before modifying protected state. The future exclusive maintenance path must acquire the lock, prove no unrelated writer/SQLite connection, preserve active journal/WAL/SHM and protected generation/outbox state and never automatically activate a restore.

This is advisory coordination among cooperative DockLantern executors. It cannot fence other root processes, arbitrary Docker CLI, filesystem writers, or real Docker state. On-disk comparison still cannot establish actual-state qualification. Phase 3c-c remains incomplete; B6-04/B6-05/B6-08 remain separate.

Do not merge the PR, install an executor, provision `/var/lib/docklantern/executor/locks`, restore a VPS journal, deploy or publish a release without explicit operator approval. [B6-03 #83](https://github.com/docklantern/docklantern/issues/83) remains open.
