# DockLantern

**Compose-first, monitoring-only Docker image visibility for VPSs and home labs.**

DockLantern is a lightweight Go web application that discovers Docker Compose services through a **restricted read-only Docker socket proxy** and checks tagged OCI registry manifests for changes. The interface displays monitoring status, remote digest observations, historical events, and conservative running-image assessments.

> **Release status:** beta.3 development is in release validation; a beta.3 image has **not yet been published**. The separately managed [deployment repository](https://github.com/docklantern/docklantern-deploy) still pins `v0.1.0-beta.2` until a reviewed release is approved.

## What beta.3 adds

- **One-label opt-in:** Add `com.docklantern.monitor: "true"` to the *target Compose service*. DockLantern automatically discovers it on the next inventory scan without adding source mounts, changing DockLantern's configuration, or registering project paths.
- **Optional intervals:** Add `com.docklantern.interval: "10m"`, inherit the persisted global default (initially five minutes), or customize/reset the interval through the GUI. Precedence: GUI override → Docker label → global default.
- **Monitoring controls:** Enroll an unlabeled discovered service in the GUI, pause/resume checks, and inspect where each setting came from.
- **Consistent navigation and service status:** Shared sidebar on all pages, per-service status, read-only registry history, clear event scope, and webhook delivery status.
- **Efficient OCI polling:** Bounded cache and credential-isolated registry rate-limit cooldowns, while preserving independent per-service schedules and backoff.
- **Safe SQLite upgrade:** Transactional schema migrations retain beta.2 policies, registrations and event history. Back up before upgrading.

Minimal service example:

```yaml
services:
  web:
    image: example/web:latest
    labels:
      com.docklantern.monitor: "true"
      # Optional:
      com.docklantern.interval: "10m"
```

Changed labels become visible **after recreating the target application service** using its normal Compose deployment process. Editing GUI intervals does **not** require restarting anything.

## Scope and safety

**Monitoring only:** DockLantern does not automatically pull images, update services, run Compose or restart monitored containers. A registry digest movement **does not prove that a newer software release is available**. Docker's image configuration digest is not automatically evidence of a deployed OCI manifest digest.

The socket proxy must be private, read-only and reject mutation methods. Do not expose port 2375. Label-based monitoring does **not** confer authorization for future managed deployment; deployment features require a separate explicit executor and source validation design.

The beta deployment runs behind **an existing HTTPS reverse proxy** with HTTP Basic authentication, origin validation and CSRF protection. Do not expose plain HTTP directly.

## Running and development

- [Deployment repository](https://github.com/docklantern/docklantern-deploy): Compose configuration, Traefik integration, backup/restore, and host prerequisites.
- [Zero-touch service enrollment](docs/user-guide/beta3-label-monitoring.md) and [live monitoring settings](docs/user-guide/beta3-live-monitor-settings.md).
- [Service-first dashboard and events](docs/user-guide/beta3-service-status.md).
- [Beta.3 SQLite backup/upgrade guidance](docs/deployment/beta3-sqlite-upgrade.md).
- [Beta.3 release plan and acceptance gates](docs/product/beta3-plan.md).
- [Development guide](docs/development/README.md), [contributing](CONTRIBUTING.md), and [security](SECURITY.md).

To run local Go tests:

```bash
go test -race ./...
go vet ./...
```

Disposable Docker integration checks require `DOCKLANTERN_INTEGRATION_TEST=1` and **must run only on a throwaway host**. The CI release gates are defined in `.github/workflows/go-poc.yaml`. **Do not execute destructive integration scripts on a production or shared VPS.**

## Roadmap

Automatic deployment, image pulls, multi-host operation and Docker event-stream discovery are **future work**, not beta.3 functionality. See the [roadmap](docs/product/roadmap.md) and [architecture decisions](docs/architecture/decisions/README.md).

Project website: [docklantern.com](https://docklantern.com). Secondary domain: [docklantern.dev](https://docklantern.dev).
