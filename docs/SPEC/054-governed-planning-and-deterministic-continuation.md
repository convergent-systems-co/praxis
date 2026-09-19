# SPEC-054: Governed Planning and Deterministic Continuation

- Status: Accepted
- Governs: promotion of planner output and continuation across the WorkPlan
  authority boundary

## Contract

A WorkPlan proposal may carry `model_proposal` candidates and relationships.
Those records are advisory: they cannot be selected or executed. An approving
authority decision over the exact proposal and independent review promotes
only those advisory records to `plan` provenance, re-sourced to the exact
authority request digest. Existing authoritative citations retain their
original provenance. Promotion does not itself grant authority; the accepted
plan is still created only by the durable authority-decision bridge.

`goals-lifecycle --operation=continue --goal-id=<id> --goal-version=<version>`
reconstructs the planning frontier from durable state and performs every
uniquely derivable transition until it reaches one of these boundaries:

- `planning_required`: an authorized planner must supply a requirement-bound
  proposal;
- `independent_review_required`: an independent reviewer must judge a
  proposal;
- `owner_authority_required`: the installation owner must approve or reject
  an exact pending request;
- `planning_revision_required`: a non-acceptable review or owner rejection
  returns control to planning, and rejected authority is never retried
  implicitly; or
- `drivable`: an accepted WorkPlan is attached to an immutable successor Goal
  generation and Goal-drive is the next admissible transition.

After an acceptable independent review, continuation deterministically creates
the exact WorkPlan authority request. After owner approval, the same operation
promotes the approved proposal, persists the accepted WorkPlan, and attaches
it to the next numeric generation without another prompt. Restart reconstructs
the same request or successor and reports replay rather than duplicating a
mutation.

Continuation fails closed instead of choosing an implicit latest record when
there is more than one accepted plan, approved request, or independently
acceptable unrequested proposal/review pair. It never supplies planning,
review, or owner judgment and never turns a rejected request into authority.

The invocation contract change is the development successor
`praxis.package.goals@0.1.4`. This source version is not an official release or
publication.
