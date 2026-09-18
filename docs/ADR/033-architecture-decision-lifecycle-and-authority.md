# ADR-033: Architecture Decision Lifecycle and Authority

- Status: Draft
- Date: 2026-09-13

## Context

Praxis is explicitly designed to learn, adapt, and improve its own graphs and behavior. That does not imply that the system should be able to silently ratify changes to the architecture that governs those mechanisms.

The redesign also needs a low-friction process. Requiring a human decision for every obvious architectural consequence would slow design work and create unnecessary interruption, while allowing an agent to mark its own architectural proposals as accepted would collapse proposal and governance authority into the same actor.

## Decision

Praxis 2 architecture decisions use the following lifecycle:

- **Draft**: actively being written or refined and not yet submitted for architectural ratification;
- **Proposed**: internally coherent and ready for human review;
- **Accepted**: explicitly approved by the architecture owner or delegated human authority;
- **Superseded**: replaced by a later accepted ADR;
- **Rejected**: reviewed and explicitly declined;
- **Deprecated**: still historically valid but scheduled to cease governing future implementation.

### Design autonomy

An agent working on Praxis may autonomously:

- create Draft ADRs;
- choose and document a recommendation when one option is clearly superior under existing design laws and constraints;
- record alternatives and rejected options;
- refine Draft ADRs;
- promote a Draft to Proposed when its own completeness checks pass;
- implement reversible exploratory work explicitly marked as provisional when doing so is consistent with existing Accepted decisions.

An agent must not autonomously mark its own architectural proposal as Accepted.

Acceptance is a governance act, not a confidence score. It requires an explicit human decision from the architecture owner or a separately authorized human delegate.

### Escalation threshold

During architecture work, human interruption is required only when all of the following are true:

1. the decision materially affects architecture, security, durable data semantics, compatibility, or user authority;
2. the alternatives are meaningfully difficult or costly to reverse;
3. no option clearly dominates under existing Accepted ADRs, design laws, evidence, and stated project goals.

Otherwise the agent selects the strongest recommendation, records the reasoning and alternatives in the ADR, and continues.

Reversible uncertainty should be resolved with a documented default rather than escalated.

### Conflicting ADRs

Accepted ADRs govern implementation until superseded. Draft and Proposed ADRs may describe intended future architecture but may not silently invalidate Accepted decisions.

If two Accepted ADRs conflict, the later ADR governs only when it explicitly identifies the earlier ADR as superseded or partially superseded. Accidental contradiction is treated as an architecture defect.

### Implementation before acceptance

Implementation may proceed from Draft or Proposed ADRs on `redesign/praxis2` when the work is explicitly part of the redesign and remains reversible. Such implementation does not confer acceptance on the ADR.

Irreversible migrations, destructive state changes, public compatibility commitments, or security authority expansion require the governing ADR to be Accepted first unless the architecture owner explicitly authorizes an exception.

### Machine-readable status

ADR status must remain mechanically discoverable so Praxis can distinguish governing architecture from proposals. Tooling may lint numbering, status transitions, supersession references, and implementation links, but must not infer human acceptance from code merge, test success, commit authorship, or elapsed time.

## Consequences

Praxis can perform substantial architecture work without repeatedly asking for decisions that already have a strong answer. Human attention is reserved for genuine forks and final governance review.

At the same time, a self-improving system cannot bootstrap its own governance authority merely by becoming confident in its recommendations.

## Non-goals

This ADR does not require committee review, formal voting, or heavyweight enterprise architecture process. A single explicitly authorized human may accept an ADR.

It also does not prevent Praxis from proposing that this governance model itself be changed. Such a change would require human acceptance under the current model before taking effect.
