# PR #11 — Authenticated monitoring administration

DockLantern's prototype web server now requires a single local administrator to view the dashboard, JSON API, and monitoring settings. The existing `/healthz` endpoint remains unauthenticated and contains no inventory data. The server still **refuses non-loopback bind addresses**, even with authentication configured.

## Start safely

Set a unique password of at least 20 characters in the process environment. Never pass credentials on a CLI argument:

```sh
export DOCKLANTERN_ADMIN_USERNAME=admin
export DOCKLANTERN_ADMIN_PASSWORD='use-a-long-random-secret-not-this-example'
go run ./cmd/docklantern-inventory -mode serve \
  -db /tmp/docklantern.db -roots /srv/compose \
  -listen 127.0.0.1:8787
```

Open `http://127.0.0.1:8787/` locally. The browser prompts for HTTP Basic authentication. Navigate to **Settings** to register a Compose project or set monitoring policies. You must explicitly supply administrator-approved Compose roots using `-roots` to enable browser registration. Projects are checked using the existing file/symlink allowlist validation and registered with `enabled=false`. Policies may only be configured for services in explicit project registrations.

## Security model and boundaries

- **HTTP Basic authentication**, using a process-local username and SHA-256 password verification with constant-time comparisons. Credentials are required for the HTML dashboard, stylesheets, settings pages, and JSON API. The readiness check is public.
- No credentials are stored in SQLite or exposed via the dashboard. Credentials are configured via environment variables. Protect process environments, logs, and deployment secrets.
- Browser form mutations require a server-generated HMAC CSRF token, same-origin validation for supplied Origin headers, and reject cross-site fetch metadata; data writes are limited to monitoring registration and policies.
- Form content is bounded to 32 KiB, values render through auto-escaping Go templates, and a strict CSP disallows external scripts.
- Paths must be absolute and reside under explicitly configured source roots. Project registration never authorizes execution, image pulls, or automatic deployments.
- **Do not expose this prototype directly through Traefik.** Even though Basic auth is now implemented, the CLI is loopback-only, lacks session management, login attempt throttling, recovery, full reverse-proxy/TLS trust configuration, and multiple users. Use a local SSH tunnel for remote development access instead. Browsers send Basic credentials on every request, so use HTTPS if a production reverse proxy is introduced in a later milestone.
- Browser registration modifies only the SQLite registration/policy tables; it does not modify original Compose sources or the Docker daemon.
- The monitoring scheduler remains a separately launched CLI process; changing policies doesn't start the scheduler.

## Tests

The new auth and HTTP management tests cover: credential validation, protected dashboard/API/asset access, unauthenticated readiness, missing/invalid CSRF token, rejected cross-site requests, validation of registered services, and paths escaping the configured root.

## Future work

An authenticated production-ready deployment requires session management, HTTPS/Traefik integration with documented trusted proxy configuration, rate limiting, authentication lifecycle/rotation, hardened host/socket permissions, and audited write actions. Automatic deployment remains disabled.
