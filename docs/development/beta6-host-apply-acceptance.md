# beta.6 B6-05 — consolidated guarded host-native image apply

This implementation targets **only one registered public Docker Hub linux/amd64 service**. It is not a general Compose orchestrator, unattended updater, or web deployment bridge.

## Actual execution boundary

The separately installed **root host CLI** now accepts `docklantern-executor apply <proposal-id>`. It reads a nonroot monitor-published proposal and root-owned protected registration; generates a unique operation ID; takes an exclusive project lock; opens the maintenance-leased WAL SQLite journal; obtains a fresh qualified OCI/Compose/actual image state; shows an independent host-local `APPLY <nonce-bound challenge>`; and independently requalifies exact proposal, registration, source, container and image state.

The native effect adapter runs only explicitly constructed, fixed-socket Docker operations. It checks the beta.6 pinned Engine **29.8.1**, API **1.56** and Compose **v5.5.1**; pulls the approved immutable public registry digest; stages and rechecks trusted Compose/env sources; captures every Compose-project container identity; and executes `compose up --no-deps --no-build --pull never --force-recreate -d <selected-service>` with only the selected image override through stdin.

After replacement, it independently inspects the selected container and its local image ID; requires running status and either healthy healthcheck or an explicit protected `allow_running_only` registration; verifies every sibling container remains running, with identical ID, image, labels, networks and mounts; and rechecks source bytes and actual Docker evidence **again** before journal COMMIT.

All phase intents are committed before effects. An ambiguous or interrupted outcome blocks new project work. No rollback or Docker group privilege is granted to the monitor. The root CLI prints no raw Docker logs, Compose/environment bytes or credentials.

## Durable verified outcome

The host-root `CommitVerified` API requires the held project kernel lock and a typed read-after-write witness. One full SQLite transaction inserts baseline evidence, records the next desired image generation, appends COMMIT completion and marks the operation `succeeded`. A Compose subprocess exit alone cannot invoke the commit. A missing health/sibling/source witness must leave an unresolved journal operation.

Deployment receipt `succeeded` status is legal only when the journal contains exactly the completed eight-phase intent/completion sequence. The GUI's read-only mirror may display this sanitized status if a separately approved host-to-monitor read-only mount is provisioned.

## Explicit scope cuts

To finish beta.6, this first path defers self-updates, service groups, private registries, nonstandard YAML, volume-backed services, backups of application data and unattended or automatic rollback. The historical image cannot safely reverse external database mutations. A failed or interrupted update requires a separate manual reconciliation/repair decision; **do not run another apply against a blocked journal**.

## Acceptance and VPS gates

This PR is code and disposable-CI-only development under the operator's prior authorization. It does not approve an executor installation, Compose mutation or beta.6 image deployment on the VPS. A real-host apply requires **new explicit approval**, a verified off-host monitoring/journal backup and completed restore drill, a read-only baseline, and separate authorization of the exact candidate and target service. The existing beta.5 monitoring image must remain reversible independently of the executor.

Review all final CI security/go/unit/Compose/disposable update jobs, and validate the actual executor binary provenance before release. Do not tag/publish or merge without explicit operator approval.
