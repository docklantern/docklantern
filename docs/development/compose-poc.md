# Compose Execution Proof of Concept

**Status: experimental.** Not a production updater. Go standard library + constrained Docker Compose CLI backend, pending evaluation of Compose Go SDK parity. The authoring environment lacks Docker, but GitHub Actions now runs disposable Docker Engine integration tests (see [integration suite](compose-integration.md)).

## Scope
- Discover running Compose services via Docker inspect labels.
- Explicit allowlisted registration and source-file validation (including symlink escape rejection).
- Load canonical Compose JSON using `docker compose config --format json`.
- Generate an image-only override in a private writable directory.
- Reload the merged config and reject changes to any other service field or top-level section.
- Optionally run scoped `pull SERVICE` then `up -d --no-deps --wait --wait-timeout 120 SERVICE` behind explicit unsafe-POC approval.

No arbitrary commands/paths from an HTTP dashboard; no dashboard exists in this POC.

## Tests
```sh
go test ./...
go vet ./...
```
Unit tests fake Docker operations. The [live integration suite](compose-integration.md) runs on isolated GitHub Actions runners and may also be run on a disposable local Docker host.

## Local disposable fixture workflow
From the repository root:

```sh
PROJECT_DIR="$(pwd)/tests/integration/fixtures/sample"
docker network create docklantern-test-front
docker compose -p docklantern-test -f "$PROJECT_DIR/compose.yaml" -f "$PROJECT_DIR/compose.prod.yaml" up -d
cat >/tmp/docklantern-poc.json <<JSON
{"name":"docklantern-test","project_dir":"$PROJECT_DIR","files":["$PROJECT_DIR/compose.yaml","$PROJECT_DIR/compose.prod.yaml"],"services":["web","worker"]}
JSON
go run ./cmd/docklantern -mode discover
go run ./cmd/docklantern -mode prepare -registration /tmp/docklantern-poc.json -roots "$PROJECT_DIR" -service web -image nginx:1.27-alpine -output-dir /tmp/docklantern-poc-overrides
```

If desired, test the gated `-mode apply -confirm-unsafe-poc-deploy` operation **only on a disposable fixture**, with a verified available replacement image. Cleanup containers and the test network afterward.

## Known gaps before production
No durable transaction journal or crash recovery; no image-digest pinning, complete provenance or rollback; no per-project lock; no private registry secret management; no HTTP auth/security model; no host-socket mediation; live Docker tests now cover a limited disposable fixture, but host/container path-parity testing is still pending. Compose original source and container-internal paths must map to **identical host absolute paths**. Additional Compose features (profiles, includes, env files and bind mounts) need integration checks. Read-only mounts do not eliminate privilege from Docker socket access.

This branch is meant to validate executor behavior, not to ship automatic updates.
