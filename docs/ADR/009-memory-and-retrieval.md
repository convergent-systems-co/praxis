# ADR-009: Memory and Retrieval

- Status: Draft
- Date: 2026-09-13

## Context

Persistent agents need durable memory, but injecting all accumulated memory into model context would make learning progressively more expensive.

## Decision

Praxis 2 separates durable memory from active context. Memory is classified at minimum as:

- episodic: what happened
- semantic: what is currently believed true
- procedural: how a class of work is performed
- relational/contextual: learned information about a human-agent working relationship and its scope

Memory is retrieved selectively according to goal, graph node, context, provenance, confidence, and policy. Full memory injection is not the default.

Durable facts and procedures should migrate out of free-form memory when they can be represented as stronger structures such as indexes, graph elements, policies, hooks, validators, or deterministic tools.

## Consequences

Persistent memory can grow while prompt/context size remains bounded. Retrieval behavior becomes part of Praxis execution and can itself be evaluated and learned.
