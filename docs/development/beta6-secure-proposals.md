# Beta.6 Phase 2b — Secure, untrusted deployment proposals

**Status:** CI-only implementation for review under [B6-02 (#82)](https://github.com/docklantern/docklantern/issues/82). This is **not deployment authorization** and does not enable the executor's `apply`, `preview`, `register` or `recover` commands.

## Authority boundary

The monitoring process may write bounded untrusted proposals; it may **not** register projects for deployment, authorize execution, or issue Docker commands. An executable proposal must ultimately be checked against a separately protected host-owned registration, independently inspected OCI/platform and Docker state, and an interactive local operator approval. The `observed` fields are hints, never authoritative state.

The new `internal/executorproposal` library provides `Parse(data, executorClock)`, `MatchesRegistration(protectedRegistration)`, and `LoadInbox(proposalID, trustedMonitorUID, executorClock)`. There is no web endpoint, user-supplied source path, command launcher, publisher, inbox cleanup job or Docker write capability.

## Proposal v1 format

The JSON format follows [the deployment contract](../architecture/beta6-deployment-contract.md). Example *placeholder* IDs and digests are not deployable:

```json
{
  "schema_version": 1,
  "proposal_id": "plan-example-01",
  "created_at": "2026-11-02T19:59:00Z",
  "expires_at": "2026-11-02T20:10:00Z",
  "registration_id": "hello-go-demo",
  "service": "web",
  "candidate": {
    "repository": "docker.io/example/hello-go",
    "requested_tag": "latest",
    "digest": "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
    "platform": {"os": "linux", "architecture": "amd64"}
  },
  "observed": {
    "container_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "image_id": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
  }
}
```

Parser constraints:

- Maximum **16 KiB** of JSON, strict v1 schema including exact nested object names, no duplicate keys (including Unicode-escaped aliases), unknown fields, type coercion or trailing bytes.
- Safe opaque lowercase ASCII proposal/registration/service IDs; no host paths, arbitrary Docker options, environment variables, commands or credential fields.
- Initial approved syntax is a canonical public Docker Hub namespace/repository, Docker tag (for explanatory display only), lowercase immutable `sha256:<64 hex>` manifest-digest candidate, exact `linux/amd64` platform, and full observed container/image IDs. Digest syntax alone is **not** OCI verification.
- Both timestamps must be canonical UTC RFC3339 `Z`, with a **15-minute maximum validity interval**, future creation at most 60 seconds ahead of executor time, and expiry strictly after current executor time. No trust in monitor clock.
- Fingerprint is SHA-256 of the **exact JSON bytes**, not a normalized reserialization. Changing any field, whitespace, or byte changes its fingerprint.
- `MatchesRegistration` additionally requires an exact protected registration ID and deployment-enabled service/repository/platform match. A match **does not** authorize deployment.

## Protected inbox read

Production reads use the fixed path `/var/lib/docklantern/handoff/proposals/<opaque-id>.json`; the library never accepts a caller-controlled host path. The **nonroot monitor UID** must be supplied by independently protected host configuration (not the proposal or web app). The directory layout is expected to have root-owned non-group/world-writable ancestors and a private `0700`, monitor-owned final proposals directory.

The Linux reader walks path components with descriptor-relative `openat` and `O_NOFOLLOW`, verifying opened directory ownership and modes using `fstat`. It bounds enumeration to **128 total entries** (including temporary files) and refuses more than **64 pending `.json` proposals**; it does not silently evict any plan. Filenames ending in `.json` must contain valid opaque IDs. It opens only the selected name with `O_NOFOLLOW | O_NONBLOCK`, requires a monitor-owned private regular file with **one hard link**, reads at most 16 KiB, and compares `fstat` metadata (including inode, timestamps, size and permissions) before/after reading. It requires the embedded proposal ID to match the selected filename.

**Limitation:** A compromised monitor can mutate or replace its own proposal *after* this read. Descriptor-relative opening and metadata rechecks protect the snapshot, but cannot turn an untrusted proposal into authority. **Phase 2c must reopen and compare the exact content hash immediately before confirmation and after fresh registration/actual-state checks.** A stable hash alone is not operator permission. The read-only inbox does not create, delete, publish, quarantine or garbage-collect files; bounded safe publisher/retention remains a separately reviewed Phase 6 interface.

The existing beta.5 monitor has not been given this directory or Docker write authority; the actual nonroot UID and isolated mount configuration require security acceptance before real-host deployment.

## Tests

Go unit/fuzz seeds cover unknown/duplicate fields, escaped keys, malformed and expired proposals, incorrect registry/tag/digest/platform, stale observed hints, registration bypass attempts and SHA-256 consistency. A separate root-owned **disposable** CI fixture uses a nonroot proposal-file owner, testing private file reads, replacement fingerprints, symlinks, hardlinks, FIFOs, unsafe ownership/modes, stale proposals, oversized data, invalid IDs and over-quota inbox contents. The runner never accesses the production path, Docker socket or VPS.

**Deferred gates:** Phase 2c independent TTY confirmation and rehash, Phase 3 durable journal, Phase 4 stable Compose/OCI/current state, Phase 5 separately approved disposable Docker mutation, and Phase 6 actual monitor bind mounts, publishing/cleanup, full security audit and off-host backup/restore. The current hello-go source parent remains group-writable (`0775`) and ineligible for real-host executor registration.

**Governance:** This code may be reviewed as a separate PR. Do not merge, publish a release, install a binary, change VPS permissions or execute Docker updates without the user’s separate explicit approval.

## Follow-up Phase 2 publication/retention library (review proposed)

A separately reviewed [proposal publication and retention slice](beta6-proposal-publication.md) adds monitor-side, fixed-inbox `PublishProposal` and `CleanupExpired` primitives with atomic no-overwrite files, capacity checks and guarded expiration cleanup. The original Phase 2b reader described above remains independently authoritative **only about syntax and untrusted-file safety**, not Docker approval. **These new primitives are not connected to the monitoring web app or scheduler, and no production bind-mount/UID is configured by the PR.** Read-only monitor architecture and all executor write protections remain unchanged.
