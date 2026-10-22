# PR #10 — Initial monitoring dashboard

The first DockLantern web interface is a **read-only, server-rendered Go dashboard**. No frontend build chain, remote assets, JavaScript framework, update controls, or API mutations are required.

## Start locally

```sh
go run ./cmd/docklantern-inventory -mode scan -db /tmp/docklantern.db
go run ./cmd/docklantern-inventory -mode serve -db /tmp/docklantern.db -listen 127.0.0.1:8787
```

Visit **http://127.0.0.1:8787/**.

The page uses existing SQLite readers to show explicitly registered projects, their discovered containers and health, unregistered discovered containers (in a separate disclosure), monitoring policies, next checks, and last successful registry digest observations. Empty states guide operators toward local CLI registration. A manually refreshed page reads the most recent database contents.

The old JSON endpoints at `/api/v1/projects`, `/api/v1/containers`, `/api/v1/monitor-policies`, and `/api/v1/registry-checks` remain unchanged.

## Security model

**Do not expose this service to the public internet.** The HTTP server remains unauthenticated and limited to loopback by the existing CLI. Reverse-proxy exposure must wait for authenticated access controls. HTML values from Docker, Compose and registries are escaped by Go's `html/template`; a restrictive Content Security Policy blocks inline scripts, third-party assets and form posts. Dashboard routes accept GET only. No controls can pull images, change policies, mutate data or restart containers.

## Semantics

- Container inventory is based on the **last explicit `scan`**, not a live polling stream.
- Registry check status **changed** means the remote *tag digest moved* relative to a previous successful check. It does **not** prove an available update for the running container.
- Monitoring policies and registry data are read from SQLite; the existing separate `monitor` CLI process is responsible for scheduled checks.
- No Docker socket is touched by HTTP dashboard requests.
- This is a monitoring prototype rather than a production-ready authenticated management UI.

Next steps: authenticated settings flows, in-process inventory refresh with bounded rate limiting, service/image identity correlation, and UX evaluation on real representative stacks.
