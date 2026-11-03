# Beta.6 Phase 2 — Host-local protected `register` CLI integration

**Status:** Proposed for review under [B6-02 (#82)](https://github.com/docklantern/docklantern/issues/82). **No executor has been installed or invoked on a VPS.** This is an explicitly operator-invoked **host-registration-file management** command; there are no Docker update commands.

## Supported CLI contract

```text
docklantern-executor register --file /root/docklantern/registration.json
docklantern-executor register --file /root/docklantern/registration.json \
  --expect-sha256 sha256:<64-lowercase-hex-of-existing-protected-registration>
```

- **Create:** `--expect-sha256` absent. Fails if the protected record already exists.
- **Update:** `--expect-sha256` required and must match the existing protected registration's *exact* record hash. Rejects blind overwrite and stale hashes.
- The CLI strictly requires `--file` and only the optional `--expect-sha256` pair. Duplicate/unknown flags, implicit `--yes`, stdin (`-`), relative/unclean/control-character paths, remote Docker options, unexpected arguments, and malformed hashes fail **before** a host action is attempted.
- No shell, Docker CLI subprocess, monitoring server integration, web route, or arbitrary Docker command is exposed. The `preview`, `apply`, `recover`, `status`, and `reconcile` CLI commands **remain unavailable**.

## Host security preconditions

**Only an authorized host administrator operating as root on a real controlling `/dev/tty` may register projects**. The input must already exist at a *canonical absolute path* with root-owned, non-group/world-writable, nonsymlink directory ancestors, and must be a root-owned, singly linked, non-executable **0600** regular file (size 1 to 16 KiB). The monitoring inbox and its nonroot files cannot qualify. The new reader walks root-owned directory components using `openat(O_NOFOLLOW)`, checks descriptor metadata before and after a bounded read, and uses the existing strict registration-v1 JSON parser. It never accepts host paths from a monitor proposal.

The existing protected publisher requires root and controlling TTY; it shows the project, ordered Compose/env sources, registered services, repositories, platforms, readiness policy, and the new registration content SHA-256. The operator must type the explicit `REGISTER <registration-id> <challenge>` response. After typing, the command **reopens and rehashes the original root-owned input bytes**, checks the source ownership policy again, and publishes only to the fixed executor-controlled `/var/lib/docklantern/executor/registrations/` directory. No registration store directories are created automatically.

The file publication uses an exclusive temporary file (0600), file and directory fsync, directory lock and atomic rename with compare-previous-hash semantics. Success reports the persisted registration identifier and hash; failures are generic, without raw JSON, secrets or file paths.

## CI evidence and explicit limits

CLI unit tests inject a fake host-only registration action to verify unsafe flag combinations cannot reach privileged filesystem code, confirm create/update arguments, check sanitized errors and maintain disabled deployment commands. A disposable **root-owned CI fixture** verifies real input-file ownership/modes, symlinked components, FIFO, hardlinks, traversal, oversized/duplicate-key JSON, changed-byte fingerprint, and noninteractive rejection. These tests run on isolated runners, not the test VPS.

**No runtime installation or host provisioning is authorized by this PR.** The current VPS's `hello-go` project parent mode `0775` still fails the strict source rule, so this PR does not make that project eligible. Input validation and Phase 2 registration checks are **not** stable Compose source snapshots against trusted root/admin races. A protected registration does not grant execution capability without Phase 3 durable journaling, Phase 4 exact-version Docker/Compose plus OCI preflight, Phase 5 separately approved Docker mutation tests, and Phase 6/8 host security, mount segregation, backup and restore gates.

Do not merge this PR, publish an executor binary/release, modify real host permissions, create real registrations, or perform Docker writes without the user's separate explicit approval.
