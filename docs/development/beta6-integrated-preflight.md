# Beta.6 Phase 4 — integrated read-only qualification

This batch combines the protected registration and strictly parsed monitor proposal with the independently obtained, read-only evidence introduced in PRs #111 and #112. `executorpreflight.Qualify` requires:

- A valid, still-current proposal that matches a protected registration and its enabled single service.
- A registered-order trusted Compose/env source fingerprint, repeated **after** registry/Compose work.
- A fresh actual Docker container/image baseline, matching the monitoring hint as a **drift check only**, re-inspected after registry work.
- An exact whole-config Compose diff affecting only the selected service image, and repeat normalization to detect changes while reading other evidence.
- An immutable OCI index digest and selected linux/amd64 platform-manifest digest validated against actual bytes.

The result contains only limited sanitized identity evidence and `ReadOnly=true`. No mutation authorization, privileged web channel, raw Compose contents, environment values, registry credentials or Docker child output is returned. A mismatch, absent source, stale baseline, OCI failure, unsupported service, config change or expired proposal fails closed.

## Production adapter still required

The `ReadOnlyEvidence` interface is intentionally not yet implemented against a real Docker host or live registry. Its CI fixtures provide adversarial orchestration coverage, **not** live Engine/Compose or public registry qualification. In particular, the host adapter must use vetted absolute binaries and sanitized env, stable trusted source handoff, and independently authenticated registry retrieval, then repeat actual state checks immediately before a separately TTY-authorized mutation. Do not call `Qualify` a deployment-approval function or use a fake implementation for actual execution.

The design can be exercised alongside parallel B6-06 GUI work. Issue #84 remains open until the real read-only adapters and guarded Compose fixtures pass. This change does not enable Docker writes, install the executor, modify the VPS or grant merge/release authorization.
