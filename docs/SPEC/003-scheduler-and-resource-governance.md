# SPEC-003: Scheduler and Resource Governance

- Status: Draft
- Governing ADRs: 004, 022, 030, 031, 034, 035, 038, 041, 042

## Purpose

Define Slice scheduling, concurrency, resource leases, cancellation, fairness, bounded work, quotas, and recovery independently of any particular domain.

The scheduler is part of the deterministic enforcement surface. A graph, plugin, model, or client SHALL NOT be able to bypass scheduler/resource limits by spawning subgraphs, retries, tools, or plugin work outside governed accounting.

## Slice scheduling contract

A runnable Slice SHALL expose:

- stable Slice identity and execution attempt identity;
- dependencies;
- required/optional resources;
- capacities;
- deadline/timeout;
- cancellation policy;
- retry policy;
- priority class;
- execution binding constraints;
- quota class;
- maximum attempts;
- maximum nested/subgraph expansion where applicable;
- cost/budget references where applicable.

The scheduler SHALL admit a Slice only when dependencies, policy, capability leases, resource availability, and applicable quotas permit execution.

## Resource acquisition

- Declared resource requirements SHALL normalize to stable typed resource keys.
- Multi-resource acquisition SHALL use deterministic ordering and all-or-nothing admission.
- A waiting Slice SHALL NOT retain a partial set of declared leases.
- Capacity MAY be greater than one.
- Exclusive requirements consume the resource's exclusive capacity.
- Dynamic requirements discovered during execution SHALL return through scheduler governance.
- A dynamically discovered resource SHALL NOT be acquired through an unmanaged side channel.

## ResourceLease

A `ResourceLease` SHALL contain at minimum:

- lease ID;
- Slice ID and execution attempt ID;
- resource key;
- capacity;
- acquisition time;
- expiry/heartbeat/recovery metadata where applicable;
- release state;
- principal/executor binding where relevant;
- correlation ID.

Resource leases are runtime coordination state and SHALL be distinguishable from security `CapabilityLease` records.

The authoritative scheduler store SHALL persist resource capacities and leases.
Multi-resource acquisition SHALL order requirements by stable resource key,
expire stale leases within the admission transaction, and insert all requested
leases atomically. A denied request SHALL retain no partial lease. Restart
MUST recover only unexpired, unreleased leases; runtime mutex state is not
authority.

Resource leases SHALL be recoverable after runtime restart; correctness cannot depend solely on in-memory mutex state.

The authoritative state provider SHALL expose cancellation release by the exact
Slice ID and execution attempt ID. The operation SHALL be idempotent, release
only active leases for that identity, and avoid requiring a caller to
reconstruct a lease-ID list. A sibling attempt sharing the Slice ID SHALL
remain unaffected.

## Quota model

Praxis SHALL support hierarchical deterministic quotas at appropriate scopes such as runtime, user, package, graph, agent, run, Slice, plugin, executor, workspace, and destination/provider.

Quota dimensions SHALL support at least:

- concurrent Slices;
- concurrent plugin/executor requests;
- graph depth/nesting;
- spawned Slice/subgraph count;
- retry count;
- wall-clock runtime;
- CPU time where measurable;
- memory where enforceable;
- filesystem/index bytes where relevant;
- network/request count where relevant;
- tool/effect count;
- context bytes/tokens;
- inference tokens/cost budget where observable;
- event/log volume;
- queue/backlog bounds.

Quota semantics SHALL specify whether a limit is hard, soft/warning, or policy-escalated.

Security/resource hard limits SHALL be enforced outside the LLM.

## Budget propagation

Nested work SHALL inherit or receive an explicit child allocation from a parent budget.

Creating child graphs/Slices SHALL NOT mint additional budget implicitly.

A parent may reserve/delegate bounded budget to a child. Unused child allocation MAY return to the parent according to policy.

Retry SHALL consume budget unless policy explicitly defines a non-consuming retry class.

## Fairness

Within equal priority, admission SHALL be age-aware FIFO or an equivalent deterministic fair policy.

Priority scheduling SHALL include bounded-starvation behavior through aging or equivalent policy.

