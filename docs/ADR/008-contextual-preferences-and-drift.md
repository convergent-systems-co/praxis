# ADR-008: Contextual Preferences and Drift

- Status: Draft
- Date: 2026-09-13

## Context

Different humans may prefer different valid processes, and the same human may prefer different processes at home, work, by project, or over time. Treating learned preferences as global and permanent creates friction when circumstances change.

## Decision

Learned preferences are scoped records with provenance, confidence, recency, and context. Scope may include human, organization, environment, role, project, machine, goal class, or other relevant context.

Praxis distinguishes:

- invariant: must remain true
- organizational convention: shared scoped rule
- human preference: explicit or learned choice
- contextual strategy: choice that works under particular conditions

Explicit human correction has higher authority than inferred historical preference. Contradictory recent behavior lowers confidence and may trigger preference-drift evaluation.

Deterministic procedures may remain stable while the resolver selecting among them adapts.

## Consequences

Thomas's graph may legitimately differ from David's. Thomas-at-home may differ from Thomas-at-work. Stable preference is useful but never automatically immutable.
