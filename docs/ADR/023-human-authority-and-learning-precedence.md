# ADR-023: Human Authority and Learning Precedence

- Status: Draft
- Date: 2026-09-13

## Context

Praxis learns user preferences and processes over time. Historical evidence can become strong enough that a system resists a changed preference, creating friction instead of adaptation. Explicit human correction must not be treated as one weak observation among hundreds of old observations.

## Decision

Praxis will maintain explicit precedence rules for human-originated learning signals.

For user-scoped behavior, evidence precedence is generally:

1. current explicit human instruction;
2. explicit scoped preference/configuration;
3. recent repeated behavior;
4. older repeated behavior;
5. inferred preference;
6. catalog/default prior.

Higher-precedence evidence can invalidate or rapidly demote lower-precedence learned behavior without requiring symmetric evidence counts.

Explicit instructions may still be constrained by non-personalizable invariants such as security, integrity, legal/policy requirements, and system authority boundaries.

Praxis must preserve prior preference history rather than rewriting it, so changes can be understood temporally and contextually.

## Consequences

Praxis can become deeply personalized without becoming stubborn. Strong historical confidence does not create a veto over the human. Preference changes become fast adaptations rather than long relearning periods.

## Non-goals

This ADR does not make every user statement permanent. Instructions may be one-time, project-scoped, contextual, or durable; Praxis must model scope explicitly.
