# Beta.6 B6-04 — pinned Compose v5.5.1 read-only compatibility

This fixture validates the exact **Docker Compose v5.5.1** CLI version observed on the test VPS, not merely the v2 plugin installed on the default GitHub Actions runner. It is an additional, independent `executor-compose-v5-readonly-contract` job in Go POC.

The CI job downloads Docker's official Linux/x86-64 release artifact from the fixed `v5.5.1` release and **verifies its published binary SHA-256** (`db1889184726840f75c4f9c001048430d4f25b3be3cb084d3ddd762bc0aed576`) before installing it into an isolated root-only Docker CLI configuration. It requires `docker compose version --short` to report exactly `5.5.1`. Unlike installing the plugin system-wide, the isolation preserves the existing v2.38.2 CI compatibility fixture.

Using the trusted host adapter from PR #114, the root-only fixture exercises real Compose v5 configuration normalization with registered root-private sources and env interpolation, a sibling service, external network, Traefik-style labels, `pull_policy: always`, and a candidate image-only stdin overlay. It verifies that only the target service image changes and source fingerprint rechecking rejects modified env bytes. No service is started, pulled or updated; all test sources are disposable.

## What this does not establish

- This is a **read-only Compose config** test, not full test-VPS Docker Engine 29.8.1 / API 1.56 write compatibility. GitHub's runner may have a different Engine version.
- The production apply command still needs a **stable trusted Compose source handoff**, independently approved terminal confirmation, project lock and durable journal intent, targeted apply and health verification, crash tests and an explicitly authorized disposable-host Docker write test.
- No runtime matrix is approved for Docker writes by passing this fixture; B6-04 and B6-05 stay open.
- All VPS operation, off-host backup/restore, deployment and release approvals remain independent.

This test only uses fixed public release bytes and root-private disposable CI paths. It does not modify a running DockLantern installation or the test VPS.
