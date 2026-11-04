# Beta.6 B6-04 — raw Compose provenance checks before normalization

The existing normalized Compose feature allowlist is necessary but does not preserve the provenance of every original YAML construct. In particular, Compose may resolve nested `include`, `extends`, service `env_file`, anchors or YAML tags and then present a deceptively ordinary image-only normalized JSON object.

This read-only batch adds `ValidateRawComposeSource` to the **descriptor-verified registered Compose bytes** in `CaptureTrustedSources`, and again to both original and staged Compose files in the root-private `SourceHandoff`. It conservatively denies a narrow group of source directives: includes, extends, service env_file, build, configs, secrets, hooks, YAML aliases/anchors/tags, explicit complex keys, flow maps/sequences, multi-document markers and block scalars. It rejects non-UTF8, NUL, CRLF and tabbed source syntax. The tool returns only generic sanitized errors, never source content.

CI adds adversarial raw input fixtures and a root-only real Compose regression that tries to introduce a raw include after staging. The trusted source capture and staged normalization must both deny the changed bytes. Existing v2/v5.5.1 read-only parity tests remain active.

## Compatibility and security limits

This intentionally restrictive lexical policy is **not a formally complete YAML parser** and should not be treated as proof against all possible equivalent YAML encodings or implicit source reads. It may reject otherwise valid Compose projects; that is acceptable for the first guarded beta.6 subset. For final B6-04 sign-off, either adopt a reviewed structural source parser with key/alias/duplicate-key validation and a bounded accepted YAML grammar, or prove a complete equivalent provenance boundary. Preserve the existing explicit project/env and external-network policy, stable root-private source handoff and exact supported Compose v5.5.1 qualification.

The first B6-05 Docker write is still blocked on independent approval, exact Engine/API/Compose write compatibility, durable journal intent, project locking, fresh terminal confirmation, guarded disposable service replacement and failure/crash testing. No new code here can pull, start, stop or recreate Docker containers; no VPS actions, merge or release are authorized.
