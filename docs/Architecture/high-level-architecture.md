# Praxis 2 High-Level Architecture

> Status: Draft
>
> Scope: Praxis 2 redesign on `redesign/praxis2`
>
> Naming: Issue #95 is deciding whether the core architecture will be formally named **SAGA (Self-Improving Agent Graph Architecture)**. This document uses neutral Praxis 2 terminology until that decision is made.

## 1. Purpose

Praxis 2 is a local-first adaptive agent system for helping a human accomplish goals across domains. It is not a development-loop product. Software development is an initial proving domain because outcomes, evidence, and repeated procedures are measurable.

The system should learn how a particular human works, discover or construct processes that support the user's goals, reduce repeated inference where evidence permits deterministic execution, and remain able to adapt as the user, environment, and goals change.

The architectural objective is:

> Use inference for uncertainty and judgment; progressively encode repeated, evidenced behavior into durable execution structure.

## 2. Core concepts

### 2.1 Human

The human remains the authority boundary. Praxis learns preferences, process tendencies, correction patterns, context distinctions, and delegation boundaries without treating those observations as universal truths.

### 2.2 Goal

A goal is the desired outcome expressed by the human or derived from an explicitly authorized higher-level objective. Goals are domain-independent.

### 2.3 Context

Context scopes learning and execution. It may include user, work/home mode, project, repository, organization, environment, domain, device, available capabilities, and current constraints.

Machine identity is evidence about context, not the definition of context.

### 2.4 Goal graph

A goal graph is a versioned executable process for accomplishing a class of goals. Praxis may select, compose, create, adapt, or retire goal graphs.

### 2.5 Agent graph

An agent is a persistent graph-based actor with identity, capability history, learned behavior, memory, state, and lineage. Agent behavior should not depend solely on persona prose.

### 2.6 Evidence

Evidence captures outcomes, validations, failures, user corrections, overrides, measured performance, and other observations used to evaluate learning.

### 2.7 Learned state

Learned state contains scoped observations such as preferences, procedures, capability demonstrations, routing knowledge, and candidate deterministic rules. Every durable learned item should carry provenance, scope, confidence, recency, and lineage.

### 2.8 Deterministic structure

Deterministic structure is behavior that no longer requires repeated model choice. Examples include graph edges, hooks, validators, policies, schemas, state transitions, executable tools, code, routing rules, or thresholds.

## 3. Architectural planes

Praxis 2 separates four major planes.

### 3.1 Interaction and intent plane

Responsibilities:

- receive human goals and corrections;
- resolve current context;
- preserve human authority and explicit preference changes;
- expose explanations, lineage, and control surfaces.

### 3.2 Execution plane

Responsibilities:

- select or construct a goal graph;
- route work to persistent agents and deterministic executors;
- maintain graph state and checkpoints;
- invoke inference only at nodes that require judgment;
- collect execution evidence.

### 3.3 Learning plane

Responsibilities:

- observe executions and outcomes;
- discover repeated successful or failing patterns;
- infer scoped human preferences;
- detect preference/process drift;
- identify redundant inference;
- propose graph, agent, routing, or deterministic changes;
- produce candidate generations rather than silently mutating trusted behavior.

### 3.4 Governance plane

Responsibilities:

- define invariants and authority boundaries;
- gate promotion of learned changes;
- version agents, graphs, policies, and deterministic artifacts;
- provide rollback and demotion;
- enforce privacy and scope boundaries;
- distinguish local personalized learning from catalog-safe generalized contributions.

## 4. End-to-end goal flow

1. **Intent intake**: the human expresses a goal.
2. **Context resolution**: Praxis determines applicable user, environment, domain, project, and constraint context.
3. **Process resolution**: Praxis selects an existing graph, composes known graph fragments, adapts a graph, or constructs a candidate process for a novel goal.
4. **Execution**: deterministic nodes execute directly; inferential nodes are routed to suitable persistent agents/models/tools.
5. **Evidence capture**: results, failures, corrections, overrides, timings, token usage, and validations are recorded.
6. **Learning**: Praxis updates observations about the human, process, agents, and environment.
7. **Candidate improvement**: the learning plane proposes changes to graph structure, agent behavior, routing, preferences, or deterministic mechanisms.
8. **Governance**: changes are evaluated, promoted, rejected, or demoted under explicit policy.
9. **Lineage update**: accepted changes create a new version/generation with provenance.
10. **Future reuse**: subsequent similar goals start from the improved local state instead of rediscovering the same procedure from scratch.

## 5. Personalization model

Praxis learns behavior at multiple scopes. A preference is not globally true merely because it is strongly evidenced for one human.

Typical precedence, from narrower to broader:

1. explicit current instruction;
2. current task/session override;
3. project-specific preference;
4. context-specific preference such as work or home;
5. domain-specific preference;
6. user-wide preference;
7. organization policy;
8. package/catalog defaults.

