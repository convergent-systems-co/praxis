# ADR-019: Stabilization, Adaptation, and Demotion

- Status: Draft
- Date: 2026-09-13

## Context

Praxis should reduce unnecessary exploration as evidence accumulates, but forced convergence would make agents rigid and frustrating when users or environments change.

## Decision

Graph and agent behavior may stabilize when evidence strongly favors a process, but stabilization is not a required terminal state.

Praxis tracks contradictory evidence, explicit human correction, context changes, and declining outcome quality. These signals may lower confidence, reopen exploration, create a context-specific variant, or demote deterministic behavior back toward bounded inference.

A stable graph may therefore:

- remain unchanged for long periods;
- fork by context;
- replace one strategy with another;
- simplify by removing steps that no longer add value;
- reintroduce inference at a previously deterministic decision point.

## Consequences

Determinism is treated as evidence-backed compression of known behavior, not permanent truth. Mature systems can adapt without discarding useful stable procedures.
