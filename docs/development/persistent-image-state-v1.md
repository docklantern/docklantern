# Persistent image-only Compose state — first release contract

**Status:** implementation groundwork; not deployable, no automatic updates authorized.

The beta.6 host executor proves a selected-service immutable image replacement,
but its override exists only in the stdin stream of one Compose invocation.
Its journal records the committed candidate digest but an unmanaged Compose
operation can restore the original image.

For the first operational release, persistence is an explicit **opt-in project
contract**, not a promise to intercept every Compose invocation.

## Eligible project subset

- A trusted, host-registered, public Docker Hub `linux/amd64` service meeting
  all existing beta.6 source, OCI, Docker and Compose qualifications.
- Exactly one project-root `compose.yaml` base file for **default-file**
  compatibility; no pre-existing or independently managed
  `compose.override.yaml`. Other layouts remain supported by beta.6's
  manually approved, ephemeral update path until separately qualified.
- All ordinary default-file Compose operations for enrolled projects run from
  the registered project root, and consume the managed override. A command
  with explicit `-f` files **does not automatically include** that override.
  It must list the managed override last in the agreed file sequence.
- No external writer changes the base Compose sources, overrides, image,
  container identity or protected journal while the executor owns the
  project lock. Any observed drift blocks further automation.

## Required executor integration (not implemented in this first commit)

1. Make persistent management a separate, trusted, host-authorized project
   setting. Do not infer it from a monitoring watch, label or GUI action.
2. Before each operation, compare the currently deployed image and the
   existing persistent override to the last committed journal generation.
   An existing unknown or mismatched file is **not adopted or overwritten**.
3. Use the shared `executoroverride.Render` and `VerifyExact` for a bounded
   immutable image-only Compose payload. Validate project-directory and file
   ownership without symlink traversal and check the exact Compose config
   produced with the persistent override.
4. Publish the new override with atomic replace/fsync under the held project
   lock **after** verification but **before** successful journal COMMIT.
   Publication and journal updates are separate durability domains: failure
   at either boundary means *needs reconciliation*, not a reported success.
5. On restart or the next operation, verify file bytes, committed generation,
   source identity and observed Docker image before accepting further work.
   Stale journal backups, missing files, foreign owners, unexpected Compose
   changes and ambiguous interrupted operations must fail closed.
6. Keep monitoring read-only. Mount only sanitized executor-originated
   receipts read-only, never the journal or managed override writable.
   Display last successful executor outcome separately from current Docker
   observation: a success receipt is historical, not proof of present state.

## Compatibility and testing gates

The new package is only a renderer and a conservative default-file shape
validator. It **does not** write a persistent file, broaden host permissions,
activate an unattended executor, or make beta.6's live update persistent.

Before making this PR ready, test at least two sequential approved updates;
an ordinary default Compose up/recreate and host restart; an explicit `-f`
invocation with and without the managed override; drift and filesystem races;
interruption between durable override publication and journal COMMIT; and
accurate receipt/actual-image reporting. Run the Docker mutation fixtures
only on disposable CI/test targets with separate host approvals.

The eventual fresh-VPS deployment must independently validate a tested
runtime version contract, off-host backup and restore, and the root-owned
source/override mount boundary. Do not weaken beta.6's existing GET-only
Docker proxy or on-demand TTY apply path.

References: ADR 0008 and Docker's
[Compose merge instructions](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/).
