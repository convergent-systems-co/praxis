# ADR-062: Provenance-Bound Work Relationships and Readiness

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-060, ADR-061, ADR-059

## Context

The dogfood controller must choose an unblocked work unit from durable Goal and
roadmap state. Roadmaps commonly describe several different relationships:
hard prerequisites, consumers, interactions, and advisory or future-learning
inputs. Treating all of them as dependencies lets model reasoning silently
turn an observation into blocking authority.

## Decision

Praxis SHALL represent work relationships as typed, provenance-bound edges.
Only a `hard_dependency` edge from an authoritative ADR, SPEC, PLAN, governed
contract, or issue may block readiness. `consumer`, `interaction`, and
`advisory` edges remain useful planning context and SHALL NOT block work.

A model may propose any relationship for review, but a model proposal SHALL
never be accepted as a blocking hard dependency. Promotion of a proposed edge
requires durable authoritative provenance. Every persisted edge binds its
source reference and source digest; missing or ambiguous provenance fails
closed.

The controller owns readiness evaluation and runnable-unit selection from
durable candidate records bound to provenance. Selection uses stable priority
and sequence fields; equal ready candidates fail closed as ambiguous. It may
report model proposals as unresolved context, but it must not use them to
authorize or deny deterministic work selection.

When the Goal-drive caller omits a child objective, this controller-owned
selection runs once before the worker turn. The selected candidate is recorded
in the turn ledger; selection does not create an internal loop. An explicit
authoritative child objective remains supported.

## Consequences

The roadmap can preserve nuanced dependency language without losing selection
determinism. A work item may be ready while it has consumers or interactions,
and a hard prerequisite can block only when its authoritative edge says so.
