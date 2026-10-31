# Beta.5 Phase 3 — Image & Service Details controls

Implementation for [#67](https://github.com/docklantern/docklantern/issues/67), building on Phase 1 [#65](https://github.com/docklantern/docklantern/issues/65) (tag persistence) and Phase 2 [#66](https://github.com/docklantern/docklantern/issues/66) (durable Check Now scheduler).

## Service Details

- **Docker-observed container image** remains read-only, separate from **Registry watch image**.
- **Watch target source** shows whether the registry target comes from the Docker reference, an explicit GUI override, or a legacy manual policy.
- Eligible, enrolled, nonlegacy services can save an **explicitly tagged reference in the same registry/repository** using `POST /settings/monitoring/watch`. This route requires Basic Auth, existing configured-Origin and CSRF validation, and delegates all platform/label/identity checks to `Store.SetServiceWatchOverride`.
- **Reset watch** restores the latest verified Docker-observed tag. The specific safe repository-drift conflict permits reset even if regular watch edits are disabled; other discovery conflicts and explicit monitor=false remain non-overridable. Legacy manual policies retain their advanced Settings workflow.
- **Check Now** requests a read-only registry observation for an already-enrolled service. The button does **not** bypass an explicit `monitor=false`, pause, invalid discovery or unenrolled status; an adversary directly posting to the endpoint is still blocked by store eligibility.
- Monitoring diagnostics distinguish **last attempt**, **last successful registry observation for this exact watched image/platform**, **last manual request**, **consecutive failures**, and **next scheduled check**. Failures can entail retry/registry cooldown.
- The form reports `saved=watch` only after the store commit, and Check Now redirects convey a strict whitelist of scheduler statuses. A redirect is **not evidence that the remote check completed**.

## Image Details

- A **Check Now** form appears only if at least one associated watcher is currently eligible. The page identifies the exact watched image/platform and number of eligible associated services.
- The image-scoped request targets that image/platform group using the existing opaque ImageID. Backend safely defers the entire group when rate-limited, and never accepts an arbitrary client-supplied registry URL.
- Registry evidence and immutable events remain scoped to the exact reference/platform; a tag change does not rewrite prior observations, declare a newer software release, or alter Compose desired state.

## Security, navigation and accessibility

All controls use the established server-rendered HTML with accessible `<label>`, normal form POST/303/GET redirects, `role="status"` notices and existing shell/breadcrumbs, styles and responsive layouts. Templates escape text; no JS runtime, external assets, new Docker write privileges, registry credentials UI, new socket permissions, auto-updates, Compose commands or container restarts are introduced.

A disabled, paused, unenrolled, or invalid watch cannot request manual registry I/O through a forged POST. Unsupported or stale status query values are rejected, and handler errors are generic to prevent leaking registry credentials or Docker metadata.

## Acceptance and deployment

Review and run Go CI, `go test -race ./...`, `go vet ./...`, HTTP security/regression and browser checks prior to merge. A merged phase-3 UI PR **does not** authorize image publication, VPS deployment, or automatic SQLite upgrade. The on-VPS beta.4.1 database is schema v3; the beta.5 branch includes forward-only v4 and v5 migrations. An approved future deployment must first verify a consistent backup, and rolling back the binary requires restoring the compatible old database snapshot.
