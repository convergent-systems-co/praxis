# ADR-010: Agent Lineage, Generations, and Introspection

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 agents are persistent, adaptive systems whose behavior can change through evidence-backed learning. Without explicit lineage, versioned generations, and runtime-derived introspection, an agent can change without a durable explanation of what changed, why it changed, or how to return to a prior state.

A self-improving agent must remain inspectable even when its current behavior no longer resembles its original seed.

## Decision

Praxis will model every persistent agent as a lineage of immutable generations.

Each promoted behavioral mutation creates a new generation referencing its parent and recording at minimum:

- agent identity;
- generation identifier;
- parent generation;
- graph and procedure versions;
- memory/schema version references;
- promoted mutations and retired behavior;
- evidence and evaluation records supporting the mutation;
- effective preference/context bindings;
- capability changes;
- rollback target.

Generation history is append-only. Current state is a projection of that history, not an overwritten biography.

Praxis will expose runtime-derived introspection capable of answering questions such as:

- What has this agent become?
- What changed between generations?
- Which behaviors are learned versus seeded?
- Which deterministic mechanisms replaced prior inference?
- Which user/context preferences currently shape execution?
- What evidence supports the current strategy?
- What known weaknesses or unresolved uncertainty remain?

Introspection must be derived from persisted state, lineage, evidence, and active graph structure rather than an LLM-generated self-description alone.

## Consequences

Agents can evolve substantially without becoming opaque. Behavioral regressions can be traced and rolled back. Multi-machine state can reconcile by lineage rather than blind overwrite. Humans can inspect developmental history without relying on prompt text as the authoritative record.

## Non-goals

This ADR does not require every observation to create a generation. Candidate learning may remain unpromoted. Generations represent promoted behavioral state, not every transient execution detail.
