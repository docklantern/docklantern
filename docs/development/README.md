# Development Guide

DockLantern currently contains specifications only. Before coding, choose runtime, execution boundary and license, then add `src/`, `tests/`, Dockerfile, example Compose stack and appropriate CI.

Tests must use disposable Docker/Compose fixtures—not an uncontrolled production socket. Prioritize path resolution, interpolation, registry authentication failures, digest comparisons, interrupted deployments and non-destructive adoption.
