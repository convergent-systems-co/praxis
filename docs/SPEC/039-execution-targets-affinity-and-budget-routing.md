# SPEC-039: Execution Targets, Affinity, and Budget-Aware Routing

- Status: Accepted
- Date: 2026-09-17
- Governing ADR: ADR-076
- Issue: #102
- Related: SPEC-003, SPEC-009, SPEC-010, SPEC-012, SPEC-013 (persistent agent identity), SPEC-016 (software-development proving package)

## Purpose

Define the executable contract that allows Praxis packages, persistent agents, graphs, subgraphs, nodes/work units, and authorized operators to constrain or prefer LLM execution surfaces while preserving provider-neutral core semantics, deterministic authority, fail-closed security, governed learning, and explainable routing.

## Terms

### Executor Surface

A concrete invokable execution surface with a stable runtime identity and metadata, for example:

- Codex authenticated CLI/subscription;
- Claude authenticated CLI/subscription;
- OpenAI API;
- Anthropic API;
- local MLX/Hugging Face execution;
- future plugin-provided surfaces.

An executor surface is not equivalent to a provider family or model family.

### Execution Target

A versioned semantic selector attached to an invocation context. It expresses requirements/preferences over eligible executor surfaces without requiring provider-specific core fields.

### Executor Profile

A cataloged semantic profile describing a class or named execution surface using properties such as capabilities, family, transport class, auth/accounting class, reasoning tier support, context limits, telemetry support, and policy labels.

### Hard Requirement

A constraint that must be satisfied or execution fails closed.

### Affinity

A preference used to rank otherwise eligible executor surfaces. Affinity cannot override hard requirements or stronger policy.

## Canonical ExecutionTarget Contract

The implementation MAY choose a different concrete serialization, but the versioned contract MUST represent these semantic dimensions:

```text
ExecutionTarget
  version
  required_capabilities[]
  required_profiles[]
  preferred_profiles[]
  allowed_fallback_profiles[]
  prohibited_profiles[]
  reasoning_tier
  transport_policy
  api_policy
  budget_profile
  telemetry_requirements
  source_authority
  scope
```

### Transport policy

The contract MUST be able to distinguish at least:

```text
subscription_cli
metered_api
local
other_plugin_transport
```

A provider/model family is insufficient to represent transport policy.

### API policy

At minimum:

```text
forbid
allow
prefer_not
required
```

`forbid` MUST prevent silent API fallback.

## Routing Inputs

The router MUST derive the eligible set before applying preferences.

Eligibility inputs include:

1. demonstrated capabilities;
2. executor health/availability;
3. plugin/adapter trust and activation state;
4. security and authority policy;
5. org/user policy;
6. package/agent hard constraints;
7. graph/node hard constraints;
8. transport/API policy;
9. required evidence/telemetry properties;
10. scheduler/resource/quota availability.

Only after eligibility is established may affinity, budget, latency, learned preference, or other optimization inputs rank the eligible set.

## Authority and Precedence

The implementation MUST define a deterministic merge of execution-target inputs.

The following semantic law is mandatory:

> lower-authority inputs may further restrict or rank eligible execution but may not relax a stronger prohibition or hard requirement.

The implementation MUST preserve the source and authority of every target contribution so the final effective target is explainable.

An illustrative precedence order is:

```text
platform security/policy
  > organization/user policy
  > explicit authorized operator override
  > package/agent hard constraints
  > graph/node hard constraints
  > package/agent preferences
  > graph/node preferences
  > governed learned preference
  > automatic ranking
```

The exact order MAY differ only if reconciled with existing authority contracts and explicitly tested.

## Persistent Agent Semantics

Executor/model identity MUST NOT be part of persistent agent identity.

A persistent agent may execute consecutive turns on different eligible surfaces while retaining:

- agent ID;
- generation/lineage;
- memory;
- preferences;
- learned state;
- evidence chain.

Each execution event MUST record the selected surface and relevant routing decision.

Changing executor MUST NOT create a new agent generation unless another independent lifecycle rule requires generation change.

## Graph and Node Semantics

A graph/subgraph/node MAY contribute:

- required capabilities;
- required profile class;
- preferred profile class;
- fallback set;
- reasoning tier;
- budget profile;
- transport/API constraints.

Graph/node targeting MUST NOT directly name a vendor in core graph semantics when an equivalent neutral profile/selector exists.

Concrete adapters/catalog records MAY contain provider/runtime/model names as descriptive execution-surface metadata.

## Hard Requirement Behavior

If any effective hard requirement cannot be satisfied:

```text
eligible set = empty
  -> do not invoke inference
  -> emit deterministic routing failure
  -> preserve reason/evidence
  -> handoff/defer/fail according to graph policy
```

No hidden fallback is permitted.

## Fallback Semantics

The router MUST distinguish:

- preferred surface unavailable;
- required surface unavailable;
- allowed fallback selected;
- fallback prohibited by policy;
- fallback lacks capabilities;
- fallback exceeds budget;
- API fallback forbidden;
- subscription/local surface unauthenticated;
- quota/concurrency unavailable.

A fallback decision MUST be durable evidence.

## Budget Semantics

### Supported dimensions

Where reliable telemetry is available, budget policy MAY include:

- input/context tokens;
- output tokens;
- total tokens;
- monetary cost;
- wall-clock/latency budget;
- context-window headroom;
- provider-specific quota/rate availability;
- per-goal budget;
- per-run budget;
- per-agent budget;
- per-node/work-unit budget.

### Unknown telemetry

Unknown telemetry MUST remain unknown.

Praxis MUST NOT estimate or fabricate provider usage as authoritative accounting unless an explicitly defined estimator is labeled as estimated and excluded from authoritative billing/usage claims.

