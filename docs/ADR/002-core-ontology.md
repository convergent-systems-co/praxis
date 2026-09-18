# ADR-002: Core Ontology

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 needs domain-neutral primitives so software-development concepts do not become core architecture.

## Decision

The core ontology is:

- **Human**: the person whose goals, authority, preferences, and corrections shape execution.
- **Context**: scoped conditions such as work/home, organization, project, machine, environment, role, and current situation.
- **Goal**: an intended outcome expressed independently of implementation process.
- **Process**: a reusable or emerging way of accomplishing a goal class.
- **Graph**: a versioned executable representation of process, constraints, transitions, and bounded inference points.
- **Agent**: a persistent, versioned actor with identity, graph(s), memory, learned behavior, capability history, and lineage.
- **Executor**: a model, deterministic program, human, service, or other mechanism capable of satisfying requested promises.
- **Capability/Promise**: what work requires and what executors or agents can provide.
- **Evidence**: attributable proof about execution, outcomes, learning, or evaluation.
- **Learning**: a proposed update derived from evidence.
- **Compiled behavior**: learned behavior represented as deterministic or bounded executable structure.
- **Policy/Invariant**: constraints that bound execution and learning.
- **Package**: a portable distribution unit for graphs, agent seeds, procedures, policies, evaluators, and preference contracts.

Domain-specific concepts such as pull request, repository, deployment, incident, or research paper belong in overlays/packages rather than the core.

## Consequences

All core schemas and APIs should use this vocabulary. Domain packages may extend it but may not redefine core authority, state, evidence, or learning semantics.
