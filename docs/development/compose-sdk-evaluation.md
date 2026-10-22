# Compose SDK vs CLI Evaluation (PR #6)

**Status: research; no executor backend replacement.** The validated Docker Compose CLI executor remains the implementation default.

## Why test the SDK

Docker Compose v5 exports a Go SDK that can load projects and run lifecycle operations, including event processors suitable for GUI progress. However the SDK is a large dependency graph and each v5 release has explicit Go toolchain requirements. The current DockLantern POC uses Go 1.23 and requires no Go modules beyond the standard library.

This evaluation uses the **v5.0.0 SDK** in a separate `tests/sdk` module because its `go.mod` specifies **Go 1.24.9** and the compatible `github.com/docker/cli v28.5.2+incompatible`. Do not mix that evaluation module into the current main application `go.mod`.

## Automated scope

`DOCKLANTERN_INTEGRATION_TEST=1 bash tests/sdk/compare.sh`:

1. Uses the existing multi-file Compose fixture (Traefik-style labels, external network, relative bind and environment interpolation).
2. Produces a normalized CLI Compose model with `docker compose config --format json`.
3. Initializes Docker CLI + Compose SDK and loads the same project via `LoadProject`.
4. Compares image references, labels, environment, bind mount and network model.
5. Loads a project scoped to the `web` service and ensures `worker` is not selected. The SDK operates on the passed project, and merely listing services in operation options may not restrict lifecycle operations.
6. Measures the **SDK probe binary size**. Note that the CLI executable bytes do not include the separately installed Compose plugin, so the values are not directly comparable as deployed image sizes.

The test **does not** call SDK `Pull` or `Up`. Successful project loading is not proof of lifecycle parity.

## Initial CI results (2026-10-22)

- All four GitHub Actions jobs passed, including the new SDK model comparison.
- SDK probe built with Compose v5.0.0 and Go 1.24.9: **39,056,846 bytes** (39.1 MB decimal).
- Host Docker CLI executable measured **43,134,496 bytes**, **excluding** its separately installed Compose plugin; these are not comparable deployed-image totals.
- Compared fields and scoped-load check passed. **SDK Pull and Up were not executed**, and no RSS/startup benchmarks were collected.
- Newer Compose v5.6.0 declares Go **1.26.8** in its module; toolchain version is a real upgrade constraint.

## Architectural recommendation

Keep **CLI executor** as the default while evaluating SDK. A production choice requires a gated SDK pull/recreate path against the same disposable fixtures, preservation of unrelated services, readiness handling, private registry credentials, and full image-size and RSS benchmarks. Avoid replacing a tested executor merely to eliminate a subprocess.

## Known gaps and risks

- SDK version v5.0.0 is not the newest v5 API. Later releases may require newer Go versions and have different Docker/Moby dependency layouts.
- The project loader may need explicit injection of the same environment file and profile context for parity beyond this fixture.
- The SDK's `ProjectLoadOptions.Services` should be the scoping mechanism; validate dependency behavior with fixtures before any mutations.
- The SDK does not remove host-path requirements or Docker socket privilege.
- No automatic deployments are enabled by this evaluation.
