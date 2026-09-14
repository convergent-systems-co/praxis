# SPEC-027: Governed WorkPlan Acceptance

- Status: Active
- Date: 2026-09-14
- Authority: ADR-064

## Proposal

A proposal SHALL identify the Goal, exact baseline generation/digest, proposer,
candidates, and typed relationships. Proposal records may be model-derived and
are advisory only. Their digest SHALL be stable and content-addressed.

## Acceptance

Acceptance SHALL bind:

- the exact proposal digest;
- the exact baseline digest;
- authoritative source reference and digest;
- acceptance-record reference and digest;
- accepting principal;
- independent review digest;
- acceptance mode (`human` or `policy`).

The accepting principal SHALL differ from the proposer. Accepted candidates
SHALL have non-model provenance, and every accepted child/relationship SHALL be
traceable to the proposal and accepted authority. Missing, stale, ambiguous, or
self-issued acceptance SHALL fail closed.

Human approval SHALL be required for unresolved, ambiguous, material, or
authority-sensitive decomposition unless an existing policy explicitly permits
independent deterministic acceptance. Policy acceptance SHALL not be inferred
from worker output, model confidence, or a passing test.

## Lifecycle and recovery

The accepted WorkPlan SHALL be embedded in the digest-bound immutable Goal
Baseline. Goal-drive SHALL consume it only after baseline verification. A
successor baseline with changed requirements SHALL invalidate the predecessor
WorkPlan by digest mismatch; silent reuse is forbidden. Accepted state and
acceptance evidence SHALL survive provider replacement and restart through the
authoritative Goal/state provider.

Unaccepted proposals, missing decomposition, and unresolved approval SHALL be
reported distinctly from `blocked`, `complete`, and `runnable`. Once an
accepted set exists, ADR-062/SPEC-025 alone determine child readiness and
selection; one blocked child SHALL not suppress a ready sibling.

## Acceptance evidence

Tests SHALL prove that a model proposal cannot self-authorize, stale baseline
input is rejected, proposer and accepter must be distinct, accepted plans bind
proposal/baseline/review digests, and accepted WorkPlans survive canonical
baseline round-trip without changing identity.
