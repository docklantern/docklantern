# Beta.6 Phase 4 — real read-only host adapters (first integration)

This batch follows merged PR #113 and adds a Linux-only, host-native `HostReadOnlyAdapter` for two of the three production evidence sources.

- `NormalizedCompose` invokes an absolute trusted Docker CLI with explicit local Unix socket, registered Compose source list/project/env/profile selection, sanitized child environment and a limited JSON response. For candidate comparison, it passes a generated **JSON-as-YAML image-only override through stdin**, with `-f -`; it never writes a managed override file or changes Docker resources. Unsupported fields on the selected or sibling services fail closed.
- `InspectActual` looks up exactly one container by Compose project/service labels, requires a 64-character Docker ID, inspects immutable image and running state and rechecks identity labels. The returned fingerprint derives from stable identity fields, not untrusted monitor observations.
- The executable subprocess runner supports only local `docker ps`, `docker inspect`, and `docker compose ... config --format json`; no shell, write command, inherited Docker remote environment, unbounded stdout or raw stderr exposure. The exported constructor requires root and a trusted, fixed Docker executable.

Test coverage injects a fake command runner to verify exact arguments, candidate override transport, unsupported sibling features, duplicate or malformed container evidence, wrong labels, stopped containers, and command allowlisting. **These tests do not claim real host compatibility.**

## Not ready to authorize Docker updates

`FetchOCI` remains deliberately unavailable. A subsequent integrated batch must independently authenticate the public registry and retrieve the index+manifest bytes; otherwise the coordinator from PR #113 fails closed. This implementation also does **not** pin Compose source files across an eventual write-capable `compose up`, perform stable-filename manifest handoff, bind an actual Docker Engine/Compose version allowlist, or fully inspect volumes and health. For that reason, it is not a beta.6 deployment authorization or completion of B6-04.

The read-only commands are not reachable from monitoring GUI, and no production executor apply/recovery command, VPS change, tag or release is introduced. Phase 5 remains strictly gated to separately approved, guarded disposable Docker hosts until final acceptance.
