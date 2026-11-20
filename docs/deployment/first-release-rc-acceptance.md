# First-release RC: paired artifacts and gated VPS acceptance

**Status:** pre-release plan, not permission to publish or deploy. The [existing VPS functional acceptance record](first-release-existing-vps-results.md)
records evidence and remaining release gates. This document
supersedes historical beta.6 one-shot test instructions for the FIRST automatic
image-update RC. No release tag or Docker Hub image is created by this PR.

## Scope and stop conditions

The release candidate must prove one ordinary workflow end to end:

1. Publish a new image tag for a disposable `hello-go`, `hello-py` or
   `hello-js` application, with the operator's separate authorization.
2. On the existing disposable VPS, the **host-local** DockLantern timer
   resolves the approved tag and updates ONLY the enrolled service to its new
   immutable digest, using the protected journal and persistent override.
3. The web GUI independently shows the currently observed image/container,
   updated registry information and historical sanitized executor receipt.
4. Ordinary default-file Compose recreation/restart preserves that digest.
5. Repeat with a second newly published digest; an unchanged tag must not
   trigger another recreation. Siblings, Traefik and proxy remain unchanged.

Then repeat the functional install/acceptance on a fresh disposable VPS.
No private registries, automatic rollback, self-updates, cosmetic UI work,
generic CLI features or monitoring-container Docker write access are in scope.

**Fail closed:** any missing backup, untrusted source path, wrong runtime,
absent/mismatched digest, unresolved journal, rejected registration, unsafe
container drift or missing GUI evidence must be documented; never silently
repair it or claim complete acceptance.

## RC0 — source, CI, artifact pairing (no VPS changes)

After selecting a reviewed `main` SHA and obtaining approval to build local
artifacts on an isolated workstation/CI runner:

```bash
git status --short
git rev-parse HEAD
bash scripts/verify-release-inputs.sh
# LOCAL DOCKER BUILD ONLY (not an SSH session on a server):
DOCKLANTERN_RC_BUILD=1 bash scripts/build-rc-bundle.sh /absolute/new/rc-directory
bash scripts/verify-rc-bundle.sh /absolute/new/rc-directory "$(git rev-parse HEAD)"
```

The build script creates a **local-only** Docker image and archive
(`monitor-linux-amd64.tar`), a separate statically linked
`docklantern-executor` from the exact digest-pinned Go builder, a
`provenance.txt` (commit, version, builder, local image ID, published=false)
and `SHA256SUMS`. It refuses dirty source, pre-existing output or Docker
endpoint overrides. The app image has **no** Docker CLI or executor. CI
runs this packaging in a disposable runner; CI artifacts are not installed,
published, signed or automatically transferred.

The checksum file proves integrity **only after an independently verified
transfer**; it is not source authenticity or a release signature. Do not
confuse the local Docker image config ID with a registry OCI index digest.
Take an off-host inventory/backups before any subsequent VPS modification.

## RC1 — existing VPS baseline and host approvals

Use the prior [beta.6 acceptance record](beta6-test-vps-acceptance.md) as
historical evidence, not blanket authorization for new operations. Record:

- Current Docker Engine/Server `29.8.1`, API `1.56` and Compose `5.5.1`
  **or explicitly establish a new compatibility contract before updating**.
  The current code has exact runtime checks. Do not install a different
  Compose version merely to force an update through the gate.
- Docker CLI/context/socket, selected target and sibling container IDs,
  image IDs, Compose project/service labels, health, mounts/networks, HTTPS
  reachability and restricted proxy behavior (GET only, POST denied).
- A consistent monitoring SQLite snapshot **plus** protected executor
  journal, registrations, source data and override generations, with
  recorded checksums. On a disposable VPS, any waiver of off-host custody
  must be an explicit, narrow operator decision.
- The separately approved host executor installation path and binary SHA-256
  match the reviewed RC output. Never copy the executor into the monitor
  container, attach a raw Docker socket to it or change socket-proxy POST
  permissions.

The original beta.6 `hello-go-beta6` journal generation may predate
persistent overrides. **Do not adopt it automatically**: its committed
journal digest may exist without a matching project-local override. Prefer a
**fresh, disposable project/registration** for the RC; any older project
migration must be reviewed separately.

## RC2 — source and standing authority

Only after approval, provision one root-owned project under a trusted
non-group-writable ancestor, with an exact root-owned `compose.yaml`
and optionally root-owned `.env`. The existing user-owned/group-writable
hello app checkout is **not eligible** for persistent source registration.
Do not chmod/chown it in place without reviewing ownership and behavior.

Prepare a full protected registration for the disposable project with:

- exactly one enabled `web` service and explicitly approved
  `docker.io/<namespace>/<repository>` on `linux/amd64`;
- `persistent_override: true`, and
  `automatic_updates: {"enabled":true,"tag":"latest"}` (or another
  operator-approved tag);
- `allow_running_only` only if the disposable service lacks a Docker
  healthcheck and the operator explicitly accepts that weaker condition.

