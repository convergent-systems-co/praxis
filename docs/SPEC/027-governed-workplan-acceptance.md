# SPEC-027: Governed WorkPlan Acceptance

- Status: Active
- Date: 2026-09-14
- Authority: ADR-064

## Proposal

A proposal SHALL identify the Goal, exact baseline generation/digest, proposer,
candidates, requirement references for every candidate, and typed relationships.
Proposal records may be model-derived and are advisory only. Their digest SHALL
be stable and content-addressed.

The current proposal foundation accepts explicitly supplied candidate records
through the Goals package and binds them to a verified immutable baseline. It
does not derive children from baseline prose, `PlanRef`, issue text, repository
shape, or provider output. Such evidence may inform an upstream proposer but
cannot replace an authoritative requirement reference.

## Independent review

A proposal review SHALL be a separate durable record bound to the exact
proposal and baseline digests. It SHALL identify a reviewer principal and
reviewer generation distinct from the proposer generation; the provider may be
the same only when those authority identities and generations remain distinct.
An acceptable review SHALL demonstrate complete referenced-requirement
coverage, no missing requirements, and no invented scope. Other outcomes are
`revision_required`, `insufficient_evidence`, or `authority_conflict`. Review
evidence is advisory for acceptance and cannot attach or activate a WorkPlan.

## Acceptance

Acceptance SHALL bind:

- the exact proposal digest;
- the exact baseline digest;
- authoritative source reference and digest;
- acceptance-record reference and digest;
- accepting principal;
- least-scope authority scope;
- independent review reference and version;
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

The production acceptance boundary SHALL reload the referenced review and
require its status to be `acceptable_for_authority_decision`, with exact
proposal and baseline bindings. A review status, model/provider result, or
caller-supplied digest alone SHALL never create acceptance authority.
Acceptance and successor-baseline attachment remain separate durable
transitions and SHALL survive restart without conversational state.

When review is acceptable but existing authority cannot decide safely, the
generic `AuthorityRequest` contract SHALL be used. It binds the exact Goal
baseline, proposal, review, requested authority and least scope, reason for
escalation, affected and transitively blocked work, unrelated runnable work,
and any recommendation/alternatives. A separate `AuthorityDecision` SHALL
bind the request digest, exact granted scope, governance principal, outcome,
authority evidence, and issue/expiry metadata. Requests are pending state, not
execution authority; decisions are structured records and cannot be minted by
natural-language model output.

Goal-drive SHALL query pending requests for the exact active Goal Baseline
generation only after deterministic readiness finds no authorized runnable
child. It SHALL surface the structured request payload as an
authority-required control-plane result; displaying a request, recommendation,
or alternative is not a decision. A blocked or authority-waiting child SHALL
not suppress an unrelated runnable sibling. A request with a persisted
decision SHALL not be surfaced again as pending, while expired, stale,
conflicting, or insufficient decisions remain non-authorizing and fail closed.

## Lifecycle and recovery

The accepted WorkPlan SHALL be embedded in the digest-bound immutable Goal
Baseline. Goal-drive SHALL consume it only after baseline verification. A
successor baseline with changed requirements SHALL invalidate the predecessor
WorkPlan by digest mismatch; silent reuse is forbidden. Accepted state and
acceptance evidence SHALL survive provider replacement and restart through the
authoritative Goal/state provider. The production `internal/goalstore`
repository is the persistence boundary for proposal and acceptance records:
proposals are stored immutably before acceptance, and an accepted record stores
the proposal, acceptance decision, and resulting WorkPlan together. This
repository boundary does not itself create a proposal, perform independent
review, attach a WorkPlan to a Goal Baseline, or grant public invocation
authority; those are separate governed operations.

Unaccepted proposals, missing decomposition, and unresolved approval SHALL be
reported distinctly from `blocked`, `complete`, and `runnable`. Once an
accepted set exists, ADR-062/SPEC-025 alone determine child readiness and
selection; one blocked child SHALL not suppress a ready sibling.
Approval, rejection, revision, deferral, and insufficient-authority decisions
remain durable outcomes consumed only by their authority-bearing transition;
Goal-drive does not interpret them as execution authority.

## Acceptance evidence

Tests SHALL prove that a model proposal cannot self-authorize, stale baseline
input is rejected, proposer and accepter must be distinct, accepted plans bind
proposal/baseline/review digests, and accepted WorkPlans survive canonical
baseline round-trip without changing identity.
