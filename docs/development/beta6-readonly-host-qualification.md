# Beta.6 integrated read-only host qualification

After merged PR #116, the host-native executor now exposes:

```text
docklantern-executor qualify <proposal-id>
```

This is a **root-only, controlling-TTY-only, read-only** operation. It uses the protected monitor UID, validated monitor proposal inbox, protected executor-owned registration and the real read-only host adapter. The adapter checks trusted Compose/env sources and their fingerprints, the exact image-only config difference, independent Docker actual container/image identity, and anonymous Docker Hub OCI index/platform proof. All relevant evidence is rechecked after potentially slow registry operations.

The outer coordinator also **reopens the proposal and registration after the full read-only preflight** and rejects any change to their exact protected-byte SHA-256 values or proposal expiry. CLI output is sanitized: only proposal/registration identifiers, service, digests, source hash and actual image/container identity; no source file contents, environment values, registry tokens, raw child logs or paths.

A positive result is explicitly labeled `ReadOnly=true`, **not** a capability or an authorization token. `apply` and `recover` remain unimplemented and disabled; no Compose pull/up, Docker API write, journal advance, override promotion or local deployment authorization is possible from this new command.

## Required before B6-05 Docker writes

- Proven stable Compose source snapshot used across the eventual **write** subprocess, not just pre/post read-only hash checks.
- Exact independently tested Linux Docker Engine / API / Compose plugin version row (the CI v2.38.2 test does not qualify VPS v5.5.1).
- Project-level lock, durable journal pre-side-effect intent, separate fresh host-local **APPLY** confirmation and guarded disposable Docker write tests with target/sibling verification.
- Recovery and restart ambiguity remain fail-closed; no automatic rollback.
- Verified off-host backup plus witnessed restore and separate operator approval before any test-VPS executor write.

This batch intentionally advances the release critical path without assuming issue #84 or the explicit B6-05 Docker-write approval gate has been satisfied. No PR merge, release, executor installation or VPS change occurs here.
