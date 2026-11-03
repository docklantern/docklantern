# Beta.6 Phase 3c-c follow-up — Read-only restored journal history comparison

**Status:** Separate CI-only library slice, after merged PR #105. No production restore activation, executor CLI, VPS work, or Docker/Compose actions.

A validated SQLite backup is not sufficient to authorize replacement of the executor journal. The private `compareRestoreJournal` helper checks exact contents of all six authority-bearing tables from a consistent active Store snapshot versus a previously validated independent backup snapshot: operations, phase events, baselines, committed overrides, override-generation events and receipt-publication history. A new phase, newer operation, staged generation, committed override or receipt event causes `journal_history_diverged` and **blocks work**. A matching result is explicitly `rows_match_unqualified` with `BlocksNewWork=true`: it never certifies live Docker/Compose actual state or authorizes a restore.

The comparison uses bounded, deterministic row digests so it never returns sensitive row contents. The candidate is checked for `integrity_check`, foreign key violations, supported schema version, application ID and necessary tables/triggers. The active Store mutex prevents concurrent writers through the same Store session during the comparison.

## Limitations

This is a diagnostic *comparison*, not a host-global quiescence mechanism. Independent executor processes may still write. SQLite rows also do not account for out-of-transaction filesystem override generations, already published outbox files or real container state. No complete host-integration or offline restore command is present. The comparison deliberately does not mark any operation successful, clear journal uncertainty, delete WAL, or replace live data.

To authorize any future offline restore, the operator must independently freeze all executor processes, preserve old live journal/WAL and protected files, compare generations and receipts from protected files, qualify fresh actual Docker/Compose state and perform a controlled, audited activation with rollback/reconciliation semantics. These remain subsequent review gates, as does B6-08 off-host backup and VPS acceptance testing.

PR merge, deployment, release and actual host filesystem changes require distinct approval. [B6-03 #83](https://github.com/docklantern/docklantern/issues/83) stays open.
