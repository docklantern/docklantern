# Security Policy

DockLantern is not yet released. Do not treat this repository as a production-ready updater.

Docker socket access may grant effectively host-level control. Even read-only mounts of Compose directories may expose sensitive configuration and `.env` files. HTTP authorization, deployment operations and registry credentials require strict controls.

Do not disclose sensitive exploit details or secrets in public issues. A private security reporting channel should be configured before the first release.

See [security architecture](docs/architecture/security.md).
