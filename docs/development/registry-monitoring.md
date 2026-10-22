# OCI digest monitoring — PR #8

**Status:** Read-only registry lookup prototype. No pulls, unattended deployments, or automatic registry schedules.

`go run ./cmd/docklantern-registry -image nginx:latest -platform linux/amd64`

Private repositories support Basic and Bearer challenge flow. Provide credentials only through a process-local environment (not command-line flags); use a token with **read:packages** or narrower registry-pull scope and do not commit credentials:

```sh
DOCKLANTERN_REGISTRY_USERNAME=bot \
DOCKLANTERN_REGISTRY_TOKEN="$REGISTRY_READ_TOKEN" \
go run ./cmd/docklantern-registry -image ghcr.io/example/private:latest
```

**Security:** The prototype sends supplied credentials to the requested HTTPS registry and, when challenged, only to the same registry host (or Docker Hub's `auth.docker.io` token service). Arbitrary cross-host token realms are rejected. Trust the image registry before providing credentials. Tokens are not logged or persisted. A future credential resolver must provide explicit per-registry configuration and token endpoint policies. Requests are HTTPS, manifests are capped at 8 MiB, content digests are cryptographically verified, and response bodies/errors never print registry tokens. No redirects should be followed by the production HTTP client.

**Semantics:** `index_digest` is the root multi-platform index or manifest digest; `platform_digest` is the resolved platform-specific manifest digest. For a single-platform manifest, the requested platform is **not independently verified**, since it is not encoded in the manifest itself. Registry manifest/index digests are **not** equal to Docker's container image ID. Comparing a remote tag with a previously *checked remote digest* establishes tag movement, not whether the running container needs replacement. Do not report an update as confirmed without reliable deployed identity/provenance.

Supported initial scope: Docker Hub (`nginx`, `docker.io/user/repo:tag`), GHCR (`ghcr.io/org/repo:tag`), and HTTPS OCI registries. HTTP-only registries, credential helper integrations, schema1 manifests, OCI artifacts other than images, and registry caching/rate limits are not supported.

Successful checks can now be recorded in SQLite through the existing inventory command:

```sh
go run ./cmd/docklantern-inventory -mode check-image \
  -image ghcr.io/example/web:latest \
  -platform linux/amd64 -db /tmp/docklantern-inventory.db

go run ./cmd/docklantern-inventory -mode serve \
  -db /tmp/docklantern-inventory.db
curl http://127.0.0.1:8787/api/v1/registry-checks
```

The first resolution reports `first_seen`; subsequent checks report `unchanged` or `changed` relative to the **previous successful remote check**. A failed lookup does not replace the prior successful observation. This is tag-change history, not a verified deployed-image update assessment. The HTTP API remains unauthenticated and loopback-only; do not expose it publicly. Checks run only on explicit CLI invocation.

The app stays monitoring-only. A scheduler, registry-specific credential store, per-service policies, and correlation of known deployed image digests are future work.
