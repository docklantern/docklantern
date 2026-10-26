# Beta.3: Service-first monitoring and registry event explanations

**Status:** Implementation under beta.3 development; does not change the currently deployed beta.2 image.

## Overview / Services

The Overview dashboard displays each discovered Compose **project/service** once, regardless of replica count. The monitoring table shows:

- Compose project and service, watched image and platform; running replica count, health summary and last discovery snapshot.
- Enrollment source: Docker label, GUI, legacy manual, not enrolled or explicitly disabled.
- Monitoring state: monitoring, paused, disabled, invalid discovery metadata, not running or registry check failed.
- Effective polling interval and its source (GUI override, label, global default or preserved legacy/manual baseline), plus last/next check times.
- **Remote registry observation** distinct from **running image assessment**. A digest movement does **not** establish that a newer release exists.
- **Edit monitoring** opens Settings with the relevant service highlighted, without requiring project source paths.
- **View related events** opens filtered Event History. For a monitored image, filtering by image includes a shared digest-change event without inventing a single service owner.

The legacy registered-projects section remains available below the service overview, but source registration is **not required** for monitoring through Docker labels or the GUI.

## Image assessments for label-only services

Monitoring-only Compose services do not need a registered Compose source directory to appear in image assessments. The server continues to rely on fresh Docker and OCI digest evidence; container config IDs are **not** treated as immutable manifest digests. Without sufficient verified provenance the assessment remains **unknown** or **unverified**, never an asserted update available.

## Event History

`remote_digest_changed` events are global to an **image + platform**, even when multiple Compose services watch the same tag. Event History and the dashboard call these **Registry image event (image/platform scope)**, show the image, platform and old/new digests, and do not display an empty `/ ` for project/service. No historical association to specific services is invented.

A registry check event is separate from **webhook delivery**. `Webhook pending (0 attempts)` means a notification has not been confirmed delivered; a notifier may not be configured. It does **not** indicate a failed manifest request.

All events remain immutable monitoring observations. DockLantern does not pull an image, redeploy a Compose stack or restart a monitored container in beta.3.

## Security and quality

The dashboard is Go server-rendered HTML with shared sidebar, responsive tables and escaped container metadata; no additional Docker API privileges are introduced. E2E tests cover label-only enrollment, replica grouping, a three-check remote digest transition (first seen → changed → unchanged) producing exactly one image-scoped event, filters, CSRF-protected Settings routing and HTML escaping.

The full beta.3 release gate remains [Issue #34](https://github.com/docklantern/docklantern/issues/34).
