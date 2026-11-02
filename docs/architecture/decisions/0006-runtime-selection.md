# ADR 0006 — Go Runtime for DockLantern

**Status:** Accepted  
**Date:** 2026-10-21

## Context

DockLantern is a continuously running, lightweight infrastructure agent that checks OCI registries, manages Compose deployments and serves a compact authenticated dashboard on small Linux VPS systems.

## Decision

Use **Go** for the controller, Docker/Compose integration, polling, deployment executor and web server. Prefer Go standard-library HTTP/templates and an embedded small frontend; exact UI toolkit, persistence library and Compose SDK integration details remain implementation choices.

## Rationale

A compiled Go binary supports straightforward Linux AMD64/ARM64 distribution, low-overhead concurrency, simple process packaging and strong Docker ecosystem integration. A small server-rendered web UI should not require a separate SPA and runtime.

## Consequences

- Keep the always-on monitoring web/controller in a single lightweight Go application, but isolate deployment in an **on-demand host-native Go CLI** per [ADR 0008](0008-on-demand-compose-executor.md).
- Use bounded registry-check workers; serialize host executor operations per Compose project with durable locks/journals and context timeouts.
- Define typed command/state interfaces to make privileged operations testable.
- No Go performance/footprint benchmark is claimed yet; measure with realistic Compose/registry workloads.
- Do not add Redis, Celery, Node runtime services or server databases just for scheduling/GUI functionality. A small, protected executor-owned SQLite journal is allowed by ADR 0008.
- Use a validated host Docker Compose CLI backend for beta.6 per ADR 0008; Go SDK research remains exploratory.

This decision does **not** settle the authenticated frontend, exact Compose executor backend or credentials design.
