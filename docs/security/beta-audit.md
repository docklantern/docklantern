# Beta security audit — PR #22

## Scope and release decision

This is a **repeatable, blocking security gate** ahead of `v0.1.0-beta.1`. Three independent jobs scan (1) Go application call paths via `govulncheck`, (2) the **built** monitoring application image, including its runtime Alpine packages, via Trivy, and (3) the optional `tecnativa/docker-socket-proxy` image from the Compose deployment overlay.

The scanners consume contemporary public vulnerability feeds during GitHub Actions runs. A clean result means *no matching known, actionable findings within those scans at that moment*, **not** that DockLantern is secure or free of unknown vulnerabilities. Re-run immediately before a release; pin the resulting GHCR image digest after publishing.

## Automated gates

- `go-vulnerabilities`: fails if `govulncheck ./...` reports a detected reachable vulnerability. The tool prioritizes reachable Go call paths and is not an exhaustive audit of every transitive dependency.
- `monitoring-image`: builds the pinned-base Dockerfile locally and fails for **fixable** HIGH/CRITICAL OS or library vulnerabilities. Unfixed findings remain outside the blocking policy and must still be reviewed.
- `docker-proxy-image`: scans the **optional, privileged** socket-proxy image configured in `deploy/traefik/compose.discovery.yaml`, and applies the same HIGH/CRITICAL fixable-vulnerability gate.
- `security-audit.yml` runs on dependency, Dockerfile, proxy-overlay or workflow changes, on main, and as a reusable workflow required by version-tag publishing.
- The tagged release workflow does not publish the GHCR image unless these security jobs pass, in addition to existing tests and disposable installation verification.

**Policy:** No automatic suppression of known findings, no externally sourced image deployment, and no release publishing if a blocking job is red. If remediation is impossible, document the CVE, package/version, exploitability reasoning and time-bound release exception in a separate reviewed PR. The scanner version and advisory database change over time, so subsequent scans may disagree with earlier ones.

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
