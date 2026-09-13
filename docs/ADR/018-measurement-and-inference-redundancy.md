# ADR-018: Measurement and Inference Redundancy

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 must be able to prove that self-improvement is actually improving delivery rather than merely adding more machinery.

## Decision

Praxis records longitudinal metrics for repeated classes of goals. At minimum, evaluation may include:

- completion quality and correctness
- policy/security violations
- regressions
- token usage
- monetary cost
- latency
- retries and repair loops
- human interventions
- graph/path variance
- repeated inference spent on previously solved procedural decisions

Praxis distinguishes novel reasoning from repeated procedural reasoning where feasible. Repeated inference clusters are candidates for deterministic extraction.

The north-star condition is: repeated classes of goals should maintain or improve quality while reducing unnecessary reasoning, retries, supervision, and delivery variance.

Learning is also tested for model portability: behavior that disappears when the underlying model/provider changes is not considered fully captured by Praxis.

## Consequences

Token reduction is a measurable consequence, not the sole objective. Praxis can identify inference debt and target high-value opportunities for compilation into deterministic structure.
