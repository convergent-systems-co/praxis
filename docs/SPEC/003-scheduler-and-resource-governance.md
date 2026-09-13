# SPEC-003: Scheduler and Resource Governance

- Status: Draft
- Governing ADRs: 004, 022, 030, 031, 034, 035

## Purpose

Define Slice scheduling, concurrency, resource leases, cancellation, fairness, and recovery independently of any particular domain.

## Slice scheduling contract

A runnable Slice SHALL expose dependencies, required/optional resources, capacities, deadline/timeout, cancellation policy, retry policy, priority class, and execution binding constraints.

The scheduler SHALL admit a Slice only when dependencies and policy permit execution.

## Resource acquisition

- Declared resource requirements are normalized to stable typed resource keys.
- Multi-resource acquisition is ordered and all-or-nothing.
- A waiting Slice does not retain a partial set of declared leases.
- Capacity may be greater than one.
- Exclusive requirements consume the resource's exclusive capacity.
- Dynamic requirements discovered during execution return through scheduler governance.

## Lease model

A lease SHALL contain lease ID, Slice ID, resource key, capacity, acquisition time, expiry/recovery metadata, and release state. Leases must be recoverable after runtime restart; correctness cannot depend solely on in-memory mutex state.

## Fairness

Within equal priority, admission SHALL be age-aware FIFO. Priority scheduling SHALL include bounded-starvation behavior through aging or equivalent policy.

## Cancellation

Cancellation SHALL propagate from graph/goal/agent/runtime scopes to affected Slices. Executors receive cooperative cancellation where supported. Praxis remains authoritative about Slice state and SHALL release/recover leases after cancellation, timeout, executor loss, or runtime restart.

## Retry

Retry SHALL be policy-driven and recorded as a new execution attempt associated with the same logical Slice. Non-idempotent external effects require effect reconciliation before retry.

## Events and telemetry

The scheduler SHALL emit/record sufficient durable events or authoritative state transitions for admission, wait reason, lease acquisition, execution start, suspension, cancellation, timeout, retry, completion, failure, and lease release/recovery.

Metrics SHALL expose queue depth, wait time, execution time, resource utilization, starvation/aging, retry rate, cancellation rate, and lease recovery.

## Acceptance tests

- deterministic acquisition ordering prevents circular wait;
- a Slice requiring two unavailable resources holds neither while waiting;
- low-priority work eventually executes under sustained higher-priority load according to configured bounds;
- cancellation releases leases;
- process crash with active leases recovers safely;
- subgraph execution cannot bypass capacity limits;
- dynamic resource discovery cannot create an unmanaged lock;
- retry does not duplicate a reconciled external effect.

## Non-goals

This specification does not mandate distributed workers in the first implementation.