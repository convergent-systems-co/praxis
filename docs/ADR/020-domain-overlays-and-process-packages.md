# ADR-020: Domain Overlays and Process Packages

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 must support development, research, security, operations, compliance, and unknown future goal classes without embedding domain assumptions into the runtime.

## Decision

The Praxis core remains domain-neutral. Domain semantics are supplied through overlays and reusable packages.

An overlay/package may define domain vocabulary, graph templates/fragments, executor adapters, resource providers, evidence types, graders, policy extensions, goal classifiers, and preference contracts.

Overlays may extend behavior but may not bypass core state, authority, evidence, lineage, learning, promotion, or rollback rules.

Development remains the first high-value proving domain, not the ontology of Praxis.

## Consequences

Praxis can learn and create processes in domains beyond software development while preserving one execution, evidence, governance, and learning substrate.
