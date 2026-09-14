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

Runnable candidates SHALL carry identity, completion state, stable priority
and sequence, source reference, source digest, and non-model provenance. The
controller SHALL select only incomplete candidates whose authoritative hard
dependencies are ready. It SHALL order ready candidates by ascending priority,
then ascending sequence, and SHALL fail closed when multiple candidates tie on
both fields. Input order, issue-number order, model proposals, or weaker
relationship kinds SHALL NOT mint selection authority.

When a Goal-drive request omits `ChildObjective`, the controller SHALL perform
exactly one such selection before invoking the worker. An explicit objective
remains eligible as caller-supplied work input; it does not authorize a second
selection or an implicit execution loop. The selected objective SHALL be
recorded in the turn ledger.

## Acceptance evidence

Tests SHALL prove that authoritative hard dependencies block and unblock work,
weaker relationships do not block, model-derived hard dependencies fail closed,
and relationships without durable provenance are rejected.
