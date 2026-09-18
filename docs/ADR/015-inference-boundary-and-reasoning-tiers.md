# ADR-015: Inference Boundary and Reasoning Tiers

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 should not treat every operation as an LLM problem. Mature behavior should move away from repeated expensive reasoning while preserving inference where it adds value.

## Decision

Praxis classifies work conceptually into three reasoning tiers:

- **D0: deterministic** — known behavior executable without model judgment
- **D1: bounded inference** — known pattern requiring limited judgment or interpretation
- **D2: open inference** — novel, ambiguous, architectural, creative, or otherwise high-uncertainty work

Learning attempts to move repeated successful behavior from D2 toward D1 and from D1 toward D0 when evidence supports doing so. The reverse transition is allowed when deterministic structure becomes brittle or context changes.

Graphs declare requirements and uncertainty characteristics rather than hard-code model names. Executor selection may use different models/tools for different tiers subject to policy.

## Consequences

Praxis can concentrate expensive reasoning on genuinely difficult decisions and use cheaper models or deterministic software elsewhere. The inference boundary becomes observable and optimizable rather than implicit.
