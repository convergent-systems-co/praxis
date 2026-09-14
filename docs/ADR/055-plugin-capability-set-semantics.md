# ADR-055: Plugin Capability Set Semantics

- Status: Accepted
- Date: 2026-09-13

## Context

The initial Go handshake required the runtime advertisement to equal the signed plugin manifest capability list. Original plugin authority instead distinguishes package review, runtime availability, routing eligibility, demonstrated competence, and granted authority. ADR-016 separates advertised from demonstrated capability. ADR-029 requires typed runtime advertisement. ADR-041 requires grants to be scoped leases independent of declarations, and permits lifecycle degradation. ADR-053 makes the exact executable and its declarative manifest reviewable without making installation a grant.

Exact equality collapsed the signed upper bound and current instance availability. It also made `Provider` route against manifest declarations, so merely deleting the equality loop would have caused unavailable capabilities to remain selectable.

## Decision

For one verified plugin executable generation:

- `Manifest.Capabilities` is the signed, locally reviewed **permitted/supported upper bound**;
- the process handshake supplies **advertised capabilities** currently available from that exact instance/session;
- every advertised capability MUST be present in the manifest, but the advertisement MAY be a subset;
- the validated provider records the actual advertised set, and routing uses that set rather than the manifest upper bound;
- graph/request requirements and local policy determine whether a subset is sufficient for a particular use or whether startup must fail/degrade;
- demonstrated capability evidence may influence eligible routing but cannot expand either set;
- a **granted capability** exists only as a deterministic, scoped lease bound to principal, instance, runtime session, operation, scope, constraints, and lifecycle authority.

The current manifest has no distinct required-capability field. Core SHALL NOT infer that every permitted capability is required. A future package/profile contract may declare required availability explicitly if use cases demand it; that semantic addition must follow contract-version policy.

An executable update changes package/plugin version and digest. A process restart changes instance/session identity. Existing leases cannot authorize either new identity. Revocation is enforced at authoritative lease consumption and does not depend on advertisement or plugin cooperation.

Runtime advertisement is ephemeral session evidence, not durable package state. Registry publication requires the validated handshake result for the exact supervised launch; a caller-selected `ready` state or provider field cannot publish availability. After restart, the old runtime advertisement is discarded and the new instance must complete fresh identity, isolation, protocol, and advertisement validation before it becomes routable.

## Consequences

- Platform-constrained or degraded instances can expose a safe subset without gaining unreviewed authority.
- An undeclared advertisement remains a fail-closed escalation attempt.
- Package declaration cannot become execution authority or even current routing availability by itself.
- Provider state must preserve validated instance advertisement independently from the manifest.
- Equality review retained exact comparison where identity is contractual: signed artifact bytes, locked dependency closures, cryptographic AAD, and bound instance/session identity. Scope containment and capability advertisement instead retain their separately specified subset semantics.
- This correction is lifecycle substrate and closes no original-intent finding without real process/restart/isolation evidence.
