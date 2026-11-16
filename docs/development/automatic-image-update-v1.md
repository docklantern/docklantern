# Host-local automatic updates — first standing-authority milestone

**Development status: PR only, not installed, not deployed.** The monitoring
container, registry polling GUI, Docker socket proxy, and manual TTY-confirmed
`apply` command do not gain any write authority.

## Scope and authorization

An independently registered, root-owned `compose.yaml` project with exactly
one enabled prebuilt public Docker Hub linux/amd64 service may opt into
`automatic_updates`. This is **standing authorization** to replace that
specific service when the approved tag changes; it is not just a watch flag.

Protected registration example (extend an otherwise valid existing record):

```json
{
  "persistent_override": true,
  "automatic_updates": {
    "enabled": true,
    "tag": "latest"
  }
}
```

That fragment is **not** a complete registration. The host administrator must
create or replace the full protected record using the existing
`docklantern-executor register --file ...` flow and its exact TTY
`REGISTER` challenge. Replacement requires `--expect-sha256` with the
previous protected fingerprint. A policy cannot be set by Docker labels, GUI
controls, HTTP, monitor proposals or untrusted environment variables. Removing
`automatic_updates` through another approved registration replacement revokes
standing authorization for future ticks.

For this initial slice, the approved tag must be public on Docker Hub. No
private registry credentials, generic image repositories, multi-service
projects, self-update of DockLantern/Traefik/proxy, or automatic rollback.

## Host tick and results

`docklantern-executor auto-once REGISTRATION_ID` requires effective UID 0
and the protected registration. An independently approved host timer may call
it periodically. The command:

1. Resolves the exact approved tag using anonymous HTTPS to Docker Hub and
   verifies its immutable OCI index and linux/amd64 platform manifest.
2. Acquires the per-project host lock. Reopens the same trusted registration
   and verifies that the tag still resolves to the original pinned digest.
3. Validates the protected journal and persistent Compose override. A matching
   committed digest AND matching running container reference is a no-op.
4. Constructs an in-memory, bounded typed candidate from the actual Docker
   container identity. No monitor-owned proposal file, dynamic Compose command
   or HTTP approval endpoint is involved.
5. Uses the existing trusted source snapshot, Compose image-only diff,
   independently retrieved OCI evidence, Docker runtime/version and actual
   container checks before the effect.
6. Reuses the **same journaled executor phases, immutable pull, single-service
   Compose up, read-after-write verification, persistent override publication
   and COMMIT** as manual apply. Any incomplete or ambiguous operation blocks
   future updates pending manual reconciliation.

A failed tick is an error, **not proof that an update happened**. No temporary
monitor-side permission to Docker is needed. Normal `apply` still requires a
controlling terminal and an exact one-time `APPLY` challenge.

The monitor independently polls public registries for notifications and
displays Docker inventory and sanitized receipt history; it is never asked to
perform the actual update. GUI correlation and live deployments are verified
later in a release-candidate acceptance session.

## Proposed systemd timer (not installed by the PR)

Unit templates are committed under `deploy/systemd/`. After an explicit
deployment approval, the operator can inspect and copy the templates into
`/etc/systemd/system/`, install an approved host executor binary, and enroll
each authorized registration separately:

```sh
# ONLY AFTER operator approval and installation/backup checks.
sudo systemctl daemon-reload
sudo systemctl enable --now 'docklantern-auto@hello-go-demo.timer'
sudo systemctl list-timers --all 'docklantern-auto@*'
```

A timer instance name is a protected registration ID, **not** a project path,
repository, image, Compose filename or CLI option. Default cadence is ten
minutes; each enabled instance runs a single bounded tick, not a daemon.
Failure and journaling are inspected with `journalctl -u
docklantern-auto@hello-go-demo.service`.

## Release candidate acceptance (separately authorized)

- Root registration/source migration and safe protected SQLite backup/restore
- Initial update from a disposable hello-app tag to index digest A
- Push a new tag to the approved Docker Hub tag and observe digest B
- Observe at most one target replacement, unchanged siblings and journal COMMIT
- Confirm default-file Compose reload retains digest B, including restart
- Check unchanged ticks never recreate the service; tag/registration drift
  never silently authorizes another repository or service
- Verify interrupted/pending journal, tampered override, unhealthy image,
  missing registry, unsupported runtime and revoked standing policy block writes
- Compare fresh GUI inventory with signed-off receipt history and test recovery
  under manually approved host procedures

**Governance:** CI may create and mutate disposable Docker fixtures. This PR
does not install timers, run updates on a VPS, publish images, tag releases or
merge itself.
