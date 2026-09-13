# ADR-001: Praxis 2 Purpose and Design Laws

- Status: Draft
- Date: 2026-09-13

## Context

Praxis is being redesigned as a general execution and learning substrate for persistent AI-assisted work. Software development is an important proving ground, but it is not the defining domain.

Current agent systems commonly encode too much behavior as advisory prose, repeatedly spend inference on solved procedures, couple agent identity to a model, treat preferences as global and static, and keep state in conversations or external systems.

## Decision

Praxis 2 adopts these design laws:

1. Inference is for uncertainty, novelty, synthesis, interpretation, and judgment.
2. Repeated successful inference should be converted, when evidence supports it, into deterministic structure.
3. Runtime state, authority, transitions, recovery, evidence, and completion are owned by deterministic software, not conversation history.
4. Models are executors, not persistent agent identities.
5. Persistent agents must survive model/provider replacement.
6. Human goals and process come before selecting a graph.
7. Learning is contextual: behavior may differ by human, environment, role, project, and situation.
8. Stability is empirical, not mandatory; preference and process drift must be detectable.
9. Learning must be governed through candidate evaluation, promotion, versioning, and rollback.
10. Persistence must not imply growing prompts; durable knowledge should increasingly be retrieved or compiled rather than injected wholesale.
11. Praxis is local-first and must not require an external system for canonical state.
12. Distribution and synchronization are transports, not ontology.
13. Delivery quality, correctness, security, and user alignment constrain any optimization for cost, latency, tokens, or autonomy.

## Consequences

Praxis 2 requires first-class persistent agents, versioned graphs, contextual preference learning, process discovery, deterministic extraction, local-first portable state, package/catalog distribution, and governed self-modification.

Existing implementation may be reused when it conforms to these laws, but this ADR series defines the redesign independently of prior ADRs.
