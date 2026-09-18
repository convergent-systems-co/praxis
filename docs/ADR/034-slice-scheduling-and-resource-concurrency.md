# ADR-034: Slice Scheduling and Resource Concurrency

- Status: Draft
- Date: 2026-09-13

## Context

Praxis graphs may execute work across heterogeneous constrained resources: LLM clients, local models, tools, plugins, human checkpoints, filesystems, network services, devices, and domain-specific executors. The externally meaningful execution unit is a Slice; implementation blocks are internal details. Without canonical concurrency semantics, plugins and runtimes can introduce deadlock, starvation, non-replayable scheduling behavior, or hidden resource ownership.

## Decision

Praxis schedules and governs concurrency at the Slice boundary. A Slice declares its resource requirements before acquisition whenever they are knowable.

### Resource model

Resources are identified by stable typed keys and may expose capacity greater than one. Examples include `executor:claude`, `executor:mlx`, `tool:github`, `human:approval`, or domain-defined resources.

A Slice declares:

- required resources and capacity;
- optional resources and fallback behavior;
- acquisition timeout/deadline;
- cancellation policy;
- priority class where applicable;
- whether a requirement is exclusive.

### Acquisition

Praxis uses ordered all-or-nothing acquisition for declared resources. Resource keys are normalized into a deterministic global ordering. A Slice must not hold one declared resource while indefinitely waiting for another declared resource.

If all required capacity cannot be acquired, the Slice waits without partial ownership. Dynamic resources discovered during execution must be requested through the scheduler; the runtime may suspend and release releasable resources before reacquisition rather than creating an ungoverned nested lock hierarchy.

### Fairness and starvation

Within a priority class, waiting Slices use age-aware FIFO ordering. Priority may influence admission but may not permit permanent starvation. Implementations must support aging or an equivalent bounded-starvation mechanism.

### Cancellation and timeout

Cancellation is cooperative at executor boundaries and authoritative at the Praxis runtime boundary. On cancellation, timeout, executor loss, or graph termination, leases are released deterministically. Resource ownership is represented as leases with identity and expiry/recovery semantics, not process-local mutexes alone.

### Determinism

Scheduling policy is deterministic for equivalent durable inputs, but wall-clock completion order of concurrent external work is not asserted to be deterministic. Praxis records admission, acquisition, release, cancellation, retry, and completion events sufficient to explain and replay state transitions.

### Nested execution

A Slice may invoke subgraphs, but subgraph execution does not bypass scheduler governance. Child work receives its own Slice identity or executes inside the parent's explicitly declared resource envelope.

## Consequences

The scheduler can reason consistently about local and remote resources. Deadlock prevention is architectural rather than left to individual graph authors. Resource behavior is observable and testable.

The design favors safety and explainability over maximum opportunistic concurrency. Later scheduler optimizations may improve throughput without changing these invariants.

## Non-goals

This ADR does not prescribe a particular queue implementation, distributed lock service, or worker topology.