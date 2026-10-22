# ADR 0001 — Docker Compose First

**Status:** Accepted  
**Date:** 2026-10-21

## Context
A lightweight Watchtower successor should work across multiple applications on a small VPS, often using Traefik.

## Decision
Use Docker Compose projects/services as primary deployment identities, initially on one local Docker Engine. Discover standalone Docker containers where practical, but prioritize Compose-aware updates.

## Consequences
Preserve Compose semantics and existing CLI/CI pipelines. Multi-host, Swarm and Kubernetes are out of the initial scope.
