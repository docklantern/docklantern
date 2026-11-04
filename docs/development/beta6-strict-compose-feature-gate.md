# Beta.6 B6-04: fail-closed Compose feature and implicit-input policy

After merged PR #119, the read-only preflight applies a positive allowlist to the **entire normalized Compose project**, not merely the selected service. This enforces the intentionally narrow beta.6 launch scope before considering a journaled update.

## Scope supported by the first update path

Prebuilt single-service image changes, previously provisioned **external** networks, ordinary environment variables, Traefik labels, ports, expose, command/entrypoint, approved platform, pull policy and basic container runtime/readiness configuration. Every sibling service's configuration must be unchanged, and the entire effective Compose JSON must match after substituting the selected image.

The feature gate rejects project-level include/configs/secrets, undeclared extensions, implicit or newly managed networks and volumes, service-level env_file/configs/secrets/build/dependencies/privileged access, and all service volume mounts (including sibling mounts). Unknown normalized service keys are denied rather than assumed safe. These are **beta.6 scope cuts**, not permanent Docker limitations. Existing services with stateful mounts should not be enrolled in the initial writable rollout.

## Detect hidden service env_file inputs

`HostReadOnlyAdapter.NormalizedCompose` now calls `docker compose config --no-env-resolution --format json`. The `--no-env-resolution` flag preserves a service's `env_file` reference for the independent feature gate, rather than silently merging contents into `environment` where its extra filesystem dependency would be invisible to a normalized-config comparison.

Both GitHub Actions real Compose v2 and pinned official v5.5.1 fixtures exercise this behavior: after successful source staging/image-only comparison, an unregistered `env_file` is injected into the disposable project and **must** be rejected.

## Critical remaining gates

This remains **read-only**. Config normalization may erase provenance for other Compose constructs (notably `include`, `extends`, interpolation sources or nested project file discovery). Before enabling Docker mutation, we must either independently verify that only registered pinned inputs can be accessed or reject such projects by a stronger source-level policy. The exact Docker Engine/API write runtime is not yet qualified; `apply` and `recover` remain disabled pending explicit operator approval, durable journal/project lock, stable handoff held through the side-effect subprocess, targeted-service verification, and disposable-host fault injection. Test VPS and releases require separate approvals.

The added code does not run `pull`, `up`, `down`, or any Docker-mutating operation. It does not modify running services or the existing beta.5 deployment.
