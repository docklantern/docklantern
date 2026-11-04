# beta.6 B6-07 — executor security evidence and release hold points

This record describes the security evidence generated on **the exact reviewed CI commit**. It is not a sign-off for deploying or publishing beta.6.

## Evidence produced by CI

The existing [Beta security audit](../../.github/workflows/security-audit.yaml) has four independently failing jobs: source vulnerability analysis, monitoring-image/privilege scan, Docker-proxy image scan and standalone executor binary scan.

The executor job builds `linux/amd64` with `CGO_ENABLED=0` using the digest-pinned Go Docker builder, runs `govulncheck -mode=binary` on the resulting executable, records Go build metadata, and now uploads that **same scanned file** as a short-lived workflow artifact, accompanied by:

- `executor.sha256` — SHA-256 of the exact scanned executable; check it with `sha256sum -c executor.sha256`
- `source.txt` — the GitHub source commit, ref, target platform, builder image reference, and an explicit `published=false` marker
- `go-buildinfo.txt` — the executable's Go build information
- `docklantern-executor` — the exact ephemeral binary tested by the security workflow

**CI artifact is not a release artifact.** The action retains the evidence for seven days. A release/installation must use a separately reviewed binary whose hash is verified against approved evidence for the final commit, or rebuild and repeat the audit. No automated installation or publication is added here. Do not infer the unstripped binary's release suitability solely from a successful audit.

## B6-07 threat-model verification matrix

The entries below identify *test locations*, not blanket PASS claims. Review the jobs and their exact commit head before changing a status to passed.

| Security ID | Existing verification mechanism / evidence | Additional release gate |
| --- | --- | --- |
| SEC-01 | Host boundary, executor CLI/registration and read-only GUI tests | Confirm no monitor-side executor launcher or elevated socket access in final image |
| SEC-02 | Protected proposal inbox and strict parser fixtures | Inspect hostile path/rename fixture output |
| SEC-03 | Executor host-boundary and protected registration/journal root fixtures | Verify UID, mounts, and ownership on proposed VPS layout |
| SEC-04 | Read-only Compose normalization/allowlist and native adapter tests | Verify exact Compose v5.5.1 compatibility on target |
| SEC-05 | Docker CLI allowlist/environment tests and pinned plugin fixtures | Verify trusted Docker executable/daemon context on target |
| SEC-06 | Immutable OCI candidate and platform qualification tests | Check digest and public-registry availability on IPv6-only VPS |
| SEC-07 | Source handoff/requalification and drift tests | Verify target still matches just before operator approval |
| SEC-08 | Cross-process lock, blocked journal and coordinator fault tests | Confirm no uncoordinated external Compose writer |
| SEC-09 | Journal phase/crash and failure injection tests | Review interrupted-operation/manual inspection runbook |
| SEC-10 | Strict receipt parser, bounded logs and GUI response tests | Review redaction output on an intentionally secret-bearing disposable fixture |
| SEC-11 | Operator immutable digest review and Docker Hub repository restriction | A digest is **not** image publisher safety/certification |
| SEC-12 | Proposal quotas and bounded output/time tests | Confirm available disk/retention limits on host |
| SEC-13 | Protected journal restore/quarantine and source-generation fixtures | **Blocking:** off-host backup custody and a completed actual restore drill |
| SEC-14 | GET-only proxy and disposable packaging/onboarding integration | Verify production mount isolation and no published proxy ports |
| SEC-15 | Native verify, running-only/health and journal commit tests | **Blocking:** real-host verified outcome, including no-HEALTHCHECK semantics |
| SEC-16 | Trusted source snapshot/recheck and handoff fixtures | Review source owners and replacement protections on target |

## Remaining beta.6 limitations / blockers

1. The first host-native apply path is deliberately limited to a single registered public Docker Hub `linux/amd64` service and pinned Engine 29.8.1 / API 1.56 / Compose v5.5.1. Hosted disposable Docker integration is not equivalent to the exact VPS runtime.
2. **Host-local `recover` is not implemented.** Interrupted operations remain blocked for independent manual reconciliation/repair; there is no automatic rollback promise. See [B6-05 acceptance](../development/beta6-host-apply-acceptance.md).
3. The executor records desired image generations in its protected SQLite journal, not a durable change to user-authored Compose YAML. A later unmanaged `docker compose up` can restore an old image.
4. The monitor-side proposal publisher remains unwired to the GUI; GUI evidence and Activity are read-only, not permission to update.
5. Before any real-host executor installation or Compose mutation, separately approve the VPS baseline, **off-host backups and successful restoration drill**, exact binary digest, and exact target candidate/service. Test each step only after authorization.
6. **Release remains NO-GO** pending review of all SEC-01–SEC-16 outcomes, CI evidence at the final commit, and B6-08 operator acceptance. A tag can automatically publish images and must never be pushed merely because this audit passed.

No tag, release, install, shared-host mutation or merge is authorized by this document.
