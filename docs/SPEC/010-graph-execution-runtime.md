# SPEC-010: Graph Execution Runtime

- Status: Draft
- Governing ADRs: 002, 004, 005, 022, 030, 034, 035, 036
- Depends on: SPEC-001, SPEC-002, SPEC-003, SPEC-006, SPEC-007, SPEC-009

## Purpose

Define domain-neutral execution semantics for versioned Praxis graphs, including nodes, transitions, loops, slices, checkpoints, cancellation, retries, nested graphs, inference points, evidence, and terminal outcomes.

## Core invariants

1. A graph is a versioned executable process representation, not a prompt transcript.
2. Runtime state transitions are deterministic given authoritative state plus declared executor/evidence results.
3. Graph semantics do not assume software-development concepts.
4. Loops are explicit and bounded by policy/quotas; acyclic graphs are not required.
5. Side effects leave the graph runtime through canonical command/effect boundaries.
6. Model output is proposal/evidence until validated by the node contract.
7. A run can checkpoint/recover without reconstructing state from conversation history.
8. Nested graphs inherit tighter effective quotas/authority and cannot expand parent scope.

## Graph definition

A `GraphVersion` SHALL contain stable graph/version identity, entry points, typed inputs/outputs, nodes, transitions, required capabilities, policy/invariant references, default reasoning tiers/budgets, checkpoint policy, failure/cancellation policy, and compatibility metadata.

## Node classes

The runtime SHALL support generic node classes sufficient to express:

- deterministic operation;
- capability invocation;
- bounded/open inference;
- human decision/approval;
- condition/branch;
- join/barrier;
- subgraph invocation;
- checkpoint;
- terminal result.

Domain packages MAY define higher-level authoring abstractions that compile into canonical node classes.

## Node contract

Every node SHALL declare stable node ID, node class/version, typed input/output contract, required capabilities/resources, reasoning tier/budget where applicable, timeout, retry policy, idempotency/effect properties, accepted evidence classes, and transition outcomes.

## Run identity

A graph invocation creates a stable `Run` identity bound to graph/package version, entry point, initiator/principal, scope/context, input digest, policy snapshot references, quota root, correlation ID, and creation time.

A run SHALL NOT silently switch graph version mid-execution. Migration/resume into a different version requires an explicit compatible transition/migration record.

## Slice semantics

A `Slice` is the externally meaningful schedulable work unit. One slice MAY contain one or more internal deterministic node/block transitions where doing so reduces orchestration overhead without hiding side effects or checkpoints.

Slices carry dependencies, resources, quotas, deadline, cancellation, retry, and executor binding constraints according to SPEC-003.

## Runtime states

At minimum run/slice execution SHALL distinguish queued, runnable, waiting, running, suspended, cancelling, cancelled, succeeded, failed, and recovery/reconciliation states where applicable.

Terminal states are immutable except through explicit corrective/migration records; they are not overwritten in place.

## Transitions

A transition is allowed only when its source state/node outcome and graph definition permit it. Runtime SHALL reject unknown transition labels, nonexistent destinations, incompatible output/input types, and transition attempts against stale aggregate versions.

Conditions intended to be deterministic SHALL execute without an LLM.

## Loops

Cycles are allowed when explicitly represented by graph transitions. Every loop path SHALL be governed by at least one bound such as attempt count, time/deadline, resource/token/cost quota, convergence/evidence condition, or explicit human continuation.

A graph failing to provide a valid bound for an execution cycle SHALL be rejected by validation unless policy supplies one externally.

## Inference nodes

Inference nodes request D1/D2 capability through SPEC-009. Model/provider identity is resolved at runtime unless the graph contract requires a provider-specific capability.

Inference output SHALL be parsed/validated against the node output schema. Invalid output may enter a bounded repair path; it cannot directly mutate authoritative graph state as free-form text.

## Deterministic compilation

