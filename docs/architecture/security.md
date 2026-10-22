# Security Architecture — Initial Threat Model

- Docker API/socket access may grant effective host-root control.
- Read-only Compose mounts still allow reading `.env` secrets or sensitive paths.
- Dashboard access must be authenticated and authorized; avoid anonymous exposure via Traefik.
- Project registration must constrain allowable file roots, paths, and services; handle symlinks/traversal and filesystem changes after validation.
- Never expose arbitrary shell execution or arbitrary Compose flags/paths through HTTP requests.
- Private registry credentials need redaction, secure storage and rotation; choose key management explicitly.
- Treat OCI manifest data as untrusted; use timeouts, backoff, concurrency limits and input validation.
- Operation journaling must allow recovery from crashes and ambiguous deployment outcomes.
- Evaluate narrow host-side execution privileges versus running the updater with unrestricted socket access.

These are requirements, not claims that DockLantern is currently hardened or deployable.
