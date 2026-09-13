# ADR-017: Cross-Agent Knowledge Transfer

- Status: Draft
- Date: 2026-09-13

## Context

Persistent agents will learn different things through different work. Blindly copying one agent's memory into another would spread irrelevant or user-specific behavior and increase context cost.

## Decision

Praxis 2 transfers learning between agents only through generalized, scoped artifacts such as procedures, graph fragments, policies, validators, retrieval rules, or capability knowledge.

Transfer flow:

1. identify a candidate lesson from one or more agents/runs;
2. remove agent-specific and human-specific state unless intentionally scoped;
3. classify the target scope;
4. evaluate the generalized candidate;
5. publish it to an appropriate shared scope;
6. allow receiving agents to inherit or adopt it according to policy and context.

Raw episodic memory is not the default transfer unit.

## Consequences

Praxis can develop organization-level competence without collapsing distinct agents into the same personality or process. Learning may remain private, become shared within a context, or graduate to a reusable package.
