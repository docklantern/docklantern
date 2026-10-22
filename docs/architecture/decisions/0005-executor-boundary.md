# ADR 0005 — Deployment Executor Boundary

**Status:** Proposed  
**Date:** 2026-10-21

## Options
1. One container with read-only Compose roots, writable updater state and Docker Engine API.
2. Small host-side executor with native filesystem paths and a narrow authorized interface.

## Proposed evaluation
Prototype containerized execution for installation simplicity but preserve a clean executor abstraction. Evaluate multi-file projects, interpolation, relative bind mounts, Traefik networks, path translation and socket privilege. Do not claim the design is settled before integration tests.
