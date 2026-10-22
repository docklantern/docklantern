# DockLantern Product Requirements Document

**Status:** Draft design baseline  
**Date:** 2026-10-21

## Vision

A lightweight, self-hosted, Docker Compose-first image updater with a Traefik-inspired dashboard. DockLantern complements existing Compose CLI and CI/CD deployment workflows instead of replacing them.

## Target environments

Initially, one Docker Engine on a Linux VPS hosting multiple independent Compose projects behind a reverse proxy (commonly Traefik). Other cloud VMs and home labs follow the same model. Multi-host and additional orchestration platforms are future considerations.

## Primary workflows

1. Discover Compose projects and running services from Docker metadata.
2. Explicitly register projects for deployment management after validating Compose configuration, paths and environment.
3. Monitor a selected image/tag at a per-service polling interval.
4. Check OCI image manifests using platform-appropriate digests, including private registries.
5. Select manual or opt-in automatic update policies.
6. Pull and deploy via Compose with a managed image override, preserving user source.
7. Detect external changes and allow adoption, restoration, or acknowledgement of drift.
8. View update status, logs, image history and failed checks.

## Requirements

| ID | Capability | Priority |
|---|---|---|
| DISC-01 | Discover and group containers by Compose project/service | Must |
| DISC-02 | Explicit enrollment and configuration validation before writes | Must |
| MON-01 | Live per-service polling changes persisted without restart | Must |
| MON-02 | Check tags against platform-specific registry manifests/digests | Must |
| MON-03 | Pause monitoring and trigger check-now | Must |
| REG-01 | Public Docker Hub, GHCR, generic OCI registry | Must |
| REG-02 | Private credentials, safe storage and redaction | Must |
| UPD-01 | Manual and opt-in auto deployment | Must |
| UPD-02 | Managed Compose image overrides with read-only source files | Must |
| UPD-03 | Journal previous/current image and status; constrained deployment concurrency | Must |
| STATE-01 | Track source, desired and actual state separately | Must |
| STATE-02 | Detect drift including same-tag/new-digest deployments | Must |
| STATE-03 | Manually adopt external deployments without recreate/pull | Must |
| UI-01 | Projects, services, tags, polling, history, last checks/pulls and logs | Must |
| SEC-01 | Authentication and restricted deployment executor | Must |
| GROUP-01 | Coordinate deployments for services sharing an app image | Should |
| DOCKER-01 | Discover and monitor standalone Docker containers | Should |

## Live configuration behavior

Store polling intervals, watched tags, deployment selections, and policies in updater state, not original Compose files. Watched tag and desired deployed reference are distinct; changing the watched tag does not automatically deploy an image. Initial suggested polling default: 300 seconds with a 60-second minimum, subject to testing and registry rate limiting.

## Acceptance examples

- Configure app1 to poll every 90 seconds and app2 every 120 seconds without restarting the updater or either app.
- Monitor and pull a private GHCR image without leaking authentication data to logs/UI.
- Deploy a new image while retaining original Traefik labels, networks, bind mounts and environment values.
- Detect an externally recreated `latest` container with an updated image digest; an image pull alone is not an external deployment.
- Adopt a running version as desired state without pulling, restarting, or recreating any container.
- Invalid or unavailable Compose files cause monitoring-only mode rather than guessed deployments.

## Non-goals (initial)

Kubernetes, Swarm, full container management, multi-host control plane, arbitrary Compose editing in the UI, automatic drift correction, general-purpose workflow automation, guaranteed rollback of database migrations, and extensive notification plugins.

## Open questions

Go versus Python, secure executor/file access, key management, authentication, recovery/rollback semantics, deployment groups, licensing and release tooling.
