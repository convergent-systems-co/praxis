# SPEC-025: Provenance-Bound Work Relationships and Readiness

- Status: Active
- Date: 2026-09-14
- Authority: ADR-062

## Contract

Each relationship SHALL contain distinct dependent and prerequisite identities,
one of `hard_dependency`, `consumer`, `interaction`, or `advisory`, a source
reference, source digest, and provenance class.

The provenance class SHALL be one of `adr`, `spec`, `plan`, `contract`,
`issue`, or `model_proposal`. A `model_proposal` hard dependency is malformed
and fails closed. A model proposal for a weaker relationship may be retained
as non-authoritative context but cannot affect readiness.

Readiness SHALL inspect only valid authoritative `hard_dependency` edges. It
returns `blocked` only when an authoritative prerequisite is incomplete;
consumer, interaction, and advisory edges cannot produce `blocked`.

Unknown kinds, unknown provenance, missing source identity/digest, self-edges,
and inferred hard dependencies fail closed. Completing a prerequisite changes
readiness only through the durable completion state supplied to the evaluator.

## Acceptance evidence

Tests SHALL prove that authoritative hard dependencies block and unblock work,
weaker relationships do not block, model-derived hard dependencies fail closed,
and relationships without durable provenance are rejected.
