# DockLantern Architecture Overview

**Status:** v0.1.0-beta.5 monitoring-only release accepted on test VPS; beta.6 on-demand host executor architecture accepted for planning, **not implemented**.

## Current production-facing beta architecture (through beta.5)

- **Dashboard/API + registry monitor (Go):** authenticated web GUI, Docker discovery via private GET-only socket proxy, bounded OCI polling, watched-tag overrides, Check Now, diagnostics, and notifications.
- **Monitoring SQLite:** monitoring policies, image/platform registry evidence, read-only Docker inventory and events.
- **Docker socket proxy:** restricted read-only discovery; the monitoring process has no direct Docker socket, Compose CLI or Docker write privileges.

Monitoring enrollment via Docker label or GUI does **not** authorize deployment. A watched registry tag is neither the source Compose image nor a desired deployment version.

## Accepted beta.6 architecture — not yet implemented

- **Monitoring GUI/controller:** may produce bounded untrusted single-service deployment proposals and display sanitized execution receipts; cannot invoke the executor or mutate Docker.
- **On-demand host-native `docklantern-executor` CLI (Go):** a separate host artifact requiring local/SSH operator invocation, independently protected project/service allowlists, validated Docker Compose CLI, targeted immutable-image replacement, durable journal and manual recovery.
- **Filesystem handoff:** monitor-writable proposal inbox, executor-only trusted state, executor-written sanitized results read-only to the monitor. No privileged RPC or always-running executor service.
- **Executor SQLite state:** protected registrations/journal/locks/committed overrides, distinct from monitor SQLite. Both require appropriate backups.

See [ADR 0008](decisions/0008-on-demand-compose-executor.md). ADR 0005's former in-process privileged executor design is historical and superseded.

## Source / watched / candidate / desired / actual

1. **Source:** operator-controlled Compose definitions and interpolation inputs.
2. **Watched:** an OCI registry reference monitored for changes, never deployment authority.
3. **Candidate:** the specific, platform-verified immutable image proposed for deployment.
4. **Committed desired:** executor-maintained image-only managed override applied after source files.
5. **Actual:** running Docker container/image and health/readiness evidence.

A changed registry digest is not proof that a semantic version is newer. An image pull is not an application deployment.

## Managed Compose update design

Only explicit host-local approved registrations can be deployed. Preserve original file order, project directory, env interpolation and profiles; only the authorized selected service image may differ. Unsupported configurations remain monitoring-only.

The executor will preserve the committed managed override, stage an operation-specific image-only override, validate effective Compose config, pull the immutable image, perform selected-service Compose replacement, verify actual outcome, and **only then** promote the candidate to committed desired state. Compose sources remain untouched.

Durably journal intent before side effects, serialize per project, and reconcile incomplete operations without automatic replay. Failed/ambiguous operations block further execution until inspected. Manual image recovery requires a new operator approval and does not promise data rollback.

## Discovery and onboarding

The beta.3+ monitor uses Compose labels/GUI enrollment without source mounts; explicit `monitor=false` remains an opt-out. Executor registration, environment reconstruction and filesystem trust are separately required for beta.6 deployment. A monitoring-era registration or label cannot be upgraded silently into deployment permission. See ADRs [0003](decisions/0003-project-discovery.md) and [0007](decisions/0007-label-enrollment-monitoring-only.md).

## External management

External Compose/CI deployments are not silently reversed. Managed override omission can result in drift. Detect and show discrepancies; broader Adopt/Restore/Ignore workflows from ADR 0004 are deferred.

## Platform and security

Target a single local Linux Docker Engine on small VPS/home-lab hosts; IPv6-only egress must be tested. Use trusted host Docker/Compose executable paths, bounded execution, validated file ownership, redacted previews/results and independent local operator approval. Docker write permission remains effectively host-administrative **during** an executor invocation.

Do not use historical ADR 0005 privileged-socket mount examples as a current deployment recipe. Neither this architecture decision nor a documentation merge enables actual Compose updates, VPS modifications, release publication or automatic updating.
