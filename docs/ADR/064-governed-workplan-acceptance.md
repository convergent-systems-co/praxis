# ADR-064: Governed WorkPlan Acceptance

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-060, ADR-062, ADR-063

## Context

ADR-063 prevents an unaccepted decomposition from becoming runnable, but it did
not define the transition from an inferred decomposition proposal to an
accepted WorkPlan. Without that boundary, accepted requirements can remain
invisible indefinitely, while a controller or model could be tempted to treat
its own proposal as execution authority.

## Decision

Decomposition is a two-record process. A `WorkPlanProposal` is advisory and may
contain model-derived candidates and relationships. A separate acceptance
authority produces a WorkPlan only after binding the exact proposal digest, the
exact Goal Baseline digest, an authoritative source reference/digest, an
independent review digest, and a distinct accepting principal. The proposer
cannot accept its own proposal. Accepted candidates must be non-model
provenance and accepted relationships must remain within the accepted set.

Acceptance may be human or policy-governed. Human acceptance is required for
ambiguous, material, or authority-sensitive decomposition unless an existing
policy explicitly grants an independent deterministic authority to decide it.
Policy acceptance still requires independent review evidence and durable
authority; model/provider output alone cannot supply it. Authenticity and
revocation of the accepting principal belong to the authoritative event/state
provider.

The Goal/plan governance boundary owns proposal review and acceptance. Goal
drive owns only materialization, readiness, selection, and bounded execution
after an accepted WorkPlan is attached to the exact immutable baseline.

The encrypted GoalStore is the restart/provider-replacement persistence owner
for proposal and acceptance records. Persistence is intentionally not
proposal generation, independent review, acceptance authority, or baseline
attachment: those operations must enter through separately governed control
plane boundaries.

After acceptance, ADR-063 owns the separate attachment transition: the
GoalStore reloads the exact source generation and accepted record, then writes
one successor baseline with explicit predecessor and source-digest lineage.
Attachment is not an in-place update and cannot be performed against a stale
or already-attached predecessor.

Requirement or authority changes create a successor baseline and invalidate
WorkPlans whose baseline digest no longer matches. Partial blockers remain
child-level readiness state; unresolved acceptance decisions are surfaced as
authority insufficiency and do not block unrelated accepted siblings.

## Consequences

Inference can improve liveness by proposing decomposition without minting
execution authority. Independent review and acceptance create a durable,
restart-readable handoff. A missing proposal or acceptance is visible as a
governance/decomposition gap rather than being silently interpreted as
completion or global blockage.
