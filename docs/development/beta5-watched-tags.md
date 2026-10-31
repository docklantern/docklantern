# Beta.5 Phase 1 — Registry watch target policy

**Scope:** [#65](https://github.com/docklantern/docklantern/issues/65) under [beta.5 #63](https://github.com/docklantern/docklantern/issues/63). Backend persistence only; GUI controls are planned in [#67](https://github.com/docklantern/docklantern/issues/67).

## Identity and precedence

- **Docker-observed image:** the tagged reference recorded in Docker inventory when the container was created. This is not changed by DockLantern.
- **Registry watch target:** the canonical tagged OCI image reference that monitoring checks. This is not a desired deployment target and does not grant Compose execution.
- For an eligible *label- or GUI-enrolled* service, watch target = **explicit GUI watch override → verified Docker-observed image reference**. Override changes are independent of GUI interval overrides.
- Existing interval precedence remains **GUI interval override → Docker interval label → global default**. Enrollment is still explicit label/GUI opt-in; `com.docklantern.monitor=false` always vetoes registry checks.
- **Legacy/manual registered policies are unchanged** and continue to control their own tagged watch target via existing registered monitoring policy operations. Phase 1 rejects watch overrides for legacy services instead of silently changing their preserved baselines. Multi-credential persistence remains out of scope ([#62](https://github.com/docklantern/docklantern/issues/62)).

## Operator action contract

`Store.SetServiceWatchOverride(ctx, project, service, image)` is a backend API **not exposed through HTTP in phase 1**.

- Non-nil `image` must be an **explicitly tagged** OCI reference; missing tag, digest-pinned references, embedded credentials, whitespace and invalid OCI identifiers are rejected.
- The canonical registry and repository must match the verified current Docker-observed reference. Only the tag can differ. The platform must match the last verified inventory and existing policy; an override cannot change architecture.
- A nil image resets to the Docker-observed tag; setting the same tag as Docker also clears a redundant override.
- Only a discovered, valid and currently enrolled non-legacy service may change its watch target. A paused service can change the target without resuming; an unenrolled, missing/conflicting or label-disabled service cannot.
- The preference survives DockLantern restarts, same-repository external Compose tag changes, temporary GUI disable/re-enable and temporary loss of discovery. If the running service changes to a **different registry/repository**, discovery marks a conflict and blocks monitoring until the operator resolves it. The stale override is preserved for review rather than silently discarded.

## SQLite and event semantics

- Add nullable `monitor_service_settings.watch_image_override` in transactional **schema v4** migration; NULL means inherited Docker tag. Existing beta.4.1 policy rows, registered legacy watches, paused/interval state and recorded histories remain unchanged.
- Effective `monitor_policies.image` updates atomically with the preference. Real changes increment policy `generation` so a stale in-flight monitor result cannot overwrite the new target. Reset only the service's last check/error/backoff fields and schedule a new registry check. Do **not** unpause, enable, pull or update a container.
- Global `registry_checks` and immutable `monitor_events` remain scoped to canonical image/platform and are **not** purged or fabricated by the override operation. Switching to a never-observed image yields a normal `first_seen` on its first successful check, not a fabricated `remote_digest_changed`. Switching back to a previously observed image can compare against that image's retained baseline; evidence must remain correctly image-scoped.
- A current image's real digest movement is **not** proof of a newer semantic software version.

## Security and rollout

The app remains monitoring-only with an unchanged, private GET-only Docker proxy. Phase 1 creates **no HTML/JSON mutation route**, registry request on page render, image pulls or Compose executor authorization. A schema v4 database must not be opened by an older v3 binary: before any separately approved deployment, take a verified compatible SQLite snapshot and define rollback by restoring that snapshot, not simply switching image tags.

Phase 2 Check Now requests and phase 3 UX must preserve these opt-out, eligibility, provenance and history rules.
