# Beta.3 SQLite schema upgrade (implementation phase #31)

**Status:** Schema foundation for beta.3. This is **not yet** the label-enrollment implementation. The deployed beta.2 app remains unchanged until a later, explicitly authorized beta.3 rollout.

## What changes

Opening a beta.2 SQLite database (schema `PRAGMA user_version = 0`) upgrades it to **schema version 1** inside a single transaction. A failed schema migration rolls back; an unsupported newer schema is rejected. Opening a version-1 database again is idempotent.

Existing `registrations`, `monitor_policies`, `inventory`, `registry_checks`, `monitor_events`, `notification_settings`, and worker status remain intact. Previous legacy column upgrades (inventory image metadata and monitoring failure counter) are retained.

New tables:

- `monitoring_defaults`: exactly one durable global interval, initially **300 seconds** (valid 60–86,400 seconds).
- `monitor_service_settings`: per-Compose **project/service** monitoring-only configuration with separate `legacy_enrolled`, `gui_enrolled`, nullable `label_opt_in`, nullable label/GUI interval overrides, and discovery state/last-seen/validation details. There is **no** dependency on a Compose source registration in this metadata table.

**Every pre-existing beta.2 monitoring policy** is tagged `legacy_enrolled=1` during the one-time migration. This does **not** grant any new deployment capability. The code that saves a manual monitoring policy also maintains its provenance in the same SQLite transaction.

Schema version 1 intentionally **does not** start scanning labels, enroll discovered services, or change current registry check scheduling; those are subsequent beta.3 issues [#29](https://github.com/docklantern/docklantern/issues/29) and [#30](https://github.com/docklantern/docklantern/issues/30).

## Before any real-host upgrade

1. Read the deployment repository's [consistent SQLite backup procedure](https://github.com/docklantern/docklantern-deploy/blob/main/docs/operations.md#consistent-sqlite-backup-while-running).
2. Produce an application-backed SQLite snapshot using the documented `-backup` mechanism; **do not copy the live database directly**, because SQLite may use write-ahead logging.
3. Verify `PRAGMA integrity_check` returns `ok` on the **backed-up** database, and store a copy off-host or in secure external storage.
4. Note the currently running image tag/digest and capture relevant operational status. Keep the existing beta.2 image available for rollback.
5. Ensure the documented deployment's optional third Compose override (`compose.projects.yaml`) remains included wherever an operator already uses it.

## Rollback and recovery

**Do not assume the older beta.2 binary can safely open schema version 1.** If reverting, stop DockLantern, verify the pre-upgrade backup, and restore the **entire pre-upgrade SQLite state** per the [deployment operations guide](https://github.com/docklantern/docklantern-deploy/blob/main/docs/operations.md#restore-from-a-known-good-sqlite-backup). Move the old `.db`, `-wal`, and `-shm` aside before restoring; never mix a restored DB with stale journals. Start a *single* process, verify health and all history.

A fresh installation has no legacy monitoring policies and receives the default 300-second configuration. It does not enroll any new service until the subsequent label/GUI feature ships. This upgrade never pulls an image or restarts a monitored container.

## Validation

Unit tests construct a real version-zero beta.2 fixture with registrations for two Compose projects, schedules and retry state, registry digest baseline, unchanged check status, a pending digest-change event, notification preferences, and an inventory snapshot. They verify:

- Exact preservation of existing policy schedules, backoff, event identity and delivery state, registry baselines, and configuration.
- Migration and repeated re-open are idempotent; the global default persists.
- Newly saved manual policies retain legacy provenance and do not create duplicate settings records.
- Older missing inventory columns are migrated within the same transaction.
- A damaged/incompatible schema rolls back without incrementing `user_version` or losing any pre-existing records.
- A database written by a newer application version fails closed rather than silently downgrading.

**No real host changes are performed as part of Issue #31.**
