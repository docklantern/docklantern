# Beta.6 B6-04: bounded structural Compose source validation

PR #122 introduced lexical rejection of common hidden inputs before Docker Compose normalization. This change adds a **bounded structural grammar** with explicit mapping/sequence nesting, two-space indentation and duplicate-key rejection. It executes on the original descriptor-verified bytes and again against the root-private source handoff, before every staged qualification.

The initial beta.6 accepted YAML subset allows simple ASCII mapping keys, indented mappings and scalar sequences, plain or simple quoted scalars, and registered explicit Compose/env files only. It rejects duplicate keys at every level, ambiguous mixed mapping/sequence constructs, skipped or odd indentation, quoted/escaped mapping keys, unquoted inline mapping delimiters and trailing comments. The previously implemented denial of include, extends, env_file, build, aliases, tags, flow/block constructs, secrets and untrusted filesystem references remains in effect. Effective-config checks still restrict the entire project to prebuilt single-service image changes on external networks and independently verified OCI/Docker state.

Tests cover duplicate target image keys, duplicate services/root/network fields, indentation trickery, mapping syntax inside scalars, trailing comments and simple supported Compose examples. Root-protected and real Compose v2/v5.5.1 regression suites run unchanged. **A supported Compose config is narrower than normal Compose YAML.** If a project uses richer syntax, qualification should fail closed rather than guess how it will be parsed.

## Limitations and release gates

This is a purpose-built subset scanner, **not** a third-party or formally complete YAML 1.2 parser. It assumes unsupported YAML forms are rejected by the combined lexical and structure gates. A final security review should specifically evaluate alternate YAML encodings, duplicate keys, trailing comments, scalar escaping, multiple registered Compose files and interpolation source provenance before calling B6-04 complete.

It is still strictly read-only: no Docker write API, `pull`, `compose up`, deployment authorization, automatic recovery, VPS installation or release. B6-05 requires separate explicit approval, exact Engine/API/Compose compatibility, durable journal intent, project locks, independent TTY confirmation and guarded disposable-host apply/verification/crash testing.
