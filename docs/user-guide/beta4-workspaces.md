# Using the beta.4 DockLantern interface

**Status:** Documentation for the in-review v0.1.0-beta.4 release candidate. These pages are not available in the published beta.3 image.

DockLantern is a **monitor**, not an image updater. A different digest is a remote OCI observation or a verifiable deployed-image difference; it is not a claim that a newer application release exists.

## Overview

The home page summarizes **unique watched image references/platforms**, not replica count:

- **Watched Images:** total watched image/platform groups.
- **Remote Changes (24h):** groups with a *retained* registry digest-change event in the last 24 hours, even if the subsequent registry check was unchanged.
- **Deployment Differences:** groups with at least one verifiably differing deployed manifest digest.
- **Monitoring Errors:** groups with one or more non-paused service monitoring errors.

**Images to Review** prioritizes registry failures, verifiable deployment differences, recent digest changes and stale observations. It links to Image Details. Recent activity and last-recorded worker status provide additional context; a last worker heartbeat is not proof of a presently running process.

## Images and Image Details

Use **Images** to search by registry/reference, platform or associated Compose project/service; filter by registry and deployment status, and sort by attention, name, last successful check, or number of watching services.

**Registry status** describes the latest successful remote check plus monitoring failure and staleness signals. The retained last-change timestamp is tracked separately. **Deployment status** is based on verifiable comparison of running service digests, and can be Current, Different, Mixed, Unknown or Not Running. Missing provenance is reported as Unknown.

Open **Image Details** to inspect the registry reference/platform, OCI index and platform manifest digests (where available), last successful check, associated services and the exact image/platform event history. Events are paginated and retained history may be pruned by the operator.

A change event is image-scoped: if several Compose services watch the same reference/platform, the event does **not** belong to an arbitrary individual service.

## Services and Service Details

**Services** groups discovered and registered services by Compose project. Replicas share one row. Search/filter by service, image and monitoring state. Discovered but unenrolled services still appear.

**Service Details** shows discovered image/platform, running-container/health context, conservative deployed evidence, monitoring source/precedence, last attempt, scheduling and a link back to Image Details.

When allowed by the persisted monitoring rules, you can use authenticated GUI forms to:

- Enable or disable GUI enrollment on an unlabeled service.
- Pause or resume monitoring.
- Override the polling interval (60–86,400 seconds).
- Reset the GUI interval override to the Docker label or global default.

The effective order is **GUI override → Docker label → global default**. A Compose label explicitly setting `com.docklantern.monitor=false` blocks GUI enrollment even if a hand-crafted POST attempts to bypass the form. These monitoring changes do not pull, restart, or redeploy the target service.

## Activity

**Activity** groups retained events by UTC day. Filter by type, webhook delivery state, and project/service/image text. Use **Older events** for cursor pagination. Remote changes, registry failures, recoveries and prolonged outages are shown with human-readable labels. Routine unchanged polls are not digest-change events.

An event can link to a current Image Details page if the image is still watched. Expand **Webhook delivery details** to inspect pending/delivered status and retry metadata. No notifier is implied by a pending event; webhook delivery is separate from OCI monitoring.

## Settings

The main **Settings** page contains:

1. **Monitoring defaults:** Global polling interval with common suggestions and a validated custom seconds input.
2. **Notification preferences:** Which persisted event categories are eligible for separately configured HTTPS webhook delivery, plus failure thresholds.
3. **History and retention:** Delivered-event retention period and a separately confirmed manual prune. Pending alerts remain intact.

Older service forms, discovered service prefill, manual project registration, manually watched services and original registration/schedule tables are available under **Advanced / Legacy Configuration**. Service management should ordinarily happen in Service Details.

Retained history is not automatically deleted merely by changing retention days.

## Operational notes and limits

- Use TLS, existing authentication and the approved origin/CSRF setup.
- Keep the Docker socket proxy private and GET-only.
- The current scheduler provides one process-wide credential host/account tuple; the registry response cache protects the effective auth scope in memory, but persistent observation keys contain **only image reference and platform**. Multiple simultaneous credential scopes for a single image, or credential-scope history across credential rotation, are **not supported**.
- Monitoring scheduling and registry checks happen in the background; refreshing a page does not force a remote request.
- This guide documents the beta.4 candidate, **not proof of an operator-approved beta.4 production deployment**.
