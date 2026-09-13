# ADR-003: Persistent Agent Identity

- Status: Draft
- Date: 2026-09-13

## Context

An agent cannot be equivalent to a prompt plus a model if it is expected to learn, adapt, move between machines, and survive provider replacement.

## Decision

Praxis 2 agents are persistent versioned entities. An agent record includes at minimum:

- stable agent identifier
- role/mission metadata
- active graph references
- memory references
- learned preference/procedure references
- capability history
- policy bindings
- generation/lineage metadata
- evaluation history
- provenance for meaningful behavioral changes

Model/provider identity is not part of agent identity. Executors are selected independently according to graph requirements, policy, context, availability, cost, risk, and demonstrated capability.

An agent may change models without becoming a new agent. A materially changed agent generation remains in the same lineage unless explicitly forked.

## Consequences

Prompts and personas become inputs or bootstrap artifacts, not identity stores. Agent inspection must be runtime-derived rather than generated from a self-description prompt.
