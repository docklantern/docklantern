# ADR 0002 — Managed Compose Overrides

**Status:** Accepted design; validation pending  
**Date:** 2026-10-21

## Context
Users need GUI-controlled image versions while source-controlled Compose files remain intact.

## Decision
Store desired image references in updater persistence; generate centrally stored image overrides per registered project. For managed deployments, preserve the original file sequence and apply the generated override last. Do not edit the user's original files or overwrite their `compose.override.yaml`.

## Consequences
Ordinary Compose commands omitting managed overrides may revert image selections; show drift and document CLI/CI integration. Watched tag changes are distinct from deployment changes. Validate host-path/environment semantics prior to implementation.
