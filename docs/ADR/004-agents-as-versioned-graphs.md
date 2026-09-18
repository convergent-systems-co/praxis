# ADR-004: Agents as Versioned Graphs

- Status: Draft
- Date: 2026-09-13

## Context

Persistent agents need explicit operational structure. If their behavior is hidden primarily in prompts, personas, or opaque model reasoning, it cannot be reliably inspected, versioned, evaluated, or improved.

## Decision

An agent may own one or more versioned internal graphs representing its operational process. These graphs define deterministic structure around bounded inference points.

Agent graphs may represent activities such as:

- receiving and classifying goals
- retrieving relevant memory
- resolving context
- assessing uncertainty
- planning or acting
- invoking tools/executors
- evaluating evidence
- escalating authority
- reflecting on execution
- proposing learning

Agent graphs are distinct from goal graphs. A goal graph represents the process for accomplishing work; an agent graph represents how a persistent agent participates in work.

Graphs are versioned artifacts. Mutations produce candidate versions and preserve lineage. Graph changes are not automatically promoted because an agent proposed them.

## Consequences

Agent behavior becomes inspectable and testable. Different agents may develop different internal graphs. The same agent may use different graph variants according to context while preserving a stable identity and lineage.