The standalone root executor's **existing interactive** `register --file`
command remains the only way to create/replace that protected authority.
A replacement requires `--expect-sha256` of the prior record. The
monitor's enrollment label or watched tag is **not** standing approval.
Record the resulting registration fingerprint and protected host-state
health without publishing secrets.

## RC3 — monitor and receipt visibility

Deploy the RC monitoring image using a separately approved, reviewed Compose
overlay/pin and backup. Do NOT edit the persistent executor target Compose
override manually. Keep the monitoring Docker socket proxy read-only.

Before the first automatic update in an acceptance test, provision the receipt
outbox and approved read-only mirror so positive Activity evidence is available.
The optional monitor receipt
mirror from `/var/lib/docklantern/handoff/receipts` to the monitor's fixed
`/run/docklantern/executor-receipts` **read-only**, after confirming the
directory/file ownership and modes match the protected receipt publisher.
Require a real root-owned directory with exact mode `0755`, all ancestors
root-owned and not group/world-writable, and root-owned single-link regular
receipt files with exact mode `0644`. Reject symlinks. Keep all private
executor directories protected; do not recursively chmod the handoff tree.
A `0700` outbox prevented receipt publication despite a successful deployment
on the existing test VPS. Correct provisioning before starting the timer;
unchanged ticks do not backfill a previously missing receipt. See the
[deployment provisioning checklist](https://github.com/docklantern/docklantern-deploy/blob/main/docs/rc-receipt-overlay.md).
This mount contains only sanitized receipt JSON; never mount registrations,
protected journal/SQLite, source snapshots, host executor binary or Docker
socket into the monitoring container.

Check fresh inventory timestamps and record the deployed container image ID
separately from registry tag/index digest. A successful past receipt is
historical evidence, not proof of the CURRENT image. A missing receipt mirror
must remain visibly unknown, never fabricate a success.

## RC4 — approved automatic end-to-end test

After confirming root-only project trust and a verified backup, install the
**separately reviewed** host timer templates; start only the explicitly
approved registration instance. Do not enable a wildcard/global timer.

For each of TWO distinct approved image revisions:

1. Record the approved repository/tag, source commit, existing target ID and
   current registry digest **before** publication. Prefer independently recording
   the new expected OCI index before it becomes the watched tag. If the normal
   publishing workflow builds and promotes `latest` together, the new digest
   cannot be known in advance: obtain explicit approval for the resulting digest
   from that exact source/tag, document the exception, and independently compare
   the versioned and watched tags as soon as publication completes. The active
   timer may deploy before that comparison. Stop acceptance if the digests or
   approved publication provenance differ.
2. After separate image publication approval, use the normal hello app
   workflow to publish the new tag; verify Docker Hub index digest and
   `linux/amd64` platform manifest. A digest change must be real, not a
   rebuild with identical bytes.
3. Allow the host timer tick or invoke the **single** `auto-once
   <registration-id>` from the approved host procedure. Never bypass the
   executor with `docker compose up` or an HTTP request.
4. Independently inspect the new target container and image identity, health,
   protected journal COMMIT, managed `compose.override.yaml` exact image
   reference, and unchanged sibling/Traefik/proxy IDs.
5. Verify the GUI's current deployed image and watch target agree with fresh
   observations; compare positive receipt history with its operation ID.
6. Run an unchanged tick; **target ID must not change**. After both updates,
   under separate approval stop only the enrolled timer and verify the executor
   is idle. Recreate only the disposable target using default-file Compose,
   `up -d --no-deps --no-build --pull never --force-recreate web`, and then
   `restart --no-deps web`. Check that image identity, original source, managed
   override, protected journal and siblings are preserved. Resume the existing
   timer only on success; a subsequent unchanged tick must preserve the new
   container ID. Verify fresh GUI inventory.

Abort on incomplete journal commit or missing trusted evidence. The beta
executor does not implement unattended recovery. Do not clear SQLite state
or delete a managed override as an automatic workaround.

## RC5 — negative checks and clean VPS

On **disposable isolated fixtures**, simulate stale override/journal backups,
unhealthy candidate, revoked policy, registry failure, changed service source,
unsupported runtime and a crash at the override/JOURNAL commit boundary.
The expected behavior is NO unauthorized update and manual reconciliation,
not self-healing or an unverified success indicator.

Finally, install the same reviewed RC (not a new unreviewed rebuild) on a
clean VPS using separately approved host, Docker, Compose, TLS, IPv6 egress,
backup and root policy provisioning. Re-run RC1–RC4 checks there, with a new
registration and no inherited old journal.

## Completion criteria

Record exact reviewed commit, CI/security status, checksums, runtime versions,
the two immutable index digests, observed target image/config/container IDs,
unchanged sibling IDs, both journal operations, override persistence, fresh
GUI/receipt evidence, negative checks and clean VPS results. A signed-off
acceptance record is required before the first release tag, public image
publication or production recommendation.

**Any tag beginning `v*.*.*` can trigger the publishing release workflow.**
Creating/pushing a release tag and deploying an RC are independent approval
gates and are outside this development PR.
