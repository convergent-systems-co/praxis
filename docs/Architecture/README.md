# Praxis 2 Architecture

> Status: Draft
>
> Branch: `redesign/praxis2`
>
> Naming note: Issue #95 is deciding whether **SAGA (Self-Improving Agent Graph Architecture)** becomes the formal name of the core architecture. Until resolved, these documents use **Praxis 2 architecture** and refer to SAGA only as a candidate term.

This directory defines the architecture for the Praxis 2 redesign. The design is intentionally broader than software development. Development is the first proving domain, not the product boundary.

## Documents

- [High-Level Architecture](./high-level-architecture.md)
- [Detailed Design](./detailed-design.md)

## Diagrams

- [System Context and Planes](./system-planes.svg)
- [Goal and Agent Graph Lifecycle](./graph-lifecycle.svg)
- [Learning and Determinism Extraction](./learning-determinism-loop.svg)
- [Local-First State and Multi-Machine Reconciliation](./state-portability.svg)
- [Catalog Bootstrap and Local Adaptation](./catalog-bootstrap.svg)

## Architectural intent

Praxis 2 is a local-first adaptive agent system that:

1. understands a user's goal and current context;
2. selects, composes, creates, or adapts processes represented as graphs;
3. uses persistent graph-based agents whose state and behavior evolve over time;
4. learns from outcomes, human corrections, overrides, repeated patterns, and failures;
5. extracts deterministic structure from successful repeated inference;
6. minimizes advisory prompt/persona state where behavior can instead be encoded and enforced;
7. scopes learned behavior to the appropriate human, context, project, environment, and domain;
8. detects preference and process drift rather than permanently enforcing stale learning;
9. versions graph and agent lineage so change is explainable, reversible, and portable;
10. keeps portable state independent of any required external service;
11. permits cataloged packages to accelerate cold start without constraining local evolution;
12. keeps human authority, governance, promotion, rollback, and privacy boundaries explicit.

## Design principle

> Repeated successful inference should become durable execution structure when evidence supports doing so, while novelty, ambiguity, changing preferences, and exceptional conditions remain available to inference.

## Relationship to ADRs

The ADRs under `docs/ADR/` are normative decision records. These architecture documents synthesize those decisions into a coherent system design. Where a conflict exists, an accepted ADR takes precedence. All current Praxis 2 ADRs are drafts unless explicitly promoted.