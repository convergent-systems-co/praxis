# SPEC-024: Architecture-from-Intent Inversion Review

- Status: Active
- Date: 2026-09-14
- Authority: ADR-045, ADR-061

## Purpose

Define the deterministic, advisory contract used to detect when implementation
location is being mistaken for architectural ownership in Goals/design review
and governed learning.

## Contract

An inversion review request SHALL contain a non-empty capability and proposed
owner plus non-empty, uniquely identified evidence references for the goal and
at least one governing invariant. A location reference alone is insufficient.
Each evidence reference binds an identifier, kind, and content digest; empty or
duplicate references fail closed.

The request MAY classify the capability as reusable across scopes. Such a
request SHALL include both mechanism evidence and policy evidence. A reusable
claim without that separation is `review_required`.

A request MAY classify the capability as domain-specific. Such a request SHALL
include a counterexample reference and a non-empty scope. Without those, the
result is `review_required`.

The reusable and domain-specific classifications are mutually exclusive;
asserting both is malformed and fails closed.

The evaluator produces one of:

- `universal_mechanism` — evidence supports a reusable mechanism and its policy
  boundary is separately represented;
- `domain_specific` — evidence supports retaining the behavior within the
  declared scope; or
- `review_required` — evidence is incomplete, contradictory, or insufficient
  to distinguish mechanism from domain policy.

The evaluator is advisory and has no side effects. It SHALL NOT register,
activate, promote, approve, or execute a capability. It SHALL preserve the
input evidence references and a deterministic reason list in its result.

## Failure semantics

Malformed input, missing required evidence, duplicate evidence identity, or an
unknown classification fails closed with an error. A valid but inconclusive
request returns `review_required`; callers must not reinterpret that result as
approval. A counterexample can prevent over-generalization but cannot by itself
prove universal ownership.

## Acceptance evidence

Tests SHALL demonstrate: a universal mechanism, a genuinely domain-specific
capability, implementation-location-only reasoning, missing evidence,
duplicate evidence, and the fact that every result remains advisory and does
not expose an authority-granting operation.
