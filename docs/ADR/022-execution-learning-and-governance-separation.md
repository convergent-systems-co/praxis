# ADR-022: Separate Execution, Learning, and Governance Planes

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 must execute work, learn from outcomes, and govern self-modification. Combining these concerns into one graph or one agent creates circular authority: the mechanism proposing a change could also approve and deploy it.

## Decision

Praxis will separate three logical planes:

1. **Execution plane**: performs user goals using current promoted agents, graphs, tools, policies, and preferences.
2. **Learning plane**: observes execution, identifies repeated inference, extracts patterns, proposes graph/procedure/preference/deterministic candidates, and evaluates evidence.
3. **Governance plane**: controls authority, promotion, rollback, scope, privacy, safety, budgets, and mutation permissions.

These planes may share runtime infrastructure but must have explicit contracts and authority boundaries.

The learning plane may propose changes but cannot directly mutate promoted execution state. The governance plane decides whether a candidate is eligible for promotion. The execution plane consumes only promoted state unless explicitly running an experiment.

## Consequences

Self-improvement becomes auditable and reversible. Learning can be aggressive without granting it unrestricted production authority. Execution remains predictable because candidate behavior is not silently introduced.

## Non-goals

This decision does not require three separate services or processes. The separation is architectural and contractual first; deployment topology can remain compact.
