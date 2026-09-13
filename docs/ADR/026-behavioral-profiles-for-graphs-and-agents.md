# ADR-026: Behavioral Profiles for Graphs and Agents

- Status: Draft
- Date: 2026-09-13

## Context

Catalog matching by goal alone is insufficient. Two graphs may both accomplish the same goal while differing materially in exploration, determinism, autonomy, evidence depth, human interaction, sequencing style, or environmental assumptions. Praxis needs a way to choose a useful starting prior for a user without asserting that one graph is globally best.

A profile is only useful if Praxis can distinguish what a publisher claims from what has actually been observed locally or measured across executions.

## Decision

Praxis will support machine-readable behavioral profiles for graphs, agents, and process packages.

Profiles describe execution characteristics such as:

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

### Profile evidence classes

Every profile field must identify its evidence class:

1. declared: asserted by the package author or publisher;
2. inherited: copied from an upstream ancestor and not yet locally validated;
3. observed: inferred from local execution history;
4. measured: produced by a defined evaluator or deterministic measurement;
5. confirmed: explicitly confirmed by a human with authority over the relevant scope.

Declared and inherited values are useful priors but must not be represented as measured facts.

Observed and measured values carry confidence, sample size where meaningful, evaluator/version provenance, and the context in which the observation was made.

### Matching

Catalog selection should prioritize closest evidenced fit to the current user, context, environment, and goal rather than popularity alone.

Matching must consider both goal suitability and behavioral compatibility. Praxis should not choose a behaviorally compatible graph that is weakly suited to the goal, nor a goal-compatible graph whose operating assumptions conflict materially with the user's constraints.

Profile matching is advisory. Security policy, capability grants, compatibility constraints, and explicit human choice take precedence.

### Local adaptation

Praxis may update a local behavioral profile as observed behavior changes, subject to provenance and learning governance.

Local observations update the local descendant's profile, not the upstream catalog record. A user may explicitly publish or contribute aggregate/profile evidence later under the privacy and catalog contribution rules.

Profile drift must be visible. When measured behavior materially diverges from declared or inherited behavior, Praxis records the divergence rather than silently overwriting history.

## Consequences

New users can start from a graph closer to how they actually work. Catalogs can contain multiple valid approaches to the same goal without forcing convergence. Personalized variants can diverge naturally from public seeds.

Praxis can explain why it selected a graph and distinguish publisher claims from evidence gathered in actual use.

## Non-goals

Profiles are not personas and should not become large natural-language prompt injections. Their purpose is matching, adaptation, measurement, and introspection.
