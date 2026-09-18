# ADR-063: Accepted Goal Work Materialization

- Status: Accepted for post-release implementation
- Date: 2026-09-14
- Related: ADR-060, ADR-062

## Context

ADR-062 defines deterministic selection from provenance-bound candidate records,
but the Goal Baseline previously had no durable representation or owner for the
accepted decomposition that supplies those records. A plan reference or model
description is not executable child authority.

## Decision

An accepted Goal decomposition SHALL be represented by an optional digest-bound
`WorkPlan` on the immutable Goal Baseline. It SHALL contain an authority
reference and digest, the exact source-baseline digest, candidate records, and
typed relationships. Candidate and relationship provenance SHALL be validated
before persistence; model proposals
cannot be materialized as runnable work. Relationships SHALL refer only to
children in the same accepted set.

Attaching an accepted WorkPlan creates a successor immutable Goal Baseline. The
successor preserves original intent and accepted requirements, records the
predecessor digest, and includes the WorkPlan whose source-baseline digest must
match the predecessor exactly. The predecessor is never mutated. The
attachment transition is the authority-bearing publication of runnable
decomposition; no conversational activation or model/provider signal is
required after the successor is durably persisted.

The Goal-drive control plane owns materialization from a verified baseline to
selector input. It SHALL NOT infer children from Goal prose, success criteria,
or a plan reference. A model may propose decomposition for review, but only a
governed baseline containing the accepted WorkPlan grants runnable authority.
An absent WorkPlan means no authoritative runnable child is available; it does
not mean that all requirements are complete or blocked.

## Consequences

Goal requirements can be preserved before decomposition, while accepted
decomposition remains restart-readable and provenance-bound. Existing selector
and parent-state semantics remain unchanged. Public ingestion and provider
execution may later populate an accepted WorkPlan, but neither is fabricated by
this control-plane bridge.
