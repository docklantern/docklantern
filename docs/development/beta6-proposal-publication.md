# Beta.6 Phase 2 — Bounded proposal publication and retention

**Status:** CI-only library and disposable security fixtures proposed under [B6-02 (#82)](https://github.com/docklantern/docklantern/issues/82). **No monitoring web-app integration, background job, host installation or Docker writes are included.**

## Authority and filesystem boundary

The new `internal/executorproposal` monitor-side functions `PublishProposal(proposal, now)` and `CleanupExpired(now)` operate **only** within the fixed inbox:

```text
/var/lib/docklantern/handoff/proposals/
```

Production requires a **nonroot effective UID** that matches the owner of an already provisioned private `0700` inbox. All ancestors must be root-owned and non-group/world-writable; the open is descriptor-relative with `O_NOFOLLOW` and checks the opened directory using `fstat`. The functions accept **no configurable source/destination pathname**, no UID override, no Docker address, no privileged RPC, and no execution commands. `PublishProposal` serializes a typed v1 proposal (not arbitrary JSON), then parses and validates the resulting exact bytes using the existing strict decoder, clock constraints and 16 KiB maximum. Publication returns the SHA-256 of exactly the published bytes, which **does not grant authorization**.

The monitoring process may own and mutate its untrusted inbox data, but it never gains ownership of the separate root-private executor registrations, journals, receipts or Docker socket. This code is **not yet called by the monitoring web app or its scheduler**. The production host mount/UID topology and operator approval are separate deployment gates; the current monitor container is not reconfigured.

## Atomic publication

The writer takes a nonblocking `flock` on the already opened inbox directory to serialize *cooperating* monitor publishers and cleanup operations; lock contention fails closed. It enumerates **at most 128 total directory entries**, checks known file name forms and refuses a **65th pending proposal**. It reserves one total-entry slot for its temporary file to prevent in-progress publication from exceeding the reader limit.

It writes each bounded proposal to a random, private `0600`, `O_CREAT|O_EXCL|O_NOFOLLOW` temporary file, synchronizes it, and atomically renames it to `<opaque-id>.json` using Linux `renameat2(RENAME_NOREPLACE)`. Duplicate IDs **never replace** existing final files. A directory `fsync` follows. Failure after rename may leave a visible file (the result is deliberately treated as ambiguous and never claimed successful). The existing executor-side reader still independently checks owner, mode, one hardlink, exact bytes, expiration and capacity and **never treats a filename or hash as approval**.

A compromised/uncooperative process using the same monitor UID can still add, replace or rename files despite the advisory lock. The executor therefore **must reopen, rehash and revalidate** independently at preview and later approval. The publisher's quota is a cooperative availability limit, not a security authorization or a promise about arbitrary monitor-controlled contents.

## Safe retention and expiry cleanup

`CleanupExpired(now)` is **explicitly invoked**, not a timer or maintenance daemon. It scans the same bounded directory under the cooperating lock and only selects:

- Well-formed `<opaque-id>.json` entries whose strict v1 content validates using its canonical creation time, whose embedded ID matches the filename, and whose expiration is **at or before** the supplied current time.
- Recognized `.proposal-<32 lowercase hex>.tmp` files that are private regular single-link monitor-owned files and whose **both mtime and ctime** are older than 30 minutes. A manipulated old mtime alone cannot immediately discard an active temporary file.

The cleaner refuses unexpected filenames, symlinks, hardlinks, FIFOs, altered modes/owners, oversized/malformed records and unsafe directory ancestors. It records opened-file metadata and exact hashes, then reopens and compares them immediately before unlinking. It uses only `unlinkat` relative to the already validated inbox directory and `fsync` afterward. Failure is **fail-closed**; malformed or suspicious records require separate operator remediation rather than broad wildcard deletion. A concurrent malicious **same-UID** actor may still race a directory entry between checking and unlinking, but can only impact untrusted monitor-owned inbox data, not an executor-owned authority file. No `rm -rf`, `chdir` traversal, directory creation or external cleanup is performed.

Expiration is evaluated against the executor/monitor host clock supplied by the caller, not a monitor-provided "current time" field. The 15-minute maximum plan lifetime is enforced by the v1 parser. Cleanup never auto-applies, implicitly revokes a previous executor-approved operation or generates a privileged receipt.

## CI and remaining gates

The standard Go unit/race/vet suite plus a **root-owned disposable filesystem fixture** checks: same-ID no-overwrite; 16 KiB and 64-pending limits; expired-only removal; quota recovery; concurrent publishing; adversarial symlink/hardlink/FIFO/mode/invalid-content rejection; orphan temporary retention; forged old mtime; no accidental deletion outside the private directory; and root-EUID rejection by production monitor API.

A second CI fixture runs the publisher/cleanup **as a real nonroot UID (65534)** in an isolated root-owned subtree under `/opt` and independently reads the resulting proposal from a root process. This tests the ownership boundary but **does not** certify container bind mounts, host UID mapping or real VPS security.

**Not provided:** a monitoring GUI button or polling worker that calls these functions, actual inbox/monitor-UID provisioning, real executor installation, Docker/Compose/OCI baseline inspection, approved TTY deployment confirmation, durable journal, read-only reconcile, Docker writes, release tags or VPS operations. Those remain separately reviewed B6-02/B6-03/B6-04/B6-05 and later security/backup gates. The observed live `hello-go` source parent mode `0775` remains ineligible under the protected-source policy.

**Governance:** No merge, deployment, release or host changes without explicit operator approval. Successful CI does not confer permission to move the monitor or executor beyond the existing no-write architecture.
