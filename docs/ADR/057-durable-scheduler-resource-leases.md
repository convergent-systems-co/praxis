# ADR-057: Durable Scheduler Resource Leases

- Status: Accepted
- Date: 2026-09-13
- Governing: ADR-003, ADR-031, ADR-038

The scheduler's resource leases are authoritative coordination state, distinct
from security capability leases. SQLite stores resource capacity and active
leases. A multi-resource request sorts requirements by resource identity,
expires stale leases inside the same transaction, validates every resource,
and inserts the complete lease set atomically; no waiting slice retains a
partial prefix. Release is idempotent. Cancellation may release by the exact
slice/attempt identity, so a caller does not need to reconstruct a lease-ID
list and cannot accidentally release a sibling attempt. Restart reuses only
unexpired, unreleased rows. Corrupt or unavailable resource state fails
closed.

This is a durable admission substrate, not evidence that every execution path
or child graph already traverses it. OI-024 remains open until cancellation,
quota inheritance, retry, process-crash recovery, and complete runtime
mediation are independently demonstrated.
