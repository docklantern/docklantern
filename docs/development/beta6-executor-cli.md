# Beta.6 Host Executor CLI — Phases 1a–1c

**Status:** Phase 1a/1b merged; Phase 1c read-only diagnostics proposed for review under [B6-01 (#81)](https://github.com/docklantern/docklantern/issues/81). This is not an executor release or authorization to deploy.

The separate `cmd/docklantern-executor` is a **non-deploying** host CLI under ADR 0008. It does not import `internal/poc`, load Compose sources, listen on a port, or modify Docker. Phase 1c adds only explicitly requested **read-only Docker/Compose CLI queries**.

`help`, `version` and opt-in read-only `diagnose` are implemented. `status`, `register`, `preview`, `apply`, `reconcile` and `recover` are intentionally reserved and exit nonzero with an unambiguous unavailable message; they do **not** attempt any filesystem or Docker IO (apart from printing the error). Unknown or unsupported arguments also fail. There is no `--yes` or background operation.

## Local development only

```sh
go test ./internal/executorcli
go test -race ./...
go vet ./...
go build -o /tmp/docklantern-executor ./cmd/docklantern-executor
/tmp/docklantern-executor version
/tmp/docklantern-executor help
```

These example commands compile and run help/version only; they perform no Docker action. The binary is **not part of the monitoring container**, is not released or installed, and has no authority or configuration for a Docker update. The binary audit and read-only preflight trust helpers are covered by merged Phase 1b. Exact version support, executable release packaging and host installation remain unapproved.

## Security boundaries and follow-up gates

- Keep beta.5 monitoring image and GET-only socket proxy unchanged. Do not copy this executable into the monitoring Docker image.
- **B6-01b:** add narrowly constrained read-only host inspection/configuration checks, protected storage ownership tests, and separately audited binary build/identity without expanding the CLI into deployment functionality.
- **B6-02/B6-03:** protected registrations, strict proposals, authorization, journal and read-only reconciliation.
- **B6-04:** demonstrate exact supported Docker Engine/Compose CLI version and safe immutable image-only preflight.
- **B6-05:** Docker writes only after independent approval and disposable-host security gates; never through the monitoring web process.
- **B6-08:** off-host backup and actual restore drill plus separate explicit permission before any test-VPS execution.

**Governance:** no PR merges, tags, release publication, Docker write-capable code or VPS changes without separate operator approval.

## Phase 1b — Host trust and executable artifact checks (merged)

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
**Phase 1c uses this environment only for explicitly requested, fixed read-only diagnostic commands.** Deployment commands remain disabled.

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

## Phase 1c — Opt-in read-only Docker/Compose diagnostics (review pending)

The new `diagnose` command performs exactly four read-only Docker CLI queries:
`context show`, local-socket `version --format`, local-socket `info --format`
and `compose version --short`. The Docker endpoint is fixed to
`unix:///var/run/docker.sock`. No Docker pull/up/exec, shell, HTTP listener,
proposal ingestion, registration or deployment path is implemented.

The CLI uses only trusted root-owned, absolute, nonsymlink executable paths
(default `/usr/bin/docker` and `/usr/libexec/docker/cli-plugins/docker-compose`,
with alternative paths supplied by the authorized **host operator** using
`--docker-binary` and `--compose-plugin`). It verifies that the actual
Compose plugin path reported by Docker matches the prevalidated path before
running `compose version`. See [Docker's plugin path instructions](https://docs.docker.com/compose/install/uninstall/#inspect-the-location-of-the-compose-cli-plugin).

It requires *pre-existing*, root-owned private
`/var/lib/docklantern/executor` and
`/var/lib/docklantern/executor/docker-config` directories. It never creates
or chmods them. Its Docker child environment is constructed from an explicit
allowlist, never inherited caller `DOCKER_HOST`, `DOCKER_CONTEXT`,
`COMPOSE_FILE`, Docker credentials or `LD_PRELOAD`. Each command has a
5-second timeout and bounded 4 KiB stdout/stderr buffers; errors are
sanitized, and no raw child output is logged.

The output lists parsed Engine/API/Compose versions, context, plugin path
and host architecture, **always** saying `Deployment support: UNQUALIFIED`.
Recognizing a Compose V2/V5-style version is not approving an update.

Example **only on a separately approved disposable host with trusted paths**:

```sh
docklantern-executor diagnose
docklantern-executor diagnose \
  --docker-binary /usr/bin/docker \
  --compose-plugin /usr/local/lib/docker/cli-plugins/docker-compose
```

Faked filesystem/process tests exercise malicious command options, output
floods, version/plugin-path mismatches, wrong context, poisoned inherited
environments, invalid host paths and cancellation. The existing Go POC CI
runs unit/race/vet; security CI continues to build and scan the separate
host executor without publishing it.

**Explicit remaining gates:** `Lstat` preflight does not prevent TOCTOU,
nor prove the actual rootful daemon or Compose digest reconciliation behavior.
The observed test-VPS Engine 29.8.1 / Compose 5.5.1 is **not qualified**;
B6-04 must establish exact-version compatibility on disposable hosts.
The real `hello-go` parent mode `0775` is still ineligible.
No VPS diagnostics, host installation, source chmod, monitoring privilege
change, new deployment command, release or merge is authorized by this PR.
