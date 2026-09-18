# ADR-056: Durable Plugin Supervisor Lifecycle

- Status: Accepted
- Date: 2026-09-13
- Governing: ADR-029, ADR-041, ADR-053, ADR-055

## Decision

Plugin supervision persists the verified launch binding and lifecycle failure
state in authoritative Praxis state. The persisted record contains the exact
package-bound executable bytes, manifest/instance identity, entrypoint, socket
endpoint, isolation requirements, failure counters, and quarantine/revocation
state. It does not persist runtime advertisement, readiness evidence, or a
capability lease as if either survived a process boundary.

On runtime restart, `starting`, `ready`, and `degraded` records normalize to
`stopped`. A new process must be launched from the persisted verified binding,
complete a fresh identity/protocol/isolation/advertisement handshake, and pass
the deterministic authority boundary before registry publication or lease use.
Quarantine and revocation remain durable and fail closed.

This separates recoverable package/lifecycle state from ephemeral process
evidence. It does not claim that a host has effective filesystem, network,
credential, or resource isolation; missing host enforcement remains a launch
failure.

## Consequences

- Restart cannot resurrect stale runtime advertisement or authority.
- Failure/quarantine state is not lost when the supervising process exits.
- Launch uses exact retained package bytes rather than a mutable path.
- Durable snapshots are an integration prerequisite; OI-021/OI-030/OI-037
  remain open until complete real-process, lease, update, and isolation
  lifecycle evidence exists.
