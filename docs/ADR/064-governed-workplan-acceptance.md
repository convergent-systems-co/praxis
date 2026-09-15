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

Proposal generation may be inference-assisted, but each candidate must retain
explicit requirement provenance and proposer generation identity. Independent
review is a third record: distinct reviewer identity and generation evaluate
coverage, omitted requirements, invented scope, dependencies, granularity,
authority boundaries, and expected evidence. Review status only determines
whether the proposal is ready to be surfaced to acceptance authority; it never
accepts, attaches, or activates the plan.

The acceptance boundary must reload that exact acceptable review record and bind
the authority's explicit least-scope scope to the proposal, source baseline,
and review. A caller cannot promote a digest that is not present in the
authoritative review store.

Authority insufficiency is represented by a generic pending request rather than
chat state or a WorkPlan-specific hidden flag. The request is immutable and
deduplicated by identity/version; a separate decision binds its exact digest,
least scope, governance principal, outcome, and expiry. Acceptance may consume
an approved decision, but cannot broaden it or treat review as approval.

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
Goal-drive surfaces pending requests only after its deterministic selector finds
no authorized runnable child for the exact active baseline generation. The
structured result includes the durable request and its bound evidence; it is a
supervision projection, not an implicit notification grant or decision. A
persisted decision removes that request from the pending projection, while
expired, stale, conflicting, or insufficient decisions remain fail-closed until
the governed acceptance transition consumes a valid approval.

That consumption is owned by the GoalStore acceptance boundary. It must reload
the exact request, decision, proposal, and acceptable review; require an
approval outcome and exact Goal/baseline/proposal/review bindings; and derive
the WorkPlanAcceptance authority fields from the durable decision. Rejected,
revised, deferred, or insufficient outcomes cannot produce an accepted
WorkPlan. Repeating the same acceptance identity with the same resulting plan
is idempotent; reuse against another generation or proposal fails closed.
Successor-baseline attachment remains a separate transition.

Revocation is a separate append-only record bound to one exact decision,
request identity/version, decision identity/version, and decision digest. It
changes effective authority at a stated time without rewriting the original
decision. Acceptance and successor attachment must consult effective authority;
principal-wide or policy-wide revocation remains with those registries. An
already attached immutable baseline and completed work remain auditable and are
not rewritten by later revocation.

Revoke and consume transitions use one database transaction and a shared lock
on the exact request/source record. Commit order is the durable authority
timeline; a pre-commit crash produces neither transition, and retries reload
effective authority. In-process mutexes are not the authority mechanism.

The generic decision also carries an exact authority reference, generation
version, and generation digest. The encrypted GoalStore provides the minimal
durable generation registry for this contract: an immutable active generation
record is bound to principal, scope, provenance, and effective time, while a
separate immutable invalidation record marks that exact generation revoked or
superseded. GoalStore never treats `AuthorityDigest` or a principal name as
proof that a broader grant remains active. Decision issuance, exact revocation,
acceptance, attachment, and generation invalidation serialize on the same
generation record; the first committed transition defines durable ordering.
Missing generation validation, stale-generation replay, and legacy unbound
decisions fail closed. More capable external policy registries may implement
the same validator contract later, but are not required by this foundation.

## Consequences

Inference can improve liveness by proposing decomposition without minting
execution authority. Independent review and acceptance create a durable,
restart-readable handoff. A missing proposal or acceptance is visible as a
governance/decomposition gap rather than being silently interpreted as
completion or global blockage.
