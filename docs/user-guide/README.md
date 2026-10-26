# DockLantern User Guide

DockLantern is a monitoring-only Docker Compose image observability tool. It does **not** pull, restart or redeploy managed application containers.

## Current release

- [Published beta.3: Label-based monitoring](beta3-label-monitoring.md)
- [Published beta.3: Live monitoring settings](beta3-live-monitor-settings.md)
- [Published beta.3: Legacy service-first UI reference](beta3-service-status.md) — describes the *older beta.3 interface*, not beta.4.

## Upcoming beta.4 interface

- [Beta.4: Overview, Images, Services, Activity and Settings](beta4-workspaces.md) — documentation for an **unreleased beta.4 candidate**.

The beta.4 release requires [CI/security validation and operator sign-off](../deployment/beta4-release-checklist.md) before image publication and a separately approved deployment.

## Operating and development references

- [Roadmap](../product/roadmap.md)
- [Beta.3 SQLite backup and upgrade guide](../deployment/beta3-sqlite-upgrade.md)
- [Docker registry cache and credential boundaries](../operations/beta3-registry-cache.md)
- [Separate deployment repository](https://github.com/docklantern/docklantern-deploy)
