# OCI digest monitoring — PR #8

**Status:** Read-only registry lookup prototype. No pulls, unattended deployments, or automatic registry schedules.

`go run ./cmd/docklantern-registry -image nginx:latest -platform linux/amd64`

Private repositories support Basic and Bearer challenge flow. Provide credentials only through a process-local environment (not command-line flags); use a token with **read:packages** or narrower registry-pull scope and do not commit credentials:

```sh
DOCKLANTERN_REGISTRY_USERNAME=bot \
DOCKLANTERN_REGISTRY_TOKEN="$REGISTRY_READ_TOKEN" \
go run ./cmd/docklantern-registry -image ghcr.io/example/private:latest
```

**Security:** The prototype sends supplied credentials to the *requested registry*, and if challenged, to the HTTPS bearer-auth realm advertised by that registry. Trust the registry URL before supplying credentials. Do not use this prototype with an untrusted registry hostname; a future credential resolver must scope credentials per host and explicitly allowlist cross-host token endpoints. Requests are HTTPS, manifests are capped at 8 MiB, content digests are cryptographically verified, and response bodies/errors never print registry tokens. No redirects should be followed by the production HTTP client.

**Semantics:** `index_digest` is the root multi-platform index or manifest digest; `platform_digest` is the resolved platform-specific manifest digest. For a single-platform manifest, the requested platform is **not independently verified**, since it is not encoded in the manifest itself. Registry manifest/index digests are **not** equal to Docker's container image ID. Comparing a remote tag with a previously *checked remote digest* establishes tag movement, not whether the running container needs replacement. Do not report an update as confirmed without reliable deployed identity/provenance.

Supported initial scope: Docker Hub (`nginx`, `docker.io/user/repo:tag`), GHCR (`ghcr.io/org/repo:tag`), and HTTPS OCI registries. HTTP-only registries, credential helper integrations, schema1 manifests, OCI artifacts other than images, and registry caching/rate limits are not supported.

The app stays monitoring-only. Integration with stored check history and inventory is the next step before a dashboard can display a reliable update state.
