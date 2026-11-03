# Beta.6 Phase 2 — Host-local read-only candidate preview

**Status:** Proposed CI-only/no-Docker-write integration for [B6-02 (#82)](https://github.com/docklantern/docklantern/issues/82). This is a **candidate and registration preview**, not an approved deployment preflight. The production binary must not be installed or run on the VPS without separate operator approval.

## Host-local command

```text
docklantern-executor preview <opaque-proposal-id>
```

The command requires a **host root EUID** and an actual controlling **`/dev/tty`**. It accepts only the one validated opaque proposal ID, never a path, UID, Docker endpoint, `--yes`, or arbitrary command. A read-only terminal preview does **not** require typing a deployment confirmation: the separately designed Phase 2c `executorreview.Review` requires qualified *actual state evidence*, which is still unavailable. This informational command never calls that authorization function.

The production preview relies on an independently host-admin-provisioned identity file:

```text
/var/lib/docklantern/executor/monitor.uid
```

That file **must already exist** in the private, root-owned executor subtree as a root-owned, single-hardlink, regular **0600** file. Its entire content must be a single decimal nonzero, nonroot UID (optionally ending in one newline). The executor never creates/chmods it or reads a monitor UID from the proposal, GUI, Docker labels, inherited environment or CLI flags. The Linux reader validates root-owned ancestors and private executor directory, walks using `openat(O_NOFOLLOW)`, and checks the opened file's identity, ownership, size, mode and timestamps before/after reading. Untrusted/missing/ambiguous identity fails closed.

The command loads a monitor-originated JSON proposal from the **fixed untrusted inbox**, using that independently protected UID and the existing Phase 2b bounded descriptor-relative reader. Separately, it reads the executor-private protected registration via Phase 2a's loader, enforcing the exact registration ID, enabled service, approved Docker Hub repository and platform. It requires the proposal to remain unexpired using the host clock and **reopens/re-hashes both records** before rendering the preview. Changed or expired records fail; SHA-256 hashes bind input bytes but are **not signatures, approvals, trusted runtime baselines or reusable capabilities**.

## Explicit preview content and limits

Output is limited to the approved registration and proposal's syntactically validated identifiers: project, service, public repository, watched tag *as hint*, immutable candidate digest *as unverified claim*, requested platform, expiration, hashes and protected `allow_running_only` policy. The preview does **not** print raw JSON, registration input filenames, source/env paths, raw Docker output, secrets or unbounded error text.

It always displays:

```text
Actual Docker/container baseline: NOT INSPECTED
Effective Compose configuration: NOT INSPECTED
OCI digest and platform authenticity: NOT VERIFIED
DEPLOYMENT AUTHORIZATION: NOT GRANTED
```

The command does **not** read the Docker Engine/Compose runtime; has **no OCI registry access**, no source snapshot or normalized config diff, and performs **no Docker or host state modifications**. Importantly, a monitor-provided observed container/image ID is never passed off as independently verified actual state. The existing `diagnose` command is a separate optional read-only environment check, **not** a deployment compatibility endorsement.

The protected record still uses source-ownership pathname `Lstat` preflight, which is **not TOCTOU-proof**; the existing test VPS `hello-go` group-writable parent mode `0775` remains ineligible. Real Engine 29.8.1/Compose 5.5.1 execution is **not qualified**.

## Test evidence and review

Fake-source Go unit tests cover exact protected registration matching, duplicate double-read, changed hash or registration, expired during preview, malformed identity, disabled service, mismatched repository and platform, rejected Docker mutations, invalid CLI flags and sanitized failures. An isolated root-owned CI fixture exercises the **protected monitor UID** reader against symlinks, hardlinks, FIFO, wrong owner, bad modes, writable or nonprivate parents and malformed UID content. It never touches the actual executor path, monitor mounts, Docker or VPS.

**Deferred:** Actual read-only Docker/Compose+OCI state/semantic verification belongs to **B6-04** and depends on review of earlier gates; durable journal/locks belong to B6-03. Complete independent local deployment approval remains blocked until those are qualified; write-capable operations remain B6-05 and later security/backup acceptance. `apply`, `recover`, `reconcile` and `status` stay disabled.

This PR must remain open for review until the operator separately approves merging. It does not authorize a release tag, binary installation, host permission/UID setup, Docker action, or VPS changes.
