# Beta.6 B6-05 — first approved disposable apply prototype

**Approval scope:** Operator explicitly approved B6-05 development and **Docker mutations on isolated, disposable CI test services only**. This does not authorize a VPS write, unattended production execution, PR merge, release, or deployment.

This batch is intentionally split into two independently testable pieces. Neither is wired to the production host CLI's reserved `apply` or `recover` commands.

## 1. Fail-closed journal phase coordinator

`internal/executorapply.RunCandidate` defines a typed, dependency-injected B6-05 coordinator that requires an externally supplied read-only qualification matching the operation and a host-only approval implementation. The approved effect sequence is PREPARE, AUTHORIZE, PREFLIGHT, immutable PULL, override STAGE, targeted APPLY and target/sibling VERIFY.

Each phase is journaled with `Intent` **before** any effect and `Complete` afterward. Every error denies further effects and leaves the project unresolved, marking uncertainty where possible. Missing or ambiguous storage acknowledgement is **not** retried. No command strings, shell expressions, registry credentials or user-supplied host paths reach the coordinator. Tests inject failures at each pre-effect intent, effect and post-effect completion boundary and assert that no subsequent effect can happen. The durable journal contract already blocks a new operation for an unresolved project after process restart.

**It intentionally stops after VERIFY without COMMIT or success** because the existing executor journal cannot yet confirm override promotion and terminal verification atomically. Neither a positive verification nor operator approval alone is a success receipt. A subsequent independently reviewed production integration will have to hold the trusted project lock, source handoff, exact qualified Docker/Compose runtime, fresh proposal/registration/actual baseline and separate interactive terminal confirmation **through** every potential Docker side effect.

## 2. Real targeted Docker Compose replacement fixture

A new opt-in GitHub Actions job runs on an isolated hosted runner with explicit `DOCKLANTERN_DISPOSABLE_APPLY_TEST=1`. It creates a private temporary Compose project and external network, a disposable localhost-only registry and prebuilt images. It verifies the fail-before-replacement case when a candidate digest is missing; performs a selected-service-only update from a separately pushed immutable local registry digest using `compose -f - up --no-deps --no-build --pull never --force-recreate -d target`; confirms the selected container/image changes, the target remains running, and the sibling's container ID/image/labels/networks/mounts remain identical; and verifies that the original Compose YAML remains unmodified. A trap removes only the unique CI fixture's resources.

This is **real Docker mutation** restricted to the explicitly approved disposable CI environment. The localhost registry is a fixture **only**: it is not evidence of production Docker Hub anonymous OCI trust, production source qualification, health verification, or a safe terminal approval. The shell fixture is not the coordinator's production Environment adapter.

## B6-05 still blocked for host use

- Connect the protected journal, kernel project lock, independently requalified source snapshot and dedicated **APPLY** TTY confirmation to one narrowly typed host-only adapter.
- Qualify an exact Docker Engine 29.8.1 / API 1.56 / Compose v5.5.1 write runtime and an immutable image resolution strategy.
- Implement durable verified override promotion and complete success/outcome recording only after independent health, container, sibling and data-safety checks.
- Inject service-health failure, unexpected external replacement, SIGKILL at each phase, journal persistence uncertainty and manual-only recovery.
- Obtain separately approved test-VPS maintenance, off-host backup/restore and deployment decisions.

No GUI write surface, no unattended updates, no recover, no auto-rollback and no release are part of this PR.