### Budget ranking

Budget pressure MAY influence ranking among eligible surfaces.

Budget policy MUST NOT:

- bypass required capability;
- bypass security/evidence requirements;
- silently lower required reasoning tier;
- silently switch to metered API execution when prohibited.

### Exhaustion

When an eligible executor cannot safely continue within a hard budget, the runtime MUST choose one of the graph/policy-authorized behaviors:

- continue on another eligible allowed surface;
- perform resource continuation/handoff;
- defer;
- fail closed.

The decision and remaining/known budget state MUST be recorded.

## Learned Routing Preferences

Routing outcomes MAY feed adaptive learning.

The learner MAY observe:

- task/work classification;
- executor surface/profile;
- latency;
- token/resource use where reliable;
- success/failure;
- review/evaluation quality;
- retry/repair counts;
- user correction;
- security/evidence outcomes.

The learner MUST NOT directly mutate active routing policy.

Required lifecycle:

```text
observation
  -> candidate preference
  -> replay/evaluation
  -> correctness/security/policy gates
  -> distinct-authority promotion
  -> active routing profile
```

Promotion and demotion MUST use existing governed adaptive-behavior semantics.

## Parallel Execution

Independent runnable graph branches MAY execute simultaneously on different executor surfaces.

The scheduler MUST account for executor-specific resources where declared, including:

- concurrency slots;
- process slots;
- provider quotas;
- transport sessions;
- memory/compute for local models;
- rate-limit windows where modeled.

`--parallel N` or equivalent graph/runtime ceilings limit runnable branch concurrency; they do not force identical executors or N agents.

A join barrier MUST wait on the required branch evidence regardless of which executor produced it.

## Develop Bundle Integration

The first-party Develop Bundle (#101) MUST consume this contract rather than inventing separate provider-selection logic.

Its specialized roles MAY express default affinities/requirements through package-owned profiles, for example implementation versus adversarial review, but those defaults remain subordinate to stronger user/org/security policy.

No canonical role is permanently bound to Codex, Claude, OpenAI, Anthropic, or any other provider.

## Subscription/API Distinction

At minimum the implementation MUST prove that these surfaces can be represented distinctly:

```text
Codex subscription/CLI
OpenAI API
Claude subscription/CLI
Anthropic API
```

Even where family/model lineage overlaps, the transports MUST remain separately selectable/prohibitable because authentication, metering, limits, policy, and observability differ.

## Operator Override

An authorized operator MAY request a preferred or required execution target.

Operator override MUST still satisfy:

- security policy;
- organizational prohibitions;
- capability requirements;
- active executor trust/health;
- API/transport policy.

Operator override is not permission to bypass security invariants.

## Routing Evidence

Each inference selection MUST record or make derivable:

```text
routing_decision_id
request/work identity
agent identity if applicable
graph/subgraph/node identity
required capabilities
effective execution target
source authorities contributing to target
eligible executor surfaces
selected executor surface
selection reason
fallback reason if any
provider/runtime/transport identity
model identity when available
reasoning tier/profile
known token/resource telemetry
unknown telemetry markers
policy/budget state relevant to decision
execution outcome
```

This evidence MUST be queryable by future supervision/TUI projections (#100).

## Failure Codes / Semantic Outcomes

The implementation SHOULD expose stable semantic outcomes equivalent to:

```text
NO_ELIGIBLE_EXECUTOR
REQUIRED_TARGET_UNAVAILABLE
FALLBACK_PROHIBITED
API_USE_PROHIBITED
AUTHENTICATION_REQUIRED
CAPABILITY_MISMATCH
TELEMETRY_REQUIREMENT_UNSATISFIED
BUDGET_EXHAUSTED
QUOTA_UNAVAILABLE
EXECUTOR_UNHEALTHY
```

Exact identifiers belong to canonical contract/versioning rules.

## Acceptance Criteria

The feature is not complete until evidence proves all of the following:

1. One graph can execute different nodes on different eligible executor families/surfaces.
2. One persistent agent can move between eligible executor surfaces without losing identity/lineage.
3. An agent/package can express a preferred surface with safe fallback.
4. A graph/node can require an execution profile and fail closed when unavailable.
5. Codex subscription/CLI and OpenAI API are distinguishable surfaces.
6. Claude subscription/CLI and Anthropic API are distinguishable surfaces.
7. User/org policy can prohibit all metered API execution.
8. No prohibited API fallback occurs silently.
9. Token/resource constraints affect ranking when reliable telemetry exists.
10. Missing telemetry remains explicitly unknown rather than fabricated.
11. Security/capability requirements outrank cost/token optimization.
12. Parallel graph branches can execute concurrently on different executor surfaces.
13. Routing decisions are explainable from durable evidence.
14. Learned routing changes require governed evaluation/promotion.
15. Provider/vendor names do not leak into core graph semantics where a neutral profile suffices.
16. Explicit operator overrides remain bounded by security/policy constraints.
17. `--parallel 1` remains semantically legal and serial without changing executor eligibility rules.
18. Restart/recovery does not duplicate a routed work unit or lose the selected-surface evidence.
19. Fallback from subscription/local to metered API occurs only with explicit policy permission.
20. Develop Bundle integration consumes this contract and does not duplicate routing logic.

## Non-Goals

This SPEC does not:

- define concrete Codex/Claude/OpenAI/Anthropic adapters;
- make any provider mandatory;
- guarantee provider token telemetry that the provider does not expose;
- permit the LLM itself to become routing authority;
- replace scheduler/resource governance;
- redefine persistent agent identity;
- make cost optimization more authoritative than correctness/security.
