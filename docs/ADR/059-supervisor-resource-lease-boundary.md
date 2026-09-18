# ADR-059: Supervisor Resource-Lease Boundary

- Status: Accepted
- Date: 2026-09-14
- Governing: ADR-034, ADR-056, ADR-057

## Decision

Every supervised plugin process attempt that declares resource requirements
must acquire the complete requirement set through the authoritative scheduler
before launch. The supervisor releases those scheduler leases on explicit
stop, revocation, launch failure, and observed process exit. Resource
requirements are policy-owned; a plugin cannot select its own capacity or
convert a capability lease into scheduler authority.

Admission failure prevents process launch and restores the prior durable
lifecycle state. Lease expiry and recovery remain owned by the scheduler;
supervisor restart never treats an old runtime attempt as live. The exact
lease IDs are retained in the supervisor snapshot so restart can explicitly
fence an abandoned attempt even when its leases have no expiry; cleanup is
idempotent and the cleared snapshot is persisted before the instance can be
started again.

The supervisor also validates the leaser's successful response at the launch
boundary. The response must be a complete one-to-one set of positive-capacity
leases bound to the exact instance, runtime-session attempt, resource keys, and
requested capacities. A partial, duplicate, or foreign response is not
admission authority and fails closed before process launch.

Runtime cleanup uses the scheduler's exact slice/attempt cancellation boundary
when the authoritative provider exposes it. The retained lease IDs remain
required for restart fencing and compatibility with providers that only expose
idempotent ID release; runtime cancellation must not reconstruct authority from
caller-controlled or stale lease lists.

## Consequences

- A process cannot consume resources without an atomic scheduler admission.
- Every terminal process path has an explicit release boundary.
- Restart and replacement cannot strand or silently discard an existing
  scheduler lease set.
- A faulty or compromised leaser cannot mint launch authority by returning a
  partial or identity-mismatched lease set.
- Scheduler coordination state remains distinct from plugin security leases.
- Full OI-024 qualification still requires cancellation, retry, quota, crash
  recovery, and end-to-end mediation evidence.
