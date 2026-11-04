# beta.6 B6-06 — read-only Deployment Activity and test-VPS candidate scope

This UI batch makes the beta.6 image testable on the existing monitoring VPS *without* granting a web-to-executor bridge. The monitor still uses only its GET-restricted Docker proxy and never invokes Compose, Docker CLI, sudo, or the executor.

## Read-only Activity

The authenticated `GET /deployments` page shows bounded executor-originated receipt summaries (operation identifier, service, phase, outcome, candidate digest, time). No POST/PUT/DELETE is available at the path. A missing receipt mount displays **unknown**, not `healthy`, `up-to-date` or `succeeded`. A malformed, untrusted, symlinked, mismatched, oversized or unexpectedly writable receipt outbox fails closed; no raw receipt data is displayed.

The monitor reads only the **fixed** path `/run/docklantern/executor-receipts`. That path is never created by the web server, read from an HTTP parameter or fetched from the host through Docker. To display receipts on an approved host, the operator must independently configure a **read-only**, root-produced mount of the executor's sanitized outbox `/var/lib/docklantern/handoff/receipts`. Do not mount the executor journal, trusted registration, Compose inputs, private override tree or Docker socket. Without that separate host configuration the page intentionally shows the unknown state.

Current receipt schema v1 has **only** `pending` and `needs_reconciliation` outcomes. Thus the UI must not and does not report an update as successful. After B6-05 gains verified committed outcomes, update the strict receipt schema and GUI in the same reviewed change. Stale receipts older than 24 hours are labeled stale, never assumed current.

Service Details now explicitly distinguishes monitored enrollment from executor deployment eligibility, and links to deployment activity. A remote registry digest change is never represented as a verified deployable update.

## Accelerated development / beta.6 candidate policy

To reduce remaining PR count, group the remaining work into **one larger executor-closure PR** (B6-05, concrete root host adapter, on-demand targeted apply, journaled success/uncertainty, safe manual recovery) and **one release/acceptance PR** (B6-06 completion with any missing proposal view, B6-07 CI, B6-08 runbook/backup/release evidence). Do not call B6-05 complete until a real, secure host-native executor can execute its approved narrow single-service path.

The user plans to test the beta.6 image on their VPS, but that future intention does not authorize an installation or Docker write now. It is reasonable to deploy the **monitoring image only** as a separate, explicitly approved validation step while keeping executor write commands disabled. Real host-executor apply is a separate operation and must retain exact version/source approval gates. Before executing any real-host apply, verify an off-host backup and complete restore test.

Initial scope remains a single-replica, public Docker Hub, linux/amd64 prebuilt hello service. Defer private registries, multi-service updates, generalized Compose constructs, auto-rollback, unattended updates and self-updating. An ambiguous outcome remains blocked for manual reconciliation.