Invariants and safety/governance constraints are not ordinary preferences and cannot be personalized away unless the governing authority explicitly permits it.

## 6. Adaptation and drift

Stable behavior may become deterministic, but deterministic does not mean permanent.

Praxis must detect contradictory recent behavior, explicit human correction, changed environment, degraded outcomes, or recurring exceptions. These signals can reduce confidence, create a new contextual variant, demote deterministic behavior back to inference, or produce a successor graph generation.

The objective is not forced convergence. Stability is an empirical outcome when evidence supports it.

## 7. Local-first durable state

Portable Praxis state is owned by Praxis rather than by a transport provider.

Portable state includes:

- user/context profiles and preference history;
- agent identity, lineage, generations, and demonstrated capabilities;
- graph definitions, fragments, versions, and promotion state;
- learned procedures and deterministic artifacts;
- provenance and evidence summaries;
- catalog package lineage;
- governance metadata needed for safe reconstruction.

Machine-local state includes:

- credentials and secrets;
- host-specific paths;
- installed executors and models;
- hardware capabilities;
- local network facts;
- device-specific policy or caches.

Sync/transport is replaceable. Possible transports may include encrypted archives, peer-to-peer transfer, SSH, Git, user-controlled object storage, or future mechanisms. Praxis must continue functioning without a mandatory external service.

## 8. Multi-machine reconciliation

Concurrent learning from multiple machines is normal. Reconciliation must distinguish:

- independent compatible learning;
- context-specific learning that should coexist;
- conflicting updates to the same scoped fact;
- graph descendants that require merge, selection, or parallel lineage;
- stale state versus genuinely changed preferences.

Reconciliation should preserve provenance rather than flattening updates into last-writer-wins state.

## 9. Catalog bootstrap

A catalog can reduce cold-start cost by distributing generalized Praxis packages containing combinations of:

- graph seeds;
- agent seeds;
- graph fragments;
- deterministic hooks/validators;
- preference contracts;
- evaluators;
- capability vocabularies;
- domain overlays;
- behavioral profiles.

Catalog artifacts are starting priors, not immutable best practices. Local descendants evolve independently and are private by default. Generalized contributions back to a catalog require de-identification, scope review, evidence, and explicit contribution policy.

GitHub may be a useful catalog transport but must not become a required state dependency.

## 10. Graph behavioral profile

Catalog graph matching should consider more than goal labels. Graphs should expose a behavioral profile such as:

- architecture-first vs prototype-first;
- exploration tolerance;
- determinism level;
- autonomy level;
- human checkpoint density;
- evidence depth;
- parallelism strategy;
- risk posture;
- compatible isolation/runtime strategies;
- expected capabilities and environment.

Matching should optimize for evidenced fit, not popularity.

## 11. Preference contract

User-focused graph/agent packages may declare preferences that materially affect execution.

Preference slots are classified as:

- **required setup**: execution cannot be correct without the value;
- **optional preference**: a safe default exists;
- **learnable preference**: setup can be skipped and Praxis may infer it over time.

Install-time answers seed local state and remain mutable. Explicit human changes override historical inference and should immediately trigger re-evaluation of affected routing or graph choices.

## 12. Inference boundary

Praxis should not repeatedly spend model tokens deciding questions that have already been sufficiently established for the current scope.

Inference remains appropriate for:

- novel goals;
- ambiguous intent;
- new environments;
- exceptions to known graphs;
- creative generation;
- architectural tradeoffs requiring judgment;
- preference uncertainty or drift;
- candidate process discovery.

Deterministic execution is preferred for:

- stable ordering constraints;
- repeatable validation;
- known tool invocation;
- policy enforcement;
- schema validation;
- deterministic transformations;
- stable routing decisions;
- proven recurring procedures.

## 13. Success measures

Praxis 2 should be evaluated on delivery quality, not token minimization alone.

Key measures include:

- goal completion rate;
- rework and correction rate;
- repeated-method exploration eliminated;
- unnecessary inference calls avoided;
- tokens per successful outcome;
- deterministic coverage of stable procedures;
- regression rate after learning changes;
- preference-friction incidents;
- recovery/rollback effectiveness;
- time to useful cold-start behavior;
- explanation and lineage completeness.

A cheaper system that delivers worse results is not successful. The target is higher-quality, more consistent delivery with inference concentrated where it adds value.

## 14. Architectural boundaries

Praxis 2 does not require:

- GitHub as a runtime dependency;
- a particular model provider;
- one canonical development methodology;
- global convergence of personalized graphs;
- all learned behavior to become deterministic;
- all goals to have permanent graphs;
- persona files as the authoritative source of agent behavior.

The architecture intentionally permits local divergence, reversible learning, contextual behavior, and multiple valid processes for the same goal class.