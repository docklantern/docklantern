# PR #13 — Safer Docker discovery and first-run onboarding

This milestone adds an **optional** Docker container inventory source. DockLantern itself no longer needs direct socket access to scan containers when a restricted Docker socket proxy is configured.

## First run

Begin with the base Traefik deployment following [traefik.md](traefik.md). The base installation works without a Docker socket and displays an empty inventory until discovery is enabled.

For discovery, from `deploy/traefik` use:

```sh
docker compose -f compose.yaml -f compose.discovery.yaml config
docker compose -f compose.yaml -f compose.discovery.yaml up -d --build
```

This adds an internal `docker-discovery` network and an isolated `docker-socket-proxy` sidecar. DockLantern connects to that sidecar over `http://docker-socket-proxy:2375`, **not** via the Docker Unix socket. It scans once at startup and then every 60 seconds.

The first scan discovers containers with both `com.docker.compose.project` and `com.docker.compose.service` labels. In the dashboard, discovered but unregistered containers appear in a separate, collapsible list. They are *not* automatically enrolled for monitoring.

To register a project through Settings, its original Compose files must first be mounted read-only into DockLantern at **exactly the same absolute host paths** and the operator must supply matching `-roots`. The provided overlay intentionally does not enable this automatically: mapping arbitrary host paths requires site-specific configuration. Registering a project does not authorize deployment. Configure watched tags/platforms and polling intervals in Settings; registry monitoring then runs inside the container.

## Security boundaries

The [Tecnativa Docker socket proxy](https://github.com/Tecnativa/docker-socket-proxy) is configured to allow container endpoint reads and forbid mutation methods (`POST=0`). Everything else is disabled where supported. Pin the upstream image to an audited immutable digest before deploying on a real VPS. A mutable tag is used in this example for portability.

**This reduces—but does not eliminate—privilege and information-disclosure risk.** Docker container inspection may expose environment variables and secret-related metadata. The proxy has direct Docker socket access and should be treated as host-privileged. The helper's private network is only attached to DockLantern and the proxy; never join unrelated workloads or publish TCP port 2375. Docker socket mounts marked `:ro` are *not* read-only Docker API access by themselves.

The Go discovery client uses only `GET /containers/json` and `GET /containers/{hex-id}/json`, limits inspect count to 500, restricts HTTP redirects, enforces request timeouts and refuses to commit partial snapshots when the proxy fails. It uses the existing Compose label filtering and does not perform container updates.

## Operational notes

- **No automatic enrollment:** Docker discovery records an inventory only. Updates and pulls remain disabled.
- **Stale data:** A complete scan replaces the stored snapshot; failed scans preserve the previous inventory and are logged. Restarting the application safely repeats discovery.
- **No Docker socket available:** Keep the base stack only; its registry checker and web dashboard continue to function. No `-scan-every` is configured by default.
- **Do not expose the proxy publicly:** It uses internal unauthenticated HTTP by design. The separate dashboard still needs HTTPS Traefik and its administrator authentication.
- **First-run diagnosis:** use `docker compose logs docklantern docker-socket-proxy`; view the unregistered-container disclosure in the dashboard and inspect the JSON inventory at `/api/v1/containers` using admin credentials.

Future work: operator approval of discovered projects in the UI, stronger separation of socket proxy trust, automatic running-image digest correlation, and safer production deployment policies.
