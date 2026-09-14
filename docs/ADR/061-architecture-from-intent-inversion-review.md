# ADR-061: Architecture-from-Intent Inversion Review

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-045, ADR-050, ADR-060

## Context

Issue-driven work and learning can accidentally infer architectural ownership
from the location of an existing implementation. That reverses the intended
direction of design: a capability may be implemented in a domain package while
its reusable mechanism belongs in a shared layer, or a domain-specific rule may
be incorrectly promoted into core because one fixture exercised it.

Implementation location is therefore evidence to review, not authority. The
review must be usable by Goals/design review and by governed learning without
allowing either model output or a counterexample fixture to promote itself.

## Decision

Praxis SHALL provide a domain-neutral, evidence-bound **architecture inversion
review**. The review evaluates a proposed ownership boundary against:

- the goal and invariant evidence that motivates the capability;
- the implementation-location evidence that describes the current placement;
- separate mechanism and policy evidence;
- explicit scope and reusable-across-scope claims; and
- counterexample evidence for a genuinely domain-specific conclusion.

The review SHALL classify a proposal as universal, domain-specific, or requiring
review. Missing provenance, implementation-only reasoning, reusable scope
without mechanism/policy separation, and unsupported domain-specific claims
require review. A domain-specific counterexample may justify retaining a rule
outside core, but only as an evidence-backed classification; it does not grant
execution, promotion, registration, or policy authority.

The review result is advisory evidence. Goals, learning, package, and execution
owners remain responsible for their own authoritative decisions. A worker,
fixture, caller assertion, or review result cannot mint approval, readiness,
promotion, or execution authority.

Learning consumers SHALL be able to record the review evidence beside a blind
candidate without feeding the candidate's conclusion back into the blind
derivation step. Historical evidence remains immutable and new evidence is
content-addressed by its caller.

## Consequences

Universal mechanisms can be identified without hard-coding a particular
handoff or package as the answer. Domain-specific behavior remains valid when
its scope and counterexample evidence support that conclusion. Ambiguous or
under-evidenced ownership is surfaced for architectural review instead of
being silently normalized from repository topology.
