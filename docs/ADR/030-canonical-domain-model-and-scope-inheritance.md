# ADR 030: Canonical Domain Model and Scope Inheritance

**Status:** Draft

## Context

Praxis 2 requires durable identities and relationships that remain stable across graph execution, learning, memory, plugins, machines, and model providers. Without a canonical domain model, individual subsystems will invent incompatible meanings for user identity, context, goals, agents, graphs, runs, preferences, evidence, and artifacts.

Praxis must also learn differently in different circumstances. A human may prefer one workflow at work, another at home, and a project-specific exception within either context. Learned behavior therefore cannot be represented as a single flat user profile.

## Decision

Praxis SHALL use **Human -> Context -> Goal** as its primary durable organizing hierarchy.

### Durable identities

The canonical model SHALL include at least these first-class identities:

- `Human`: the person whose goals, authority, preferences, corrections, and contextual behavior Praxis serves.
- `Context`: a durable scope such as work, home, organization, project family, or another meaningful operating context.
- `Goal`: an intended outcome pursued within a context.
- `Graph`: a reusable executable or behavioral graph identity.
- `GraphVersion`: an immutable version of a graph definition.
- `Node` and `Edge`: version-owned graph structure.
- `Agent`: a persistent actor identity whose behavior may evolve.
- `AgentVersion`: an immutable behavioral/configuration generation of an agent.
- `Run`: one execution instance of a graph/goal/agent activity.
- `Plugin`: an installed extension identity independent of its process instance.
- `Capability`: a typed behavior that can be requested without naming an implementation.
- `Observation`: recorded evidence about behavior, environment, outcome, or human interaction.
- `Evidence`: validated support for a claim, decision, promotion, or outcome.
- `Preference`: a scoped, attributable and revisable learned or explicit human preference.
- `Candidate`: a proposed behavioral, graph, policy, deterministic, or agent change awaiting evaluation/promotion.
- `Decision`: an attributable governance or human decision.
- `Artifact`: a durable output or referenced external result produced or consumed by work.

### Scope inheritance

Preferences, policies, learned behavior, and other inheritable state MAY exist at multiple scopes. The initial precedence SHALL be:

`Goal > Context > Human > Catalog/default prior`

A narrower applicable scope overrides a broader scope when the two conflict.

Graph- or agent-specific state MAY introduce additional specificity beneath the applicable goal/context scope, but SHALL retain provenance to the human/context/goal lineage that caused it.

### Explicit versus learned state

Explicit human instructions and corrections SHALL be distinguishable from inferred preferences. Learned state SHALL record provenance, confidence, evidence, creation time, last validation time, scope, and version/generation information sufficient to explain why it currently applies.

A human correction SHALL outrank conflicting learned preference at the same or broader scope.

### Context is first-class

`Context` SHALL NOT be merely an arbitrary string tag attached to runs. It is a durable identity capable of owning scoped preferences, policies, graph variants, agent behavior, memory boundaries, and portability/reconciliation state.

Praxis MAY infer candidate contexts, but creating, merging, splitting, or materially reclassifying a context must remain explainable and reversible.

### Goals are outcomes, not workflows

A `Goal` describes the desired outcome and associated constraints. It SHALL NOT be synonymous with a graph. Praxis may select, compose, create, adapt, or replace graphs while pursuing the same goal.

This distinction permits learning to improve the process without silently changing the human's intended outcome.

### Graph identity versus graph version

`Graph` is a stable lineage identity. `GraphVersion` is immutable. Executions SHALL identify the exact graph version used.

Self-improvement creates candidate/new versions rather than mutating historical graph definitions in place.

### Agent identity versus agent version

`Agent` is a persistent actor identity. `AgentVersion` represents a specific behavioral generation. Learning SHALL NOT erase the ability to explain what an agent was at an earlier point in time.

Runs and decisions involving an agent SHALL be attributable to the exact agent version active at the time.

### Stable identifiers

Canonical durable entities SHALL use globally unique opaque identifiers rather than names as identity. Human-readable names are mutable attributes.

Identifiers SHALL remain valid across rename, machine migration, synchronization, and catalog-derived local divergence.

### Ownership

The domain model belongs to Praxis core contracts. Plugins MAY extend entities through namespaced typed extension data and capabilities but SHALL NOT redefine canonical identity or ownership semantics.

## Alternatives considered

### Goal-centric model

Making `Goal` the universal root simplifies execution-oriented queries but inadequately represents behavior that applies across goals and contexts. It also encourages agents/preferences to become transient goal attachments.

### Context-centric model

Making `Context` the root improves behavioral scoping but does not adequately distinguish the human whose authority and corrections span contexts.

### Flat entity graph

A fully general entity/relation graph maximizes flexibility but pushes basic precedence and ownership semantics into inference. Praxis requires deterministic rules for the most important scope relationships.

## Consequences

- Human/context/goal behavior can diverge without creating contradictory global profiles.
- Preferences and learning gain deterministic precedence rules.
- Goals can survive graph replacement and improvement.
- Agents and graphs gain stable lineage plus immutable historical generations.
- Storage, events, memory, reconciliation, and APIs now have canonical identity anchors.
- Implementations must carry scope/provenance metadata consistently rather than using convenient unscoped dictionaries.

## Follow-up

The protobuf/domain schemas SHALL encode these identities after event and persistence semantics are finalized. Storage layout is deliberately not selected by this ADR.