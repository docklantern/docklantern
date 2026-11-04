# Beta.6 B6-04 — real Compose read-only runtime contract

Following merged PR #115, this change adds a **real Docker Compose CLI** read-only integration fixture rather than relying exclusively on mocked subprocesses. The protected-journal fixture remains separate.

On an isolated GitHub Actions Linux host, under root and a mandatory `DOCKLANTERN_PREFLIGHT_COMPOSE_FIXTURE=1` guard, the test creates disposable root-private Compose/env files, validates their ownership and exact source fingerprint, invokes the production **read-only** `HostReadOnlyAdapter.NormalizedCompose` twice, and verifies that an immutable, JSON-encoded stdin override changes only the registered service's image. It exercises env interpolation, a sibling service, `pull_policy: always`, Traefik-style labels and an external Compose network. A later env change must invalidate the prior source fingerprint.

No containers are created, started, pulled, stopped or mutated in this fixture. The CI job runs `docker compose version` and `config --format json` only, and uses no VPS paths or credentials. This is concrete evidence for the GitHub Actions host's Compose plugin; it **does not** establish equivalence with the test VPS's observed Compose v5.5.1.

The image-only configuration guard is also extended to reject high-risk service privileges on **every** service, not just the selected target: host networking/PID/IPC namespaces, devices, capabilities, system security overrides, and Docker socket or sensitive /proc, /sys, /run, /var/run and /dev mounts. Explicit adversarial tests cover sibling abuse as well.

## Remaining release gates

B6-04 is still not an executor authorization. A disposable Docker execution test must establish the exact Engine/Compose command semantics, a stable source snapshot preserved into the future write-capable Compose operation, precise version allowlist, fresh actual baseline/recheck, and approved terminal confirmation. Test-VPS acceptance needs a separate explicit operator decision after off-host backup/restore drills. The monitor remains GET-only with no executor invocation capability.

No actual registry pull, Compose up, live Docker mutation, release, tag or VPS change is implemented by this PR.
