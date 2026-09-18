# ADR-024: Multi-Machine State Reconciliation

- Status: Draft
- Date: 2026-09-13

## Context

Praxis state is local-first and portable, but a user may work on multiple machines concurrently. Those machines may learn different facts under different contexts. Treating synchronization as last-writer-wins would destroy lineage and confuse concurrent learning with conflict.

## Decision

Praxis will reconcile portable state using lineage, provenance, scope, and immutable event history rather than blind file overwrite.

Each portable state mutation must include sufficient metadata to identify:

- originating identity and agent;
- generation or parent state;
- machine/session provenance;
- time;
- context scope;
- mutation type;
- supporting evidence;
- whether the mutation is commutative, conflicting, or independently scoped.

Reconciliation rules must distinguish:

- **concurrent compatible learning**, which can be merged;
- **context-separated learning**, which should coexist;
- **true conflicts**, which require deterministic resolution rules or human review;
- **stale descendants**, which must not overwrite newer lineage.

Machine identity is evidence about environment, not the canonical user context. Home/work/project context must remain separately modeled.

## Consequences

A user can move among machines without losing developmental history. Different machines can contribute learning without flattening context. External sync transports remain replaceable because reconciliation semantics belong to Praxis.

## Non-goals

This ADR does not choose a particular transport. Git, SSH, object storage, peer-to-peer transfer, or encrypted archive movement may all implement transport without defining state semantics.
