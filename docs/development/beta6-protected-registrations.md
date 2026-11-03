# Beta.6 Phase 2a — Protected Executor Registrations

**Status:** Phase 2a implementation proposed for review under [B6-02 (#82)](https://github.com/docklantern/docklantern/issues/82). **No Docker update, registration-writing CLI or host installation exists.** This is a library and CI-only trust foundation, not deployment authorization.

## Authority boundary

A monitoring enrollment, historical monitoring database `registrations.enabled`, Docker label, watched tag, observed digest or GUI proposal **cannot register an executor target**. The only production registration loader is `executorregistration.LoadProtected(registrationID)`, which reads an independently created host-administrator-owned JSON file from the fixed, **executor-private** directory:

```text
/var/lib/docklantern/executor/registrations/<opaque-registration-id>.json
```

The executor registration subtree must have a root-owned `0700` final directory and non-group/world-writable root-owned ancestors; each file must be a root-owned `0600`-equivalent regular file with **exactly one hard link**. The loader opens every directory component using directory-relative `openat` with `O_NOFOLLOW`, validates the opened descriptor with `fstat`, refuses symlinks, hardlinks, nonregular files, unsafe permissions and oversized JSON, and calculates the SHA-256 of the exact bytes read. It does **not** accept a path from the GUI/monitor or a command-line option. A loaded registration and its hash are evidence, **not** independent permission to mutate Docker.

## Versioned registration schema

Only `schema_version: 1` and `feature_policy: single-service-prebuilt-v1` are accepted. The compact, strict schema has:

- Exact opaque `registration_id` and Compose `project_name`, both restricted to safe ASCII identifiers.
- Canonical absolute `project_directory`; ordered, unique `compose_files` (1–8) and `env_files` (0–8), each contained within that project root. Explicit profile list (0–8), no duplicates.
- Explicit `trusted_source_uid` selected by the host administrator. A missing or null owner is rejected; zero means root-owned sources.
- One to eight distinct service records with explicit `deployment_enabled`, canonical public Docker Hub `approved_repository` (e.g. `docker.io/example/hello-go`, **not** `:latest` or a digest), exact `linux/amd64` platform, and optional protected `allow_running_only`. At least one service must be enabled, but **future updates still target only one approved service at a time**.
- No arbitrary Docker flags, image pulls, Compose modifications, shell arguments, secrets, host executable paths or Docker endpoint in the registration schema.

The first host-executor registration policy is intentionally **Docker Hub public images only**; GHCR, private registries, credentials and additional architectures are not implicitly accepted. Introducing them requires a separately reviewed eligibility policy.

Illustrative, *nondeployable* JSON shape:

```json
{
  "schema_version": 1,
  "registration_id": "demo-hello-go",
  "project_name": "hello-go",
  "project_directory": "/srv/docklantern-fixtures/hello-go",
  "compose_files": ["/srv/docklantern-fixtures/hello-go/docker-compose.yaml"],
  "env_files": ["/srv/docklantern-fixtures/hello-go/.env"],
  "profiles": [],
  "trusted_source_uid": 1000,
  "services": [
    {
      "name": "web",
      "deployment_enabled": true,
      "approved_repository": "docker.io/example/hello-go",
      "platform": {"os": "linux", "architecture": "amd64"},
      "allow_running_only": false
    }
  ],
  "feature_policy": "single-service-prebuilt-v1"
}
```

The parser enforces **16 KiB maximum bytes**, no duplicate object keys (including escaped duplicate spellings), no unknown fields, no trailing JSON and bounded nesting/elements. Its errors are generic and do not echo secret-bearing JSON or host paths. A service disabled in the protected registration cannot be enabled by monitoring data or proposal fields.

## Validation and limitations

The library validates registered Compose source owners and all ancestors using Phase 0's strict source policy. It rejects group-/world-writable directories, symlinks, unexpected owners and project-root escapes. The observed `hello-go` VPS `deployments/` ancestor is mode `0775`, so **that live project is intentionally ineligible** until a separate approved host-admin remediation.

**Phase 2a has no registration creation/update command.** The existing CLI `register`, `preview`, `apply`, `recover` etc. remain unavailable. The library's `LoadProtected` is not wired into the CLI or web app, and no registration files or permissions are written on any real host. A later locally authorized administrative writer/preview flow will need its own review and appropriate fsync/atomic-publication controls.

The registration file itself is opened by descriptor, but **the registered Compose source path checks are still `Lstat` preflights and are NOT race-proof snapshots**. This PR does not confirm Compose semantics, resolve OCI platform digests, verify actual Docker targets, enforce TTY authorization, journal operations, or enable Docker writes. Those are B6-02b/2c and B6-03/04/05/07/08 hard gates.

## Test evidence

Standard Go unit tests and fuzz seeds exercise strict schema validation, duplicate/unknown JSON keys, traversal and sibling-root path attacks, unsafe repository/platform/service choices, and simulated untrusted source ownership. A **separately guarded root-owned disposable CI fixture** opens an isolated private registration and verifies rejection of symlinks, hardlinks, group-readable files, group-writable directories, malformed bytes and ID traversal.

The regular CI unit/race/vet and existing security workflows remain required. Nothing is installed, published, merged automatically, or executed on the test VPS.
