# Architecture Decision Records

Accepted ADRs capture agreed product architecture. Implementation validation is still required for some choices.

| ADR | Topic | Status |
|---|---|---|
| [0001](0001-compose-first.md) | Compose-first support | Accepted |
| [0002](0002-managed-overrides.md) | Generated image overrides | Accepted design; validation pending |
| [0003](0003-project-discovery.md) | Explicit discovery/registration | Accepted design; validation pending |
| [0004](0004-external-adoption.md) | Manual external deployment adoption | Accepted |
| [0005](0005-executor-boundary.md) | Containerized Go Compose executor; host helper fallback | Accepted design; validation pending |
| [0006](0006-runtime-selection.md) | Go runtime | Accepted |
| [0007](0007-label-enrollment-monitoring-only.md) | Label/GUI opt-in monitoring independent from deployment registration | Accepted for beta.3; implementation pending |

ADR 0007 refines the monitoring-only enrollment portion of ADR 0003; ADR 0003/0005 still require explicit source validation for any future managed deployment.

Create new ADRs with context, decision, alternatives, consequences and validation requirements. Supersede rather than erase past decisions.
