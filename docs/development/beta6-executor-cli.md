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
