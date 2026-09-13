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

A repeated or material conformance failure may create a governed learning candidate targeting the responsible graph/process. Candidate adaptation follows ADR-012: diagnose, create a new immutable graph generation, sandbox/replay against the original goal and regression corpus, evaluate security/correctness/cost, promote only through policy, and retain rollback.

No learner may directly mutate the active graph. Goal Conformance itself is not execution authority.

## Consequences

- 100% plan completion is insufficient without goal-conformance evidence.
- Blind qualification prevents test-answer leakage.
- Self-improvement becomes evidence-driven rather than prompt-driven.
- The same mechanism can discover unknown omissions, not merely confirm known ones.
- Promotion remains deterministic and reversible even when diagnosis/candidate generation uses inference.
