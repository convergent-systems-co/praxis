# ADR-076: Governed Executor Affinity and Routing Targets

- Status: Accepted for post-release implementation
- Date: 2026-09-17
- Issue: #102
- Related: ADR-003, ADR-015, ADR-016, ADR-022, ADR-034, ADR-037, ADR-038

## Context

Praxis deliberately routes inference by demonstrated capability rather than hard-coding model vendors into graphs. That preserves portability, evidence-based selection, and persistent agent identity independent of any specific model.

That rule is necessary but incomplete for real multi-agent execution. Different agents, graph branches, and work classes can materially benefit from different execution surfaces for quality, specialization, availability, latency, context limits, subscription accounting, or token/cost management.

Praxis therefore needs a governed way to express executor affinity and hard execution constraints without turning provider names into core graph semantics.

The architecture must also distinguish execution family from transport. Codex through an authenticated subscription CLI and OpenAI through an API are not the same execution surface. Claude through an authenticated subscription CLI and Anthropic through an API are likewise distinct because authentication, metering, policy, quotas, observability, and availability differ.

## Decision

Praxis preserves **capability-first routing** while introducing a provider-neutral **Execution Target** abstraction.

An Execution Target may express semantic constraints and preferences including:

- required capabilities;
- preferred executor family/profile;
- explicit hard executor requirement;
- acceptable fallback profiles;
- reasoning/quality tier;
- transport/auth class;
- local/subscription/API preference or prohibition;
- token/context/resource budget profile;
- executor-specific concurrency/quota class;
- security/evidence requirements.

The executable contract is defined by SPEC-039.

### Capability remains the base eligibility rule

Executor affinity cannot make an otherwise ineligible executor eligible.

Conceptually:

```text
capability + security + authority + policy eligibility
                    ↓
            eligible executor set
                    ↓
       execution target / affinity
                    ↓
          budget and availability
                    ↓
          deterministic selection
```

Preference is advisory routing input, not authority.

### Hard targeting is explicit and fail-closed

A package, agent definition, graph/subgraph, node/work unit, or authorized operator may require an execution profile only through an explicit versioned contract that distinguishes `required` from `preferred` semantics.

If no eligible executor satisfies a hard requirement, execution fails closed. Praxis must not silently substitute another provider family or transport.

### Transport identity is distinct from provider/model family

Praxis must model execution-surface identity so policy can distinguish at least:

- subscription/authenticated CLI execution;
- metered API execution;
- local model execution;
- future transport classes.

Permission to use a provider/model family does not imply permission to use every transport in that family.

### Persistent agent identity is executor-independent

Agent identity, memory, lineage, preferences, and generations remain independent of executor/model identity. Re-routing an agent to another eligible executor does not create a new agent identity.

Executor selection is execution evidence, not identity authority.

### Graph and agent targeting are bounded by stronger policy

Execution-target information may exist at platform/org/user policy, package, agent, graph/subgraph, node/work-unit, runtime learned-preference, and operator-invocation levels.

The final precedence order must be explicit, deterministic, and specified. Lower-authority preferences may rank or narrow eligible choices but may not override stronger security, metering, capability, organizational, or operator prohibitions.

### Token and resource management are routing inputs, not safety overrides

Where reliable telemetry exists, Praxis may use token, context-window, rate-limit, latency, and monetary/resource budgets to choose among otherwise eligible executors.

Praxis must not fabricate token or monetary usage when an execution surface does not expose reliable telemetry.

Budget pressure may cause routing to a cheaper eligible target, deferral, continuation/handoff, or fail-closed behavior according to policy. It may never weaken security, evidence, or required-capability constraints.

### Learned routing remains governed

Observed executor performance may create candidate routing preferences, but active routing changes only through the governed learning lifecycle:

```text
observation
  -> candidate routing preference
  -> replay/evaluation
  -> correctness/security/policy gates
  -> distinct-authority promotion
  -> active routing profile
```

An agent/model may not directly rewrite its own active execution target.

### Parallel branches may use different executors

Parallel graph branches may select different eligible executor profiles concurrently. Scheduler/resource accounting must model executor-specific quotas/concurrency as resources where required.

This is a direct dependency for the first-party Develop Bundle (#101): specialized roles may target different eligible execution surfaces without hard-coding those example choices into Praxis core.

### Fallback is explicit

Praxis distinguishes:

- preferred target unavailable;
- required target unavailable;
- eligible fallback available;
- fallback prohibited by policy;
- fallback lacks capability/evidence requirements;
- budget/quota exhausted;
- API use prohibited;
- subscription surface unauthenticated.

No subscription/local execution may silently fall back to metered API use unless policy explicitly permits that transition.

### Selection is explainable evidence

Each routed inference execution must preserve enough durable evidence to explain:

- requested capabilities;
- applicable target/profile constraints;
- eligible executor set;
- selected executor/surface;
- selection reason;
- fallback reason where applicable;
- provider/runtime/transport identity;
- model identity when available;
- reasoning tier/profile;
- reliable token/resource telemetry when available;
- applicable policy/budget state;
- execution outcome.

## Consequences

### Positive

- specialized agents and graph branches can intentionally use the strongest eligible execution surface;
- subscription/local execution can be preferred over metered API use;
- token/context budgets become explicit routing inputs instead of hidden prompt behavior;
- persistent agent identity remains portable across executor changes;
- routing decisions remain explainable and replayable;
- first-party bundles can demonstrate true multi-provider parallel execution.

### Costs

- routing contracts become richer than simple capability matching;
- precedence and fallback semantics require explicit conformance tests;
- executor adapters must expose accurate transport/capability/accounting metadata;
- budget and telemetry uncertainty must be represented rather than guessed.

## Rejected Alternatives

### Raw provider/model strings directly in core graph nodes

Rejected because it couples graph semantics to vendors and weakens portability.

### Capability-only routing forever

Rejected because it cannot express legitimate specialization, subscription/API distinctions, explicit transport policy, or intentional token-budget strategy.

### Let the model choose its own provider dynamically

Rejected because executor selection affects cost, security, authority, availability, and reproducibility and therefore requires deterministic governance.

### Silent API fallback

Rejected. A subscription/local target must never silently become metered API execution unless policy explicitly permits it.

## Invariants

1. Capability, security, authority, and policy eligibility precede affinity optimization.
2. Hard target requirements fail closed.
3. Preferences cannot mint capability or authority.
4. Agent identity is independent of executor identity.
5. Provider family and transport identity are distinct concepts.
6. Metered API fallback is never implicit.
7. Budget optimization cannot weaken security/evidence requirements.
8. Missing telemetry is represented as unknown, never fabricated.
9. Learned routing changes require governed promotion.
10. Parallel branches may use different eligible executors while respecting scheduler/resource governance.
11. Selection and fallback decisions are durably explainable.
12. Praxis core remains provider-neutral; concrete provider/transport identities belong to adapters/catalog metadata and versioned profiles.

## Required Follow-up

1. Implement SPEC-039 execution-target contract and routing precedence.
2. Reconcile ADR-037 executor adapter semantics with surface identity/transport metadata.
3. Reconcile SPEC-009 inference routing/budgets with target/fallback semantics.
4. Reconcile SPEC-003 scheduler/resource governance with executor-specific quotas/concurrency.
5. Make #102 a predecessor/companion of #101 first-party multi-agent bundle implementation.
6. Surface routing evidence in #100 supervision/TUI work.
7. Preserve #37 as the adapter/executor-fabric reconciliation epic rather than duplicating adapter work here.
