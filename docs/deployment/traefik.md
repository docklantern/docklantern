# PR #12 — Container packaging and Traefik HTTPS deployment

This is a **single-container monitoring-only** deployment example. It runs the existing Go dashboard and OCI registry scheduler; it does **not** run the experimental Compose update executor. No pull/update actions are exposed.

## Preconditions

- An existing Traefik v3 deployment with a `websecure` HTTPS entry point, a configured ACME certificate resolver, and a trusted external Docker network named `web`.
- A dedicated domain resolving to Traefik.
- Docker Compose v2 and a host administrator who understands that anyone with Docker socket access can control the host.

## Setup

From the repository root:

```sh
cd deploy/traefik
cp .env.example .env
# Edit .env: domain, admin username and Traefik certificate resolver.
mkdir -p secrets
chmod 700 secrets
# Supply a unique randomly generated admin password (20+ chars) as a file.
openssl rand -base64 48 > secrets/admin_password.txt
sudo chown root:root secrets/admin_password.txt
sudo chmod 600 secrets/admin_password.txt

docker compose config
docker compose up -d --build
```

The dashboard is available at `https://<your-domain>/` **only after** Traefik has issued or loaded a valid TLS certificate. Verify HTTPS is working before supplying Basic credentials in a browser.

The generated admin password is mounted as a Compose secret and is **not committed to Git**. With `cap_drop: [ALL]`, rootful Docker needs a host bind-mounted secret accessible to container UID 0 without `CAP_DAC_OVERRIDE`. On standard rootful Docker, use a root-owned `0600` file as shown; on rootless/user-namespace deployments, verify the mapped UID/GID and secret accessibility before exposing the app. Never make the secret world-readable. `/data` is a persistent named volume containing SQLite and its journal. The monitoring image embeds the dashboard HTML/CSS but intentionally **does not include Docker CLI, Compose or Buildx binaries**. Container inventory through the opt-in discovery overlay uses the read-only HTTP Docker proxy. Run any CLI-based Docker discovery outside the image on a suitably trusted host.

## Authentication and reverse-proxy trust

- Public listening is permitted only with `DOCKLANTERN_ALLOW_PROXY=1`, `DOCKLANTERN_PUBLIC_ORIGIN=https://<domain>` and `-listen 0.0.0.0:8787`. Otherwise the legacy loopback-only restriction remains.
- The HTTP app checks the exact public host plus `X-Forwarded-Proto: https`; it never derives an allowed origin from client-supplied `X-Forwarded-Host`.
- Authentication protects the dashboard, CSS, JSON APIs and management forms. Form writes additionally require HMAC-based CSRF tokens and the configured public HTTPS Origin.
- The Traefik router uses `websecure`, TLS, security headers and a basic rate-limit middleware. The container does **not** publish a host port.
- Run this only behind a **trusted** Traefik proxy on a network not reachable by untrusted workloads. Direct access to the internal port bypasses TLS; an attacker on the same Docker network can forge forwarding headers. For wider/shared Docker networks, use an additional isolated proxy network and restrict membership.
- Basic auth is still a single-admin credential, not a session-based identity provider. Set a strong random password, limit public exposure, and rotate it using secret-file update followed by service recreation. Future SSO/session and login protections are desirable.

## Docker discovery and Compose source registration

**The sample intentionally does not mount `/var/run/docker.sock`**. Its registry monitoring works without a Docker socket, but container discovery and the `scan` CLI cannot operate until explicitly enabled. The dashboard displays prior/registered data from SQLite.

Adding the Docker socket is a substantial increase in privileges even if mounted read-only. If you choose to enable discovery, evaluate a constrained Docker socket proxy or another scoped read-only mechanism and restrict Docker APIs; ordinary socket mounts offer host administrative power. Compose source mounts must preserve identical absolute paths and should be mounted read-only. Register project source roots explicitly with `-roots`; the sample does not enable project registration from the browser.

For a local, non-public dashboard, use the default loopback mode and omit public origin/proxy environment variables.

## Limitations

The example should be tested against your specific Traefik deployment before use. No managed Docker updates, restart actions, credential vault, audit trail, user roles, host-socket isolation, distributed scheduler or deployed-digest verification is implemented. The beta candidate uses a minimal patched Alpine runtime without Docker CLI plugins; the optional Docker socket-proxy image is rebuilt with Alpine security updates. Both runtime and proxy scans must pass before release.
