# ADR 0003 — Discover Then Register Projects

**Status:** Accepted design; validation pending  
**Date:** 2026-10-21

## Context
Docker metadata reveals Compose project/service labels but does not prove the original Compose files exist or match running configuration.

## Decision
Discover via Docker metadata, support configured read-only Compose roots, and explicitly register projects after validating file order, path mapping, working directory, and interpolation context. Unvalidated projects remain monitor-only.

## Consequences
Registration adds small onboarding overhead but prevents guesswork and unsafe updates. Read-only mounts still expose source secrets.
