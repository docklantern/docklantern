# Beta.6 B6-04 — root-private stable Compose source handoff

Following merged PR #118, the executor preflight now includes `StageTrustedSources` and `NormalizedComposeFromHandoff`. This is the first tested **byte-stable Compose input copy** for future guarded operations.

## Design

- Requires host root, a valid host-owned registration and a pre-existing trusted, root-private snapshots directory (`/var/lib/docklantern/executor/source-snapshots`). No untrusted CLI/monitor path controls production staging, and no parent directory is automatically provisioned.
- Records the exact registered ordered Compose and explicit interpolation env files, reads them using the existing nofollow, trusted-owner, bounded descriptor checks, and copies them into a newly created private directory with 0400 files and a 0500 directory.
- Fsyncs copy files and the stage directory; all file names are deterministic safe internal names, independent of attacker-controlled basenames.
- Verifies the original source fingerprint before and after capturing, and verifies the staged byte hashes again before read-only Compose config calls. The handle refuses use after `Close` and removes its own transient copies without modifying user-authored files.
- Uses the same restricted `docker compose config --format json` command with the original explicit `--project-directory`, but the verified staged `-f` and `--env-file` lists. Both v2 and pinned v5 integration tests compare the normalized baseline to the original and verify an image-only candidate diff. A change to an original env file invalidates the handoff rather than being silently ignored.

## Remaining fail-closed scope

This is a **preflight read-only handoff prototype**, not an authorized write path. Only the explicitly registered Compose and `--env-file` contents are copied. Nested `include`, `extends`, runtime `env_file`, `configs` or `secrets` references, external source trees, relative bind mounts, Docker image resolution and Compose's hidden filesystem reads require a separately reviewed strict feature/source allowlist; a root-only copy alone does not prove equivalent write behavior. Additional entries must either be snapshotted safely or refused.

Likewise, a private directory does not make the original authorized project immutable: a trusted administrator can still edit it while a command runs. Future B6-05 must hold the source snapshot for the complete targeted Compose subprocess, check input and runtime state again immediately before side effects, establish a supported Engine/API/Compose version row, take the project lock, commit durable journal intent **before** the first Docker write, and obtain a separate interactive APPLY confirmation. Following an ambiguous crash, manual reconciliation remains mandatory. No automatic recovery or rollback is introduced.

The fixture performs only read-only Compose operations on disposable host paths. No Docker pull, up, service replacement, journal mutation, VPS update, release or published image is included.
