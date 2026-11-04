# Beta.6 B6-05 — bind APPLY to exact protected record bytes

Following merged PR #125, the host-local APPLY gate and the journal-first candidate coordinator now require the exact SHA-256 of **both** the protected proposal and protected registration in the read-only qualification result. Previously, the check compared candidate digest, service, source and actual Docker identity; a changed protected record with the same visible candidate was not part of the challenge identity.

`executorpreflight.Qualify` now carries `ProposalSHA256` and `RegistrationSHA256` directly from the verified protected record snapshots. `QualifyFromSource` continues to reopen both records after potentially slow OCI/Docker/Compose inspection and rejects exact-byte changes. The apply coordinator binds the proposal hash to the journal operation's immutable `ProposalSHA256` and rejects malformed registration hashes before `Begin`. The terminal confirmation challenge includes both hashes, and requalification must match both **after operator input** and again in the journaled PREFLIGHT phase. A registration replacement is therefore rejected even when its service/repository allowlist and visible candidate are otherwise unchanged.

The `qualify` CLI still reports **read-only evidence only**, never an authorization token; adding these fingerprints does not grant Docker write permission. New failure-injection tests change each protected record hash during the terminal prompt and assert no journal begin or Docker effect occurs. Existing protected root-only, pinned Compose v5.5.1, disposable-only Docker update and security workflows remain required.

## Remaining work

This is a strict authority-binding hardening batch, not a production `apply` enablement. The approved B6-05 milestone still requires a concrete non-injectable protected host adapter with `AcquireProjectLock`, `OpenHost`, a controlling `/dev/tty`, a stable source handoff held **through** the write subprocess, exact Engine/API/Compose write qualification, independently verified container health and sibling invariance, and a durable committed override/promotion outcome. An injected prototype may not publish a success receipt or be exposed through the monitoring web service.

The user's explicit approval covers only development and Docker mutations on disposable CI test services, not VPS work, release publication or merging PRs.
