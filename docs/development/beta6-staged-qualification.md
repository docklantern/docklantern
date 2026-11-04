# Beta.6 B6-04 — integrate staged source handoff with host qualification

Following merged PR #120, the host-root, controlling-TTY **read-only** `docklantern-executor qualify` command now binds its *entire* preflight to one staged set of trusted Compose and interpolation environment bytes. Previously, PR #119 provided the stage and independent Compose tests, but the live qualification path still normalized the mutable original file paths.

The new `stagedReadOnlyEvidence` wrapper stages registered inputs once, returns the original source fingerprint through the existing preflight coordinator, and runs **all four** Compose normalization checks through `NormalizedComposeFromHandoff`. This keeps the existing exact diff, OCI digest proof, actual Docker identity, trusted source rechecks, proposal/registration reopen, and exact v5.5.1 runtime requirement. A changed original source or modified staged input aborts qualification. The wrapper has **no fallback to direct mutable Compose normalization** if the underlying adapter cannot render staged files. The host CLI closes and removes the stage whether qualification succeeds or fails; cleanup failure denies a positive result.

Root-only CI tests exercise the complete coordinator with an injected read-only adapter, source edits during the second Compose normalization, refusal of a reader without staged support, wrong registration and unavailable staging. Existing Compose v2/v5.5.1 read-only fixture tests remain in place.

## Remaining acceptance blockers

This is **not** a Docker write authorization or final B6-04 sign-off. The strict normalized policy alone does not establish that Compose did not load an **unregistered raw YAML include or extends source before normalization**, or that a future write-capable Compose invocation sees only the staged inputs. The production stage root must be pre-provisioned securely on an approved host; absence fails closed. Exact Engine/API/Compose write compatibility, journal intent/locks, terminal APPLY confirmation, health checks, sibling invariance and crash-injected disposable-host writes still require their own gated implementation and tests.

The monitoring GUI retains GET-only Docker access. No `pull`, `up`, `down`, executor write command, VPS deployment, release publication, or PR merge is introduced.
