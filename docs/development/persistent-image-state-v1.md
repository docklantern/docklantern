# Persistent image-only Compose state — first release contract

**Status:** draft opt-in implementation; unaccepted and not deployed. Automatic updates remain unauthorized.

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

## Implemented safeguards and remaining integration gates

1. **Implemented:** `persistent_override` is an optional field in a root-owned,
   independently registered project record, never inferred from a monitoring
   watch, label or GUI action. Old beta.6 registrations default to false.
2. **Implemented:** the executor reads the latest committed desired digest
   from protected journal state, validates exact bytes and root ownership of
   `compose.override.yaml`, and rejects unknown, missing or mismatched files
   before a new apply. It also checks the prior Docker image reference.
3. **Partially implemented:** `executoroverride.Render` and `VerifyExact`
   supply exactly one digest-pinned service image. Root-owned project and file
   access uses no-follow directory traversal. A real Compose validation
   fixture must additionally prove the *default-file* and explicit `-f`
   behavior on the supported test host.
4. **Implemented in the host apply path:** a new override is written via
   exclusive private staging, fsync and atomic rename before the protected
   verified journal COMMIT under the existing project lock. Failure during
   publication prevents success and leaves the operation unresolved. The
   journal/file crash boundary still requires fault-injected integration tests.
5. **Partially implemented:** a subsequent apply checks committed journal
   digest against the exact public override and running image reference.
   Interrupted updates remain blocked by the existing executor journal.
   Restoring stale journal/override backups and unexpected external Compose
   configuration changes still require a witnessed end-to-end test.
6. **Partially implemented:** the GUI now separates Docker's local image
   configuration ID and observed container ID from any OCI digest and presents
   inventory timestamps. Deployment Activity remains a historical receipt
   view; the read-only receipt mount and positive outcome/inventory
   correlation still need host acceptance.

## Compatibility and testing gates

The draft now has an opt-in persistent mirror with root-owned filesystem
checks and on-commit publication. It has **not** been validated against a
real sequential Docker Compose update or approved for deployment. It does
not broaden host privileges, enable unattended execution, or modify old
registrations that have not explicitly selected persistence.

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
