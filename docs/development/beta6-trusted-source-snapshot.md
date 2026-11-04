# Beta.6 Phase 4 — trusted Compose source fingerprint and recheck

This batch follows merged PR #111 and adds a read-only, bounded, descriptor-relative capture of approved Compose and environment file bytes.

`CaptureTrustedSources(reg)` requires a validated protected executor registration and root-owned execution context. It walks the registered project and source paths with `openat` and `O_NOFOLLOW`, verifies root or explicitly trusted owner, rejects group/world-writable components, symlinks, hardlinks, nonregular files and unsafe source sizes, then hashes exact bytes in registration order. It returns only a hash and file count; it never serializes source or env contents to the monitor. `RecheckTrustedSources` fails closed if any source changes.

The disposable root CI fixture covers successful capture/recheck, changed env, order sensitivity, symlink, hardlink, writable ancestor/source, FIFO, missing/oversized input and invalid registration.

## Non-authorization boundary

This is *not* a complete stable Compose source handoff. It does not pin source file descriptors across the future Compose command; a trusted host operator may still change filenames or content after a recheck. A future read-only preflight must combine a trusted stable source invocation, exact Compose CLI compatibility, OCI registry evidence and fresh actual Docker state. No Docker command, container change, privileged executor launch, restoration, release, VPS provisioning or automatic updates are added by this PR.

A source fingerprint is evidence, not deployment permission. Phase 4 issue #84 remains open. B6-05 applies only after the independently reviewed combined preflight and local TTY approval.
