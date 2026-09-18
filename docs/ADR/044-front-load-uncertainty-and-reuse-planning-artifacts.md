# ADR-044: Front-Load Uncertainty and Reuse Planning Artifacts

- Status: Draft
- Date: 2026-09-13

## Context

Praxis development has demonstrated a high-leverage pattern: substantial discovery and architectural variance were resolved once before implementation, then encoded into ADRs, diagrams, specifications, and a dependency-ordered master plan. Subsequent implementation became faster because execution agents did not repeatedly rediscover the same intent, constraints, boundaries, and work decomposition.

The existing `develop` proving graph includes discovery, classification, optional planning, implementation, validation, review, and repair. It does not yet explicitly model a reusable architecture/planning artifact stack or distinguish project-level uncertainty reduction from per-slice planning.

A development system that performs a large planning pass independently for every implementation slice wastes inference, increases latency, and permits architectural drift. Conversely, requiring heavyweight architecture for every small change would reproduce the overthinking problem Praxis is intended to avoid.

## Decision

Praxis SHALL support an optional but first-class **front-loaded uncertainty reduction phase** for work whose scope, novelty, architectural impact, or duration justifies it.

The development package SHALL distinguish two kinds of planning:

1. **Project planning** resolves durable uncertainty once and produces reusable project artifacts.
2. **Slice planning** consumes those artifacts and plans only unresolved local implementation details.

Project planning artifacts SHOULD include, when justified by the work:

- problem/goal statement and constraints;
- discovery evidence and repository/workspace model;
- open questions and variance/decision register;
- ADRs for durable architectural decisions;
- architecture views appropriate to the system, including high-level/container/component views and relevant sequence/data/security diagrams;
- implementation specifications with contracts, invariants, failure behavior, persistence/security/observability impact, and acceptance tests;
- a dependency-ordered master plan/work-bundle graph;
- architecture conformance mappings from decision -> specification -> implementation -> executable evidence.

These artifacts form a **Planning Baseline**. A baseline is versioned, digest-addressed, evidence-linked, and associated with the goal/workspace snapshot for which it was produced.

Implementation slices SHALL reference the applicable baseline rather than independently reconstructing project architecture.

## Progressive rigor

The development graph SHALL NOT require the full artifact stack for every task. Classification SHALL select a rigor profile based on objective signals such as ambiguity, architectural impact, security sensitivity, novelty, expected duration/scope, number of affected subsystems, persistence/schema impact, external contracts, and whether an adequate current planning baseline already exists.

Recommended profiles:

- **Direct**: narrow, low-risk, objectively testable work; discovery -> implementation -> validation.
- **Planned**: bounded work needing local design; discovery -> concise plan -> implementation.
- **Architected**: material architecture/security/cross-cutting work; discovery -> decisions/ADRs -> diagrams/views -> SPEC -> master plan -> implementation bundles.

The user MAY explicitly request more or less rigor. Policy can impose minimum rigor for protected classes of work.

## Baseline reuse and invalidation

A planning baseline is reusable only while its assumptions remain sufficiently current. Praxis SHALL track inputs such as workspace revision/fingerprint, relevant file/symbol evidence, dependency state, requirements, ADR/SPEC versions, and explicit assumptions.

Before a slice performs expensive planning, Praxis SHALL first ask deterministically:

1. Is there an applicable planning baseline?
2. Is it fresh enough for this slice?
3. Which assumptions/evidence changed?
4. Can the slice be derived from the existing master plan?

If yes, the slice consumes the baseline and performs only delta planning. If not, Praxis refreshes the smallest invalidated planning layer rather than rebuilding the entire planning stack by default.

Examples:

- source implementation changed but architecture contracts did not: refresh evidence/context only;
- SPEC changed: rederive affected bundles, not unrelated ADRs;
- architectural assumption changed: invalidate dependent SPEC/plan portions;
- new security boundary: require architecture/security reconsideration before mutation.

## Planning is compiled knowledge

Planning artifacts SHALL be treated as durable, reusable knowledge compiled from expensive discovery/reasoning, not disposable prose generated for one agent invocation.

They are not automatically authority. ADR acceptance, policy, permissions, and side-effect authorization retain their existing governance boundaries.

## Variance register

Project planning SHALL maintain an explicit variance/open-question register distinguishing:

- resolved decision with rationale;
- assumption accepted provisionally;
- reversible implementation choice;
- unresolved material fork requiring human input;
- invalidated prior assumption/decision.

Clear recommendations SHOULD be resolved by the planning graph and recorded rather than repeatedly escalated. Only material, difficult-to-reverse ambiguity without a dominant recommendation should interrupt the user, consistent with ADR-033.

## Plan derivation

The master plan SHALL be derived from accepted/current architecture and SPEC dependencies. Work bundles SHALL be generated from SPEC acceptance criteria and conformance mappings, not invented independently by each implementation agent.

Per-slice planners SHALL receive:

- the goal and slice contract;
- applicable ADR/SPEC excerpts or references;
- relevant architecture views;
- dependency/predecessor outputs;
- current Workspace Intelligence evidence/context pack;
- acceptance/security tests;
- unresolved local variance only.

They SHOULD NOT receive or regenerate unrelated project context.

## Reasoning economics

Praxis SHALL measure planning amortization, including:

- project-planning inference/token/time cost;
- repeated planning avoided;
- percentage of slices using baseline/delta planning;
- baseline invalidation rate;
- architecture drift/conformance failures;
- time to first useful implementation action;
- total reasoning cost per accepted implementation outcome.

Optimization target is not minimum planning. It is minimum **repeated uncertainty resolution** while maintaining correctness and conformance.

## Consequences

### Positive

- expensive discovery/architecture reasoning is amortized across many slices;
- implementation agents receive smaller, higher-quality context;
- fewer agents independently reinterpret system architecture;
- master-plan decomposition becomes reusable rather than repeatedly regenerated;
- architectural drift becomes detectable through baseline/conformance references;
- simple work retains a fast path.

### Costs

- planning artifacts require lifecycle/version/invalidation semantics;
- stale planning can become actively harmful if freshness is not checked;
- architected mode has meaningful upfront latency;
- the development package needs a project/slice distinction and artifact dependency graph.

## Rejected alternatives

### Always require full architecture before implementation

Rejected because it imposes heavyweight reasoning on narrow, reversible work and increases latency without proportional value.

### Let every implementation agent plan independently

Rejected because it repeatedly pays the same discovery/architecture cost and increases divergence.

### Treat plans as conversational context only

Rejected because conversation history is not a durable, versioned, portable source of implementation constraints.

### Treat planning artifacts as unquestionable authority

Rejected because artifacts can become stale or wrong; they require provenance, versioning, validation, and governance appropriate to their type.
