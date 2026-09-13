# ADR-026: Behavioral Profiles for Graphs and Agents

- Status: Draft
- Date: 2026-09-13

## Context

Catalog matching by goal alone is insufficient. Two graphs may both accomplish the same goal while differing materially in exploration, determinism, autonomy, evidence depth, human interaction, sequencing style, or environmental assumptions. Praxis needs a way to choose a useful starting prior for a user without asserting that one graph is globally best.

## Decision

Praxis will support machine-readable behavioral profiles for graphs, agents, and process packages.

Profiles describe observed or declared execution characteristics such as:

- goal classes and domains;
- architecture-first versus prototype-first tendencies;
- exploration tolerance;
- determinism level;
- autonomy level;
- human checkpoint frequency;
- evidence and validation depth;
- parallelism strategy;
- workspace/environment assumptions;
- risk posture;
- known adaptation points;
- historical effectiveness metrics where available.

Behavioral profiles are descriptors, not immutable identities. Locally adapted descendants may develop profiles different from their catalog seed.

Catalog selection should prioritize closest evidenced fit to the current user, context, environment, and goal rather than popularity alone.

Praxis may update a local behavioral profile as observed behavior changes, subject to provenance and learning governance.

## Consequences

New users can start from a graph closer to how they actually work. Catalogs can contain multiple valid approaches to the same goal without forcing convergence. Personalized variants can diverge naturally from public seeds.

## Non-goals

Profiles are not personas and should not become large natural-language prompt injections. Their purpose is matching, adaptation, measurement, and introspection.
