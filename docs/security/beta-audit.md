# Beta security audit — PR #22

## Scope and release decision

This is a **repeatable, blocking security gate** ahead of `v0.1.0-beta.1`. Three independent jobs scan (1) Go application call paths via `govulncheck`, (2) the **built** monitoring application image, including its runtime Alpine packages, via Trivy, and (3) the optional `tecnativa/docker-socket-proxy` image from the Compose deployment overlay.

The scanners consume contemporary public vulnerability feeds during GitHub Actions runs. A clean result means *no matching known, actionable findings within those scans at that moment*, **not** that DockLantern is secure or free of unknown vulnerabilities. Re-run immediately before a release; pin the resulting GHCR image digest after publishing.

## Automated gates

- `go-vulnerabilities`: fails if `govulncheck ./...` reports a detected reachable vulnerability. The tool prioritizes reachable Go call paths and is not an exhaustive audit of every transitive dependency. It runs inside the **exact pinned Go builder image** used by the Dockerfile, not the older language-version directive.
- `monitoring-image`: builds the pinned-base Dockerfile locally and fails for **fixable** HIGH/CRITICAL OS or library vulnerabilities. Unfixed findings remain outside the blocking policy and must still be reviewed.
- `docker-proxy-image`: scans the **optional, privileged** socket-proxy image configured in `deploy/traefik/compose.discovery.yaml`, and applies the same HIGH/CRITICAL fixable-vulnerability gate.
- `security-audit.yml` runs on dependency, Dockerfile, proxy-overlay or workflow changes, on main, and as a reusable workflow required by version-tag publishing.
- The tagged release workflow does not publish the GHCR image unless these security jobs pass, in addition to existing tests and disposable installation verification.

**Policy:** No automatic suppression of known findings, no externally sourced image deployment, and no release publishing if a blocking job is red. If remediation is impossible, document the CVE, package/version, exploitability reasoning and time-bound release exception in a separate reviewed PR. The scanner version and advisory database change over time, so subsequent scans may disagree with earlier ones.

## Audit results — October 23, 2026

**Original PR #22 release recommendation: NO-GO pending remediation.** The first security workflow run [#40008555155](https://github.com/docklantern/docklantern/actions/runs/40008555155) found blocking known vulnerabilities. The failures are findings, not CI infrastructure breakages; **do not suppress or bypass these gates**.

| Surface | Findings in first run | Decision |
|---|---|---|
| Go source check using the module's declared Go 1.23 toolchain | **Corrected pinned-builder scan: PASS.** The original superseded scan using Go 1.23.12 had reported 44 standard-library vulnerability advisories; those did not represent the builder in the shipping image. | **Source check passed** with the Dockerfile’s digest-pinned Go builder in [updated security run #40009129971](https://github.com/docklantern/docklantern/actions/runs/40009129971). This does not clear other binaries shipped in the runtime image; review the container scan separately. |
| Built monitoring image | 40 OS package findings (38 HIGH, 2 CRITICAL). The bundled `docker` Go binary showed 26 findings; `docker-buildx` 63 and `docker-compose` 64. The compiled `docklantern-inventory` binary showed **zero Trivy findings** in this scan. Counts may overlap across packages/artifacts. | **Blocked.** Move away from the stale Docker 28 CLI image or upgrade/rebuild runtime packages; consider shipping a minimal default runtime and keeping the CLI in an explicit operator-only variant, preserving any required local discovery capability. |
| Optional Docker socket proxy v0.5.0 | 6 fixable HIGH Alpine findings, affecting OpenSSL and PCRE2 libraries | **Blocked.** Rebuild on up-to-date patched packages (or change to a verified maintained alternative) and rerun the proxy mutation-denial/inventory integration tests. No upstream v0.5.1 release was identified during review. |

**Scope nuance:** The *first* Go source job inadvertently scanned a Go 1.23 toolchain selected from the module directive. A follow-up commit corrected the security workflow to scan from the **digest-pinned Go builder**, matching the release compilation environment; that job **passed**. The Docker CLI runtime still bundles binaries built with different Go versions, and its Trivy findings remain independent blockers. These are distinct scan targets. The first report should **not** be interpreted as 44 confirmed vulnerabilities in the shipping DockLantern application binary. Use the toolchain/version and binary evidence for each artifact during remediation.

The original PR #22 scanner exit codes were intentionally nonzero. **PR #23** reduces runtime attack surface by removing Docker/Compose/Buildx binaries, upgrades Alpine packages, pins the runtime base, and rebuilds the optional proxy with patched Alpine packages. A green rerun of all three scanner jobs is required; this document retains the original audit evidence for traceability.

The scanner exit codes are intentionally nonzero. Green functional CI cannot override these security gates, and **no `v0.1.0-beta.1` tag should be created** until the findings have been assessed and addressed. The initial report is not a full manual penetration test or audit of the Compose/TLS/network trust model.

### Proposed remediation order

1. Continue scanning Go with the **pinned builder** (passing in corrected run), and update to patched supported toolchains whenever required by new advisories. Rescan the actual binary and source on each builder change; preserve Compose SDK compatibility.
2. Reduce the main runtime image's Alpine/CLI footprint or upgrade it; check whether bundling the Docker Compose and Buildx CLI plugins is necessary for the monitoring-only service.
3. Replace or rebuild the optional privileged Docker socket proxy with patched Alpine packages and verify **POST rejection** and private-network isolation against a disposable Docker daemon.
4. Rerun all three audit jobs, existing Go tests, deployment package, Docker discovery proxy, and full monitoring flow. Review **unfixed** findings manually and record a release approval decision.
5. Only after a green scan, reviewed vulnerability exceptions if any, and explicit operator approval, proceed with creating a release tag and publishing GHCR.

### Evidence links

- [Go source scan](https://github.com/docklantern/docklantern/actions/runs/40008555155)
- [Built container image and optional socket proxy Trivy scans](https://github.com/docklantern/docklantern/actions/runs/40008555155)
- [Original beta packaging/security regression suite](https://github.com/docklantern/docklantern/actions/runs/40008555014)

## Human review before tagging

1. Review Trivy scan output **including unfixed findings**; scan defaults suppress them from the blocking view. Consider re-running `trivy image --ignore-unfixed=false --severity HIGH,CRITICAL ...` in a disposable audit environment for a fuller list.
2. Review HTTP Basic auth exposed only through trusted TLS Traefik, authenticated admin routes, CSRF guard, root-owned secret files, untrusted networks, and the public readiness endpoints. Scanner success does not validate the security design.
3. Assess the socket proxy's privilege boundary. `POST=0` and private networking help but the proxy itself retains host Docker socket access. The default deployment does not mount it, and permission to enable the optional proxy should be a separate operator decision.
4. Check Go build and runtime OS dependencies, Docker Engine CLI version, GHCR/registry credentials and installed GitHub Actions. Review tag protection and approval rules.
5. Perform a disposable-host installation and backup/restore verification. Maintain **monitoring-only** status; no update execution.
6. Record the date, CI workflow run URLs, vulnerabilities and remediations, auditor, and an explicit publish/no-publish decision. Never assert a completed audit from CI green alone.

## Additional controls left for later

- Dedicated code review / threat modeling of webhook SSRF, registry auth redirects, Basic auth, and Docker socket proxy trust
- Container artifact signature verification and external provenance attestation policy
- Dependencies and GitHub Actions pinned to reviewed commit SHAs
- A formal SBOM + advisory monitoring over released immutable digest
- Rootless deployment and secret ownership testing, external security monitoring, coordinated SQLite instance safety

**Nothing in this PR creates a release tag, publishes an image or deploys to production.**
