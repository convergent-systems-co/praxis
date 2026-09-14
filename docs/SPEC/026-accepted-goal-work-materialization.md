# SPEC-026: Accepted Goal Work Materialization

- Status: Active
- Date: 2026-09-14
- Authority: ADR-063

## Contract

`GoalBaseline.WorkPlan` is optional. When present it SHALL have a non-empty
authority reference and digest, at least one candidate, unique candidate IDs,
validated non-model provenance, and relationships whose endpoints are in that
candidate set. Candidate and relationship source provenance remains required.

The materializer SHALL return only a validated WorkPlan. It SHALL reject a
missing plan, model-proposed candidates, invalid provenance, duplicate IDs, and
relationships to non-materialized children. It SHALL never parse free text,
`PlanRef`, or worker/model output as executable child authority.

The controller SHALL pass the materialized set to ADR-062 readiness evaluation;
only that evaluator may determine runnable, blocked, or complete state and select
one unit. A missing accepted WorkPlan is an authority insufficiency and SHALL
fail closed without claiming completion, blockage of all requirements, or
progress.

## Acceptance evidence

Tests SHALL prove that a baseline without WorkPlan yields no runnable children,
an accepted WorkPlan round-trips through canonical baseline persistence and
materializes its exact candidates, model proposals cannot be promoted, and
relationships outside the accepted set are rejected.
