# ADR-016: Demonstrated Capabilities and Routing

- Status: Draft
- Date: 2026-09-13

## Context

Static executor advertisements describe what an executor claims it can do, but persistent agents should also accumulate evidence about what they have repeatedly demonstrated they can do well.

## Decision

Praxis 2 tracks demonstrated capabilities separately from advertised capabilities.

A demonstrated capability record includes scope, evidence count, confidence, recency, relevant context, and provenance. It may be attached to an agent, executor, graph/package, or other actor where appropriate.

Routing may consider both required promises and demonstrated performance, subject to policy. Demonstrated capability never overrides hard eligibility, authority, or security constraints.

Capability confidence may decay when evidence becomes stale or contradictory.

## Consequences

Praxis can route work based on observed competence rather than static labels alone. Persistent agents can become measurably specialized through experience without tying specialization to a particular model provider.