Quota exhaustion by one principal/package SHALL NOT indefinitely starve unrelated principals sharing the runtime when independent capacity remains.

## Cancellation

Cancellation SHALL propagate from graph/goal/agent/runtime scopes to affected Slices and child work.

Executors receive cooperative cancellation where supported. Praxis remains authoritative about Slice state and SHALL release/recover resource leases after cancellation, timeout, executor loss, plugin loss, or runtime restart.

Cancellation SHALL NOT by itself imply that an external effect did not occur; effect state from SPEC-002 governs reconciliation.

## Retry

Retry SHALL be policy-driven and recorded as a new execution attempt associated with the same logical Slice.

Retry admission SHALL re-evaluate:

- retry limit;
- remaining budget/quota;
- capability/authority state;
- cancellation state;
- effect reconciliation requirements;
- target preconditions where relevant.

Non-idempotent external effects SHALL be reconciled before retry.

Retry storms SHALL be bounded through maximum attempts plus backoff/rate policy.

## Circuit breaking and overload

The scheduler/runtime SHOULD support deterministic overload controls for repeatedly failing plugins/executors/providers.

Circuit state SHALL have explicit scope, reason, opened-at time, retry/probe rules, and recovery semantics.

A model SHALL NOT be able to disable a hard circuit breaker merely by requesting another attempt.

## Events and telemetry

The scheduler SHALL emit/record sufficient durable events or authoritative state transitions for:

- admission;
- wait reason;
- quota denial/warning;
- resource lease acquisition;
- execution start;
- suspension;
- cancellation;
- timeout;
- retry;
- completion;
- failure;
- resource lease release/recovery;
- circuit open/probe/close where implemented.

Metrics SHALL expose at least queue depth, wait time, execution time, resource utilization, starvation/aging, retries, cancellations, lease recovery, quota exhaustion, child-work expansion, and circuit state.

## Failure behavior

- resource unavailable: wait or fail according to policy, without partial acquisition;
- hard quota exceeded: deterministic denial/suspension, no unaccounted child work;
- executor/plugin disappears: transition to governed failure/recovery; reclaim leases safely;
- crash with active leases: recover from durable lease state/fencing semantics;
- cancellation during side effect: delegate effect outcome handling to SPEC-002;
- corrupted lease state: fail closed for contested/exclusive resources until reconciled;
- retry budget exhausted: terminal failure or human/policy escalation, not infinite retry.

## Acceptance tests

The implementation SHALL prove:

1. deterministic acquisition ordering prevents circular wait;
2. a Slice requiring two unavailable resources holds neither while waiting;
3. low-priority work eventually executes under sustained higher-priority load according to configured bounds;
4. cancellation releases/reclaims resource leases;
5. process crash with active leases recovers safely;
6. subgraph execution cannot bypass capacity or quota limits;
7. dynamic resource discovery cannot create an unmanaged lock;
8. retry does not duplicate an unreconciled external effect;
9. nested graphs cannot mint unlimited child budget;
10. maximum graph depth and spawned-work count are enforced independently of model behavior;
11. failing executor/plugin cannot cause unbounded retry storm;
12. quota accounting survives restart for durable budgets where required;
13. one package exhausting its quota does not consume unrelated reserved capacity beyond policy;
14. cancellation does not incorrectly mark an unknown external effect as absent.

## Deliverables

- scheduler admission contract;
- `ResourceRequirement` and `ResourceLease` schemas;
- quota/budget schemas and hierarchy rules;
- retry/backoff contract;
- cancellation propagation contract;
- durable lease recovery/fencing design;
- fairness/starvation policy;
- overload/circuit interface;
- scheduler event/metric definitions;
- adversarial resource-exhaustion fixture corpus.

## Exit criteria

SPEC-003 is implementation-ready when resource acquisition, fairness, quota inheritance, child-budget propagation, retry/cancellation, durable lease recovery, and exhaustion behavior can be expressed as deterministic state-machine/conformance tests.

## Non-goals

This specification does not mandate distributed workers in the first implementation and does not require every operating system to expose identical CPU/memory enforcement primitives. Unsupported enforcement properties SHALL be reported rather than assumed.
