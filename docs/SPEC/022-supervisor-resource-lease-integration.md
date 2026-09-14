# SPEC-022: Supervisor Resource-Lease Integration

1. A supervisor policy MAY declare typed scheduler resource requirements and a
   lease lifetime. Declared requirements require an authoritative resource
   leaser.
2. Start SHALL request all requirements atomically before invoking the process
   launcher, using an attempt identity bound to the supervised instance and
   runtime session.
3. If admission is denied, the launcher SHALL NOT be invoked and the prior
   lifecycle state SHALL remain authoritative.
4. Successful attempts SHALL release their scheduler leases on launch failure,
   explicit stop, revocation, or unexpected process exit. Runtime cleanup SHALL
   prefer the authoritative exact slice/attempt release boundary when exposed;
   otherwise it MAY use the retained lease IDs through an idempotent provider
   operation. Release SHALL NOT make a revoked or failed plugin routable again.
5. Supervisor snapshots SHALL retain the exact lease IDs for an active
   attempt. Supervisor restart SHALL not resurrect a runtime process or its
   ephemeral attempt identity; it SHALL release the retained lease IDs and
   persist the normalized snapshot before a new attempt may start. Scheduler
   expiry/recovery remains authoritative and release SHALL remain idempotent.
6. The supervisor SHALL validate that a successful admission response contains
   exactly one positive-capacity lease for every requested resource key, with
   matching slice and attempt identities and capacities. A partial,
   duplicate, foreign, or otherwise unbound response SHALL fail closed before
   launch and release any returned lease IDs.

This specification integrates the supervisor with the scheduler admission
substrate; it does not by itself satisfy OI-024.
