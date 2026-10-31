# DockLantern

**Compose-first, monitoring-only Docker image visibility for VPSs and home labs.**

DockLantern is a lightweight Go web application that discovers Docker Compose services through a **restricted read-only Docker socket proxy** and checks tagged OCI registry manifests for changes. It presents registry observations, conservative deployed-image comparisons, image-scoped history, and service monitoring controls.

> **Release status:** [v0.1.0-beta.4.1](https://github.com/docklantern/docklantern/releases/tag/v0.1.0-beta.4.1) is the published GitHub prerelease (October 28, 2026), and the operator confirmed the released image running and ready on the test VPS (October 31). The [deployment repository PR #5](https://github.com/docklantern/docklantern-deploy/pull/5) has since aligned its image pin to the published digest. The older `v0.1.0-beta.4` tag already published images from an earlier commit and must not be moved or overwritten; the PR #58 local test build was separate from the release. Future release publication and VPS deployment require new, separate operator approvals.

## Beta.4.1: Image-first web interface (released)

The GUI has five primary destinations:

- **Overview:** Watched Images, Remote Changes in the last 24 hours, verified Deployment Differences, Monitoring Errors, and an Images to Review queue.
- **Images:** Group monitored image references by canonical reference and platform; search, filter, sort, and inspect OCI registry observations separately from deployed digest assessments.
- **Image Details:** Last successful digest evidence, recorded image-scoped changes, and links to all associated Compose services.
- **Services and Service Details:** Compose project-grouped service inventory, discovered unmonitored services, runtime context, monitoring interval provenance, and authenticated enable/disable, pause/resume, override/reset controls.
- **Activity and Settings:** Day-grouped, filterable event timeline with expandable webhook delivery details; global defaults, notifications, retention and Advanced / Legacy Configuration.

**A remote digest movement does not establish that a newer software version is available.** Registry status and running deployment status are independent, and insufficient evidence is reported as Unknown.

Beta.4.1 remains monitoring-only. It does not add container pulls, automatic updates, Compose execution, container restarts, or Docker write permissions.

## Opt in a Compose service

Add the following labels to the **target service**:

```yaml
services:
  web:
    image: example/web:latest
    labels:
      com.docklantern.monitor: "true"
      # Optional: override the initial five-minute global default.
      com.docklantern.interval: "10m"
```

Recreate that service using your normal Compose workflow to apply changed Docker labels. DockLantern discovers the labels on its next inventory pass. A GUI interval change does **not** restart the service.

For a discovered unlabeled service, use **Services → Service Details** to enroll it. GUI override → Compose label → global default determines the effective interval. An explicit `com.docklantern.monitor=false` prohibits GUI enrollment. Monitoring enrollment is never authorization for future deployment operations.

## Security and current limits

- Use a private, read-only, GET-only Docker proxy. Never publish its Docker API port or mount a writable Docker socket in the app.
- Run the app behind approved TLS reverse proxy authentication, origin validation and CSRF controls; do not expose the plain HTTP listener publicly.
- Remote digest differences and stored Docker image config digests are not, by themselves, proof of an update being available.
- The scheduler currently uses **one configured registry credential host/account per process**, not per-service registry credentials. In-memory cache keys distinguish effective credential scope, but SQLite digest baselines and image grouping are keyed only by image reference and platform. **Multiple simultaneous credential scopes for one image, or scope-aware history across credential rotation, are not supported in beta.4.1.** See [registry monitoring limitations](docs/operations/beta3-registry-cache.md), [beta.4.1 acceptance](docs/deployment/beta4-release-checklist.md), and [future issue #62](https://github.com/docklantern/docklantern/issues/62).
- The UI is server-rendered, no JavaScript frontend framework is needed, and Docker mutations are explicitly outside beta.4.

## Guides, development and release

- [Beta.4 workspace guide](docs/user-guide/beta4-workspaces.md).
- [Beta.4.1 acceptance record](docs/deployment/beta4-release-checklist.md).
- [Beta.4.1 release notes](docs/releases/v0.1.0-beta.4.1.md) and [historical beta.4 draft](docs/releases/v0.1.0-beta.4.md).
- [Label-based monitoring](docs/user-guide/beta3-label-monitoring.md) and [existing interval controls](docs/user-guide/beta3-live-monitor-settings.md).
- [Deployment configuration](https://github.com/docklantern/docklantern-deploy) and [SQLite backup/upgrade guidance](docs/deployment/beta3-sqlite-upgrade.md).
- [Development guide](docs/development/README.md), [roadmap](docs/product/roadmap.md), [beta.5 planning (#63)](https://github.com/docklantern/docklantern/issues/63), [contributing](CONTRIBUTING.md), and [security policy](SECURITY.md).

Run local Go checks:

```sh
go test -race ./...
go vet ./...
```

**Disposable Docker integration tests** require `DOCKLANTERN_INTEGRATION_TEST=1` and must only run on a throwaway host. CI checks live in `.github/workflows/go-poc.yaml`. Do not run destructive integration scripts on a shared/production Docker daemon.

Automatic deployment, Compose source modification, multi-host management and semantic tag update policies remain future work.

Project website: [docklantern.com](https://docklantern.com). Secondary domain: [docklantern.dev](https://docklantern.dev).
