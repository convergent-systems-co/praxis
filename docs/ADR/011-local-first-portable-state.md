# ADR-011: Local-First Portable State

- Status: Draft
- Date: 2026-09-13

## Context

Persistent agents, learned preferences, graph lineage, and memory must survive machine replacement and movement between machines without making Praxis dependent on GitHub, iCloud, a SaaS database, or another external service.

## Decision

Praxis 2 owns a canonical, versioned, portable state format stored locally by default.

Portable state includes agent identity and lineage, graph versions, learned preferences and scope, memory metadata, procedures, evaluation/promotion history, and other durable user/agent state.

Machine-local state remains separate and may include installed executors, local paths, hardware, credentials, local network properties, and machine-specific policy.

Praxis will support export/import and a transport-neutral synchronization contract. Git, GitHub, object storage, peer-to-peer transfer, filesystem copy, SSH, or future transports may move state, but none is required by the core.

Concurrent state from multiple machines must preserve provenance and support reconciliation rather than last-writer-wins replacement.

## Consequences

A user can move to another machine without losing agent developmental history. Context is not equated with machine identity, so the same machine may participate in different contexts such as work and home.
