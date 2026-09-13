# SPEC-009: Inference Routing and Reasoning Budgets

- Status: Draft
- Governing ADRs: 015, 016, 018, 038, 040, 043
- Depends on: SPEC-001, SPEC-003, SPEC-004, SPEC-007

## Purpose

Define how Praxis chooses deterministic execution versus bounded or open model inference while optimizing for correctness, latency, token/cost usage, and observed capability rather than assuming maximum reasoning is always desirable.

## Core model

Praxis SHALL classify executable work into:

- `D0 deterministic`: no model judgment required;
- `D1 bounded inference`: narrow judgment with explicit constraints and bounded effort;
- `D2 open inference`: novel/ambiguous/high-uncertainty reasoning.

The default objective is the **lowest reasoning tier that reliably satisfies the task**.

## ReasoningBudget

Every D1/D2 request SHALL resolve a versioned budget containing at least:

- reasoning tier;
- maximum wall-clock latency/deadline;
- maximum tokens/context bytes;
- maximum attempts/retries;
- maximum monetary/provider budget where measurable;
- required capability/evidence threshold;
- escalation policy;
- fallback/degradation policy;
- cancellation policy;
- optional time-to-first-useful-action objective.

A graph/package SHOULD express outcome requirements and uncertainty, not vendor-specific hidden-thinking controls.

## Fast-path rule

Praxis SHALL NOT invoke inference when a D0 capability can deterministically satisfy the request with required correctness/evidence.

D1 SHALL begin with the smallest eligible bounded executor/profile demonstrated to meet the requirement. D2 SHALL use stronger/open reasoning only when classification or escalation requires it.

## Escalation

Escalation MAY occur when:

- deterministic validation fails with ambiguity rather than denial;
- D1 confidence/evidence falls below threshold;
- tool output is inconsistent or incomplete;
- a bounded attempt exhausts its allowed repair loop;
- novelty/uncertainty classifier crosses configured threshold;
- policy explicitly requires independent stronger review.

Escalation SHALL record reason, source tier, destination tier, elapsed latency/tokens/cost, and evidence causing escalation.

Security/policy denial SHALL NOT be converted into an inference escalation intended to persuade the system to allow the action.

## De-escalation and learning

Observed repeated success MAY nominate work for D2 -> D1 or D1 -> D0 extraction according to learning/governance ADRs. Extraction requires deterministic/evaluable acceptance evidence; model self-confidence alone is insufficient.

If a D0/D1 path becomes unreliable, policy/evidence MAY promote it back to a higher tier.

## Executor capability evidence

Routing SHALL use demonstrated capability evidence where available: task class, success rate, latency distribution, token/cost distribution, context limits, tool support, structured-output reliability, and recency/sample size.

Provider/model identity MAY be part of evidence but SHALL NOT itself prove capability.

## Latency optimization

Praxis SHALL measure:

- queue wait;
- time to executor start;
- time to first useful action/evidence;
- total inference latency;
- tool-call latency;
- repair/retry latency;
- total goal/slice latency.

Routing MAY prefer a faster executor with sufficient demonstrated quality over a stronger but slower executor when the task contract permits it.

## Parallelism and speculative inference

Speculative/parallel inference is permitted only when its expected value exceeds resource/cost policy and it cannot duplicate unsafe side effects. Competing inference results remain proposals until deterministic reconciliation/selection.

Do not parallelize authoritative side effects merely to reduce latency.

## Context minimization

Inference requests SHOULD use Workspace Intelligence/context packs and memory retrieval to provide the minimum sufficient evidence. Full history/repository injection is not a substitute for retrieval.

Context budgets SHALL preserve provenance/trust labels and destination sensitivity controls.

## Slow-model protection

An executor exceeding budget MAY be cancelled or superseded according to policy. Cancellation SHALL not imply that any external side effect already requested by that executor is safe to retry; side effects remain under SPEC-002 reconciliation.

## Client-hosted inference

When inference is provided by an LLM client rather than a direct provider adapter, Praxis SHALL distinguish measured client/model capability from host enforcement properties. Strong reasoning capability does not imply permission/enforcement capability.

## Acceptance tests

1. D0 task executes without model call;
2. D1 eligible task chooses bounded executor satisfying threshold;
3. failed D1 ambiguity escalates to D2 and records reason;
4. policy denial does not escalate to model override;
5. faster sufficient executor is preferred when latency objective permits;
6. slow executor is cancelled at configured deadline;
7. context budget is not exceeded silently;
8. repeated successful fixture can be nominated for lower tier but not auto-promoted without governance;
9. executor self-reported confidence cannot substitute for acceptance evidence;
10. speculative inference cannot duplicate external side effects.

## Deliverables

- reasoning-tier and budget contracts;
- executor capability evidence model;
- deterministic router interface;
- escalation/de-escalation event model;
- latency/token/cost telemetry;
- cancellation/deadline integration;
- routing benchmark corpus;
- conformance tests for D0/D1/D2 behavior.
