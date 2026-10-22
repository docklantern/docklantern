# DockLantern

**Keep your containers current, and your deployments under control.**

DockLantern is a planned lightweight, Compose-first container image update manager with a simple web dashboard, built initially for VPS deployments, cloud VMs, and home labs.

> **Project status: design / pre-implementation.** No runnable application exists yet.

## Product goals

- Discover Compose projects and explicitly register services for management.
- Monitor public and private Docker Hub, GHCR, and standards-based OCI registries.
- Change watched tags, polling intervals, and manual/automatic update policies live.
- Deploy using generated Compose image overrides without rewriting source configurations.
- Detect drift between Compose source, updater desired state, and Docker running state.
- Manually adopt externally deployed images without pulling or restarting containers.
- View checks, pulls, image versions, deployment history, and on-demand logs.
- Keep the footprint small and integrate with existing Traefik/Compose environments.

## Documentation

- [Product requirements](docs/product/prd.md)
- [Roadmap](docs/product/roadmap.md)
- [Architecture overview](docs/architecture/overview.md)
- [Security architecture](docs/architecture/security.md)
- [Architecture decisions](docs/architecture/decisions/README.md)
- [Development](docs/development/README.md)
- [User guide](docs/user-guide/README.md)
- [Contributing](CONTRIBUTING.md)

## Project links

- Primary domain: docklantern.com (reserved)
- Secondary domain: docklantern.dev (reserved)
- GitHub: https://github.com/docklantern/docklantern

The implementation language, web framework, executor arrangement, and license remain open decisions.
