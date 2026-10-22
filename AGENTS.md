# DockLantern — Codex / AI Contribution Instructions

Read `docs/product/prd.md`, `docs/architecture/overview.md`, and the ADR index before implementation.

- Do not assume a language or framework has been approved.
- Distinguish Accepted ADRs from Proposed decisions, and record meaningful changes in ADRs.
- Docker Compose is the primary deployment model; preserve original Compose definitions as read-only.
- Keep Compose source, managed desired state, and Docker actual state distinct.
- Require explicit project registration/validation before updater-controlled deployments.
- Reconcile original Compose file order, environment interpolation, and host bind-mount semantics.
- A watched tag change is not automatically a deployment request.
- External adoption is non-destructive: never pull, restart, or recreate containers during adoption.
- No automatic drift correction by default.
- Never expose arbitrary shell commands, filesystem paths or raw Docker API functionality to dashboard users.
- Docker socket access is highly privileged; all credential and access designs must be security-reviewed.
- Keep the system suitable for small VPS deployments and avoid needless infrastructure dependencies.
- Do not deploy to actual infrastructure or use real secrets without explicit authorization.
- State test evidence and limitations; do not promise guaranteed rollback.

Source, test, and CI layout will be decided after the stack selection.
