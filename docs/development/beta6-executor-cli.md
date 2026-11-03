# Beta.6 Phase 1a — Host Executor CLI Skeleton

**Status:** Implementation proposed for review under [B6-01 (#81)](https://github.com/docklantern/docklantern/issues/81). This is not an executor release or authorization to deploy.

The independent Go executable `cmd/docklantern-executor` is an **inert shell** for ADR 0008. It imports the shared `internal/buildinfo` version and the new `internal/executorcli` command dispatcher; it does **not** import `internal/poc`, run subprocesses, access the Docker socket, open Compose files, listen on any port, or call the Docker Engine.

Only `help` and `version` are implemented. `status`, `register`, `preview`, `apply`, `reconcile` and `recover` are intentionally reserved and exit nonzero with an unambiguous unavailable message; they do **not** attempt any filesystem, network or Docker IO (apart from printing the error). Unknown or extra arguments also fail. There is no `--yes` or background operation.

## Local development only

```sh
go test ./internal/executorcli
go test -race ./...
go vet ./...
go build -o /tmp/docklantern-executor ./cmd/docklantern-executor
/tmp/docklantern-executor version
/tmp/docklantern-executor help
```

These commands only compile and run the inert CLI; they perform no Docker action. The binary is **not part of the monitoring container**, is not released or installed, and has no authority or configuration for a Docker update. Release packaging, separate binary security audit, protected filesystem layout and trusted CLI/daemon identity checks remain B6-01b follow-up work.

## Security boundaries and follow-up gates

- Keep beta.5 monitoring image and GET-only socket proxy unchanged. Do not copy this executable into the monitoring Docker image.
- **B6-01b:** add narrowly constrained read-only host inspection/configuration checks, protected storage ownership tests, and separately audited binary build/identity without expanding the CLI into deployment functionality.
- **B6-02/B6-03:** protected registrations, strict proposals, authorization, journal and read-only reconciliation.
- **B6-04:** demonstrate exact supported Docker Engine/Compose CLI version and safe immutable image-only preflight.
- **B6-05:** Docker writes only after independent approval and disposable-host security gates; never through the monitoring web process.
- **B6-08:** off-host backup and actual restore drill plus separate explicit permission before any test-VPS execution.

**Governance:** no PR merges, tags, release publication, Docker write-capable code or VPS changes without separate operator approval.

## Phase 1b — Host trust and executable artifact checks (in review)

The new `internal/executorhost` library provides read-only checks for a
future trusted host runtime. `ValidateTrustedExecutable` rejects binaries
that are not root-owned regular executables beneath root-owned, nonwritable,
nonsymlink ancestors. `ValidateTrustedSource` rejects group-/world-writable
Compose-source ancestors (including the current `hello-go` parent at `0775`)
or unexpected owners. `Layout.Validate` checks protected, root-owned journal,
registration, override and receipt directories, requiring a distinct
unprivileged monitor owner for the proposal inbox. No host directories are
created or permissions modified.

`MinimalChildEnvironment` ignores caller environment variables and specifies
a fixed local Docker socket, isolated Docker configuration directory, and
no inherited `DOCKER_CONTEXT`, `COMPOSE_FILE`, `LD_PRELOAD` or credentials.
**It is not yet used to run any executable.** CLI help/version and disabled
deployment commands remain exactly as in Phase 1a.

These `Lstat` checks are **preflight-only**: they do not defend against
concurrent replacement of trusted files, guarantee stable Compose source
resolution, verify runtime Docker/Compose versions, or establish actual
monitor bind-mount isolation. Those controls remain prerequisites for later
phases; this library is not authorization to perform Docker mutations.

The Go POC CI workflow now includes a disposable nonroot Unix ownership
fixture proving a monitor identity can write its isolated inbox but cannot
write executor journal or receipt paths. That fixture does **not** test
real monitoring-container bind mounts. Existing GET-only Docker proxy tests
remain required.

The security workflow builds the standalone Linux/amd64 executor from the
same digest-pinned Go builder as the monitor image, records its checksum/build
metadata, and runs `govulncheck -mode=binary` on that exact executable.
The output remains ephemeral inside CI: no artifact upload, release, host
installation, or copy into the monitoring Docker image.

**Still blocked:** secure descriptor-relative opens and snapshot/TOCTOU
proof, fully verified Docker and Compose binaries/plugins and supported
versions, protected registration and TTY authorization, journaling,
reconciliation, update/recovery operations, and VPS acceptance.
