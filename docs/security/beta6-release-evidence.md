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
| SEC-15 | Native verify, running-only/health and journal commit tests | **Witnessed on disposable VPS 2026-11-07:** target replacement, running-only acceptance, journal COMMIT/succeeded and sibling IDs; not a substitute for healthchecked or production-specific tests |
| SEC-16 | Trusted source snapshot/recheck and handoff fixtures | Review source owners and replacement protections on target |

## Remaining beta.6 limitations / blockers

1. The first host-native apply path is deliberately limited to a single registered public Docker Hub `linux/amd64` service and pinned Engine 29.8.1 / API 1.56 / Compose v5.5.1. Hosted disposable Docker integration is not equivalent to the exact VPS runtime.
2. **Host-local `recover` is not implemented.** Interrupted operations remain blocked for independent manual reconciliation/repair; there is no automatic rollback promise. See [B6-05 acceptance](../development/beta6-host-apply-acceptance.md).
3. The executor records desired image generations in its protected SQLite journal, not a durable change to user-authored Compose YAML. A later unmanaged `docker compose up` can restore an old image.
4. The monitor-side proposal publisher remains unwired to the GUI; GUI evidence and Activity are read-only, not permission to update.
5. The operator separately authorized and witnessed the **specific disposable-VPS** executable installation and one immutable hello-service apply on 2026-11-07. The operator explicitly waived off-host backup custody and restore testing **for that disposable test only**. The waiver does not satisfy the off-host/restore criterion for production or authorize another target, apply, or VPS mutation.
6. **G7 tag/publication remains NOT AUTHORIZED.** The audited source/CI commit was `f26d2c9`; the release-preparation documentation merge will create a **different exact release head**. Recheck its CI/security evidence and release eligibility, review unfulfilled SEC/B6-08 criteria and accepted beta-only deferrals, and obtain separate explicit authorization before a tag triggers publication.

## Dated disposable-VPS acceptance evidence (2026-11-07)

- **Source and test CI:** `f26d2c9aaf62494700433a77e74e02a64c1a4621` (PR #133); [Go POC 17/17 jobs](https://github.com/docklantern/docklantern/actions/runs/41907889810) and [security audit 4/4 jobs](https://github.com/docklantern/docklantern/actions/runs/41907889788) succeeded at that source commit. CI jobs do **not** independently certify every SEC-01–SEC-16 host acceptance scenario.
- **Executor artifact and runtime:** the operator reported installed `/usr/local/libexec/docklantern-executor`, SHA-256 `1bd2a8afa155e87306af3849a346fd4d6b4bf1fd864ba7e5413b3e2cd81657bb`; Linux/amd64 VPS, Engine 29.8.1/API 1.56/Compose 5.5.1. Protected registration, private proposal writer and source snapshots were provisioned.
- **Positive executor verification:** independently qualified candidate OCI index `sha256:2784c9e97340a33997905ad651c034fe11dba9f3a1ab8a2218ccffc093c1a603`, explicit host-local `APPLY`, new running disposable `hello-go-beta6/web` container, and `status apply-be7c0588e024e4648dee3885` reporting `COMMIT / succeeded / Blocks new work: false`. Six original siblings remained unchanged. The fixture had no HEALTHCHECK; the operator expressly accepted running-only verification.
- **Monitoring candidate acceptance:** local build from the same source, not the CI/published image, ran `v0.1.0-beta.6-rc.f26d2c9` healthy on the VPS with unchanged sibling container IDs. Seven browser GUI checks passed, including `/deployments` showing **unknown/unavailable** without a read-only receipt mount. The positive receipt-outbox-to-Activity path was **not** tested.
- **Recovery limitation:** monitoring SQLite backup existed locally and passed schema-v5 integrity and SHA-256 verification, but it was **not copied off-host or restored**. No executor journal/override independent restore drill, interrupted live apply, or manual recovery was exercised. SEC-13 and broader production recovery qualification remain open.

See the exact digests, container IDs, timestamps, and checksums in the [B6-08 witnessed acceptance record](../deployment/beta6-test-vps-acceptance.md). This update records partial acceptance, not unconditional closure of issue #88, production readiness, or independent G7 authorization.

No new tag, release, PR merge, unrelated install, update attempt or deployment is authorized by this record.
