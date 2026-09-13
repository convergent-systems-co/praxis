# ADR-035: Command, Query, and Authoritative Mutation Boundary

- Status: Draft
- Date: 2026-09-13

## Context

ADR-031 establishes a durable event log and projections. Praxis also exposes CLI, API, graph, plugin, and LLM-client integration surfaces. Without a common mutation boundary, different entry points could alter authoritative state through inconsistent paths, weakening replay, auditing, governance, and recovery.

Praxis does not need heavyweight CQRS infrastructure, but it does need an explicit semantic distinction between requests that may change authoritative state and requests that only observe it.

## Decision

Praxis adopts a lightweight Command/Query boundary.

### Commands

A Command expresses intent to change authoritative Praxis state or cause governed external effects. Commands:

1. carry command identity, actor/provenance, scope, and correlation/causation identifiers;
2. are validated against current authoritative state, policy, capabilities, and invariants;
3. execute through the Praxis application/runtime boundary;
4. append durable domain events for accepted authoritative state transitions;
5. use idempotency semantics when retry is possible;
6. never mutate projections directly.

A command may be rejected without producing a domain state-transition event, although audit/telemetry records may record the rejection.

### Queries

A Query requests information and has no authority to mutate Praxis state. Queries read projections, indexes, snapshots, or explicitly read-only external sources. A query must not silently trigger learning, preference updates, graph modification, package installation, or other authoritative mutations.

If observation itself must become durable knowledge, the observation enters through an explicit command/event path.

### External effects

External side effects are initiated only by governed command execution. Praxis records intent and outcome with correlation sufficient for recovery. Where atomicity with an external system is impossible, the implementation uses idempotency, effect records, reconciliation, or compensating behavior rather than pretending the event log and external service share a transaction.

### Surface neutrality

CLI commands, APIs, hooks, skills, plugins, graph nodes, and LLM-client adapters all map into the same command/query semantics. No integration surface receives a privileged mutation path.

## Consequences

Replay and auditing remain coherent across interfaces. Read paths can evolve independently from mutation rules. Plugins and LLM clients cannot bypass governance merely because they use a different transport.

This introduces command envelopes and application handlers, but does not require separate services, databases, or a CQRS framework.

## Non-goals

This ADR does not require event sourcing for every ephemeral runtime detail. It defines the authority boundary for durable Praxis state and governed effects.