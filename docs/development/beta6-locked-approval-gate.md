# Beta.6 B6-05 — locked host-local APPLY authorization gate

Following merged PR #124, the proof-of-concept coordinator now has a separate **host execution gate**, `RunLockedCandidate`. It is an injected component for deterministic tests, **not** exposed through the production CLI or web monitor. There is no new executable Docker write adapter or unattended automation.

## Required order

1. Demand a trusted **host-root** authority and actual interactive controlling terminal. The eventual production adapter must use `executorjournal.AcquireProjectLock`, `executorjournal.OpenHost`, and the established root-only terminal implementation; the test seams do not grant production authority.
2. Acquire the project-specific kernel lease before opening the journal, reading the protected proposal or inspecting Docker.
3. Obtain fresh read-only qualification matching the exact operation, project service, registration, candidate digest and source/actual-state evidence. Limit freshness to 10 seconds.
4. Display a fresh random 96-bit **APPLY** challenge bound to operation identity, candidate, source, actual container/image, platform manifest and nonce. Require an exact `APPLY <challenge>` response on the terminal within two minutes; no stdin, `--yes`, network request or prior `REVIEW` acknowledgment can substitute.
5. Requalify independently under the same lock **after** operator input and reject any change to candidate/source/actual identity. The coordinator also requalifies again during its journaled PREFLIGHT phase.
6. Keep the lock and protected journal session open throughout every subsequent staged phase, and close the journal before releasing the project lease. The prior `RunCandidate` phase coordinator records durable intent before its injected effects, never runs an effect if intent fails, and never declares COMMIT or success without independently verified promotion.

## Tests

The injected gate suite exercises successful challenge, exact phase order and cleanup, wrong/missing challenge, non-root, absent terminal, malformed nonce, stale/future qualification, and a live container-identity change occurring while the operator interacts. Denials happen before beginning an operation and cannot invoke Docker effects. CI's existing isolated real Compose target-only replacement fixture remains separate and approved only for disposable services.

## Hard boundaries remaining

**This is still a non-production prototype.** An injected `IsHostRoot`, `AcquireLock`, `OpenJournal`, `Qualify` and `Environment` must be replaced by strictly audited host-root implementations before the real `apply` CLI can be enabled. The host must hold the same verified Compose stage through actual write operations, validate the exact Engine/API/Compose write runtime, execute a safe OCI-pinned pull, stage and promote an override with a durable committed-generation record, verify health and sibling invariance, and enforce fail-closed crash/manual recovery behavior. In particular, no event or success receipt may be emitted merely because `RunCandidate` reached VERIFY.

The user's authorization permits **disposable CI Docker mutations only**, not VPS mutation, release publication, automatic rollback, PR merges or live deployment.
