# ADR-012: Governed Self-Modification

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 allows agents and graphs to improve over time, but direct self-mutation would make behavior difficult to trust, audit, reproduce, or recover.

## Decision

All learned behavioral changes are candidates before they become active.

A candidate may modify graph structure, routing, procedures, retrieval, preferences, thresholds, hooks, policy bindings, or other behavior. Promotion requires evaluation against relevant baselines and invariants.

At minimum, promotion considers:

- correctness and task quality
- security and policy compliance
- regressions
- human preference/alignment where applicable
- token and monetary cost
- latency
- retries and recovery burden
- human intervention rate

Optimization objectives are constrained by invariants; lower cost cannot justify reduced correctness or security unless explicitly permitted by policy.

Every promoted change is versioned and rollback-capable. Some changes may require explicit human approval according to authority policy.

## Consequences

Praxis can self-improve without granting unrestricted authority to the learner. Exploration and production behavior remain separated by evidence and promotion gates.