Repeated learned behavior MAY be compiled into D0/D1 structures through learning/governance. Compiled nodes retain lineage to evidence/learning records and remain subject to graph versioning.

## Capability nodes

Capability invocation resolves an eligible capability provider/lease through SPEC-007. Capability advertisement is insufficient. Side-effecting capabilities SHALL use ActionIntent/effect mediation.

## Human nodes

Human-input/approval nodes suspend durably while awaiting an external decision. A client disconnect does not discard the pending state. Approval semantics follow SPEC-002/ADR-042 and are not represented merely by conversational assent text.

## Nested graphs

A subgraph invocation creates child run identity linked by causation. Effective scope, quotas, capabilities, and policies are intersection/tightening of parent plus child requirements. A child cannot broaden authority inherited from the parent.

Nested depth SHALL be bounded.

## Checkpointing

Checkpoint records SHALL include run/graph version, current node/slice state, completed transition/evidence references, outstanding dependencies/effects, quota usage, active/recoverable leases, and event sequence/checkpoint required for resume.

Checkpoint creation SHALL not serialize private keys, raw credentials, or unnecessary model context.

## Cancellation

Cancellation propagates from run/goal/agent/runtime scopes to active slices/subgraphs according to policy. Executors receive cooperative cancellation where available. Praxis remains authoritative for resulting state and resource/lease cleanup.

External effects with unknown outcomes enter reconciliation rather than being marked cancelled as though they did not occur.

## Retry

Retry creates a new execution attempt linked to the same logical node/slice. Retry limits/backoff are deterministic. Side-effect retry requires proven idempotency or reconciliation.

A retry cannot reset parent quota consumption unless policy explicitly defines refundable units.

## Evidence

Node completion SHALL emit attributable evidence sufficient for downstream validation. Evidence records bind node/run/attempt identity, provider/executor, inputs/provenance, output/result digest, validation/evaluation result, timing/resource usage, and trust/evidence class.

## Failure handling

Failures SHALL be typed at minimum as validation, capability unavailable/denied, resource/quota, timeout, cancellation, executor failure, inference invalid, external-effect unknown, invariant/policy violation, migration/version mismatch, and internal runtime fault.

Graph transitions MAY handle declared recoverable failure classes. Policy/invariant violations cannot be swallowed by a graph transition intended to bypass enforcement.

## Recovery

Startup recovery SHALL reconstruct runnable/waiting/suspended/reconciling states from authoritative persistence. It SHALL revalidate ephemeral external enforcement/capability conditions before resuming privileged work.

No LLM is required to determine where a run resumes.

## Observability

Record run/slice/node/attempt identity, state durations, transition decisions, wait reason, resource/quota usage, executor/provider selection, D0/D1/D2 tier, time to first useful action, evidence refs, retry/escalation, cancellation, and terminal outcome.

## Acceptance tests

1. deterministic fixture graph runs without model inference;
2. explicit loop terminates at configured bound;
3. unbounded cycle is rejected;
4. invalid transition label/destination is rejected;
5. restart resumes from checkpoint without replaying completed side effects;
6. cancellation propagates to child run and releases schedulable resources;
7. unknown external effect enters reconciliation, not false cancellation/success;
8. child graph cannot expand parent quota or capability scope;
9. stale aggregate transition fails optimistic concurrency;
10. invalid inference output cannot mutate run state without validation;
11. human decision suspends/resumes durably across client disconnect;
12. same graph executes with multiple eligible executors without graph rewrite;
13. token/cost/attempt quota bounds recursive/looping graph work.

## Deliverables

- canonical graph/run/node/transition schemas;
- graph validator;
- run/slice/node state machines;
- transition evaluator;
- bounded loop validator/runtime accounting;
- checkpoint/resume representation;
- cancellation propagation;
- retry/attempt model;
- subgraph authority/quota inheritance;
- execution/evidence event definitions;
- deterministic fixture graph suite.
