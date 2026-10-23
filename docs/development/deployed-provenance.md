# PR #15 — Deployed OCI image provenance (read-only)

DockLantern can now attempt a defensible comparison between the image used by a running container and the most recently resolved remote OCI manifest. **A different digest is not proof that a release is newer, nor permission to deploy it.**

## Provenance source and checks

When inventory discovery is enabled, it still inspects Compose-labeled containers using read-only Docker Engine requests. For containers with a valid immutable local configuration ID (`sha256:<64 lowercase hex>`), discovery optionally calls `GET /images/{config-id}/json` and records only:

- The local image configuration ID already associated with the container
- `RepoDigests` (manifest references such as `nginx@sha256:...`)
- Image OS, architecture, and variant

A local image inspection result must include the **same** immutable configuration ID as the container. Failed/denied/unavailable image inspections are not treated as proof. The scan keeps the container inventory and leaves provenance empty when image inspection is unavailable. Image inspections are cached per immutable config ID within a proxy scan.

The Docker socket proxy overlay now enables read-only image endpoints (`IMAGES=1`) while retaining `POST=0` and the private internal network. This adds image metadata visibility to that already sensitive proxy; it does not expose mutation endpoints or mount the socket into DockLantern.

The local Docker CLI discovery path also attempts image inspect by immutable configuration ID.

## Assessment behavior

For a tagged running container, DockLantern can now report `current` or `different` **only** when:

1. A previously completed remote registry check contains the watched image digest.
2. The discovered image inspect result is bound to the exact container local configuration ID.
3. The inspected image platform equals the policy platform.
4. Exactly one distinct matching digest can be established from `RepoDigests` for the watched repository (ignoring digests for unrelated repositories).
5. All observed replicas establish identical digest evidence.

If metadata is missing, ambiguous, associated with another image ID, points at a different platform/repository, or replicas disagree, the assessment stays `unknown`. A pinned original `Config.Image` reference remains a separate supported evidence source. Digest comparisons cannot determine release ordering: `different` may indicate a newer, older, or retagged image.

**Do not equate Docker's local image configuration ID with a remote OCI manifest digest.** Even when both are SHA-256 values, they hash different documents.

## Persistence / migration

Existing SQLite inventory tables are migrated additively to store `repo_digests_json`, `image_os`, `image_architecture`, and `image_variant`, retaining existing rows. The inventory scanner replaces snapshots atomically as before. The HTTP dashboard and existing read-only JSON image-assessment API reflect the new evidence without adding write actions.

## Remaining limitations

- Remote image observations and local snapshots can become stale. Add explicit freshness constraints, including the times of both observations, before using the comparison for update notifications.
- Registry tags can be moved backwards and Docker `RepoDigests` metadata may contain old or multiple associations. Any contradictory evidence is treated as unknown.
- OCI multi-platform index vs platform-manifest digest semantics are important. Matching either exact remote index or selected platform digest is useful positive evidence; an unequal digest alone does **not** prove a newer version.
- Some local image stores, Docker versions, and image origins do not provide usable `RepoDigests`. Image status remains unknown in those cases.
- Container metadata access can expose sensitive image information. The socket proxy still has host-level privileges; do not publish it or join untrusted workloads to its network.
- No automatic Docker pulls, container updates, registry mutation, deployment controls or production VPS changes are introduced.
