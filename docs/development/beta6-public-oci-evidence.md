# Beta.6 Phase 4 — anonymous Docker Hub OCI identity adapter

The host-native read-only `HostReadOnlyAdapter.FetchOCI` retrieves an explicitly registered public Docker Hub image **by immutable index digest**, then retrieves its unique linux/amd64 platform manifest by descriptor digest. Every response is content-hashed, size-bounded and checked against its requested SHA-256; the existing `VerifyOCIPlatformProof` revalidates the index/platform relationship and manifest media type, size and structure.

## Network and authorization boundary

- Only repositories matching the executor's strict `docker.io/namespace/image` allowlist are accepted. Registry and token hosts are fixed to `https://registry-1.docker.io` and `https://auth.docker.io/token`.
- HTTP GET only, no credentials, Docker config, inherited proxy settings, or caller-selected hosts. TLS uses Go's default certificate and hostname verification. Redirects are refused rather than followed.
- Docker Hub challenge must exactly identify the expected token realm, service `registry.docker.io` and repository-specific `pull` scope. Token responses and values are bounded; no token/error body is exposed publicly.
- Index/manifest content types, selected platform, digest headers (when present), response sizes, and SHA-256 actual bytes are verified. No fallback to an untrusted tag or a different registry if the digest cannot be fetched.
- DNS/TLS/IPv6 reachability, rate limiting, incomplete evidence, repository privacy, auth/registry failure or malformed bodies fail closed. No fallback to insecure HTTP or a second registry.

The CI tests are hermetic: a typed fake HTTP client asserts expected HTTPS host/path, authentication, scope and read-only method, then injects malformed realms, leaked token syntax, redirects, mismatched digests, duplicate platform descriptors, invalid child content and oversized responses.

## Remaining Phase 4 release gates

This source change wires a real network implementation but CI **does not claim live Docker Hub reachability**, complete system integration or deployment authorization. Before B6-05 can run on a host, require exact tested Engine/Compose versions, a trusted stable source handoff preserved during Compose execution, precise Compose feature compatibility and fresh actual Docker state. Real Docker/Compose and registry integration on a guarded disposable host and independent operator approval for any later test-VPS action remain required.

The monitoring process stays GET-only and never holds executor/registry credentials. No write-capable Docker CLI, apply/recovery command, executor installation, release, or VPS change is added here.
