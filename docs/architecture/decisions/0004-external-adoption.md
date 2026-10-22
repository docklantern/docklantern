# ADR 0004 — Manual External Image Adoption

**Status:** Accepted  
**Date:** 2026-10-21

## Context
Users deploy through external Compose commands or CI/CD and may deliberately roll back applications.

## Decision
Track source, desired, and actual state. Detect drift including a new image digest under an unchanged tag. Provide Adopt / Restore / Ignore, defaulting to manual choice. Adoption records actual deployed identity and updates desired state/override but never pulls, restarts or recreates a container.

## Consequences
An image pull alone is not a deployment. Remove redundant overrides where source matches. Unknown image provenance and mixed-replica states need operator intervention; automatic drift correction is disabled.
