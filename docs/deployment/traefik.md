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
chmod 600 secrets/admin_password.txt

docker compose config
docker compose up -d --build
```

The dashboard is available at `https://<your-domain>/` **only after** Traefik has issued or loaded a valid TLS certificate. Verify HTTPS is working before supplying Basic credentials in a browser.

The generated admin password is mounted as a Compose secret and is **not committed to Git**. `/data` is a persistent named volume containing SQLite and its journal. The image embeds the dashboard HTML/CSS and includes the optional Docker CLI.

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

The example should be tested against your specific Traefik deployment before use. No managed Docker updates, restart actions, credential vault, audit trail, user roles, host-socket isolation, distributed scheduler or deployed-digest verification is implemented. The built image uses a pinned Docker CLI major version, not a full immutable digest; pin and audit exact upstream image digests for production releases.
