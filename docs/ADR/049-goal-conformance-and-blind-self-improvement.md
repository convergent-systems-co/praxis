# ADR-049: Goal Conformance and Blind Self-Improvement

- Status: Draft
- Date: 2026-09-13

## Context

Plan completion is not equivalent to goal satisfaction. A plan can omit a required behavioral property and then report complete against its own incomplete denominator. Praxis requires a mechanism that compares original intent to executable evidence and can improve its own graphs when evidence demonstrates a systematic process gap.

A qualification oracle must not contaminate discovery. If the evaluator is told a known omission before evaluation, detecting it proves recall rather than independent conformance analysis.

## Decision

Praxis introduces Goal Conformance as an independent post-plan gate.

The evaluator receives only a frozen Goal Baseline/original intent and admissible implementation evidence. Known qualification omissions, expected finding labels, remediation hints, and oracle text are withheld until the evaluator's findings are frozen and content-addressed.

Conformance findings distinguish at minimum: satisfied, unsupported, contradicted, and indeterminate. A claim is satisfied only by executable or authoritative evidence appropriate to that claim; plan/spec prose alone cannot prove behavioral implementation.

After blind findings are frozen, an external oracle may score whether expected omissions were independently rediscovered. Oracle comparison cannot mutate the frozen result.

### Versioned oracle authority

Oracle evidence and oracle authority are separate. Oracle bytes are immutable
evidence for the qualification context in which they were created. Authority
over those bytes is a versioned record containing a stable oracle identity,
generation, scope/context, effective-from time, bound goal and claim-set
digests, provenance, and any supersedes relationship with an explicit reason.

A historical oracle remains authoritative for its original historical replay
context. It MUST NOT become permanent release authority when a later governed
denominator or remediation changes the qualification context. A successor
release oracle MUST bind the current denominator and release context without
rewriting predecessor bytes. Unknown, ambiguous, unbound, or mismatched oracle
authority fails closed and cannot qualify a release.

A repeated or material conformance failure may create a governed learning candidate targeting the responsible graph/process. Candidate adaptation follows ADR-012: diagnose, create a new immutable graph generation, sandbox/replay against the original goal and regression corpus, evaluate security/correctness/cost, promote only through policy, and retain rollback.

No learner may directly mutate the active graph. Goal Conformance itself is not execution authority.

### Finding persistence across child scope

A material architectural finding discovered during a child objective SHALL survive that objective's local edit boundary. The agent records the observation, authority sources, classification, affected parent claims/gates, and disposition before selecting implementation order. “Out of scope for this child objective” is a scheduling state, not a relevance or dismissal state; only parent-goal evaluation can determine genuine irrelevance.

Frozen diagnoses are immutable evidence. Later remediation creates successor evidence and re-evaluates affected and unrelated claims independently. The withheld qualification oracle remains unavailable during discovery, diagnosis, candidate generation, and remediation. Retrospective-learning inputs preserve the observation and investigation trail without embedding the generalized lesson the learner is expected to derive.

## Consequences

- 100% plan completion is insufficient without goal-conformance evidence.
- Blind qualification prevents test-answer leakage.
- Self-improvement becomes evidence-driven rather than prompt-driven.
- The same mechanism can discover unknown omissions, not merely confirm known ones.
- Promotion remains deterministic and reversible even when diagnosis/candidate generation uses inference.
