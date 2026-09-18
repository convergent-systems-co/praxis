# ADR-058: Durable Effect Outcome Lifecycle

- Status: Accepted
- Date: 2026-09-13
- Governing: ADR-002, ADR-031, ADR-038

The authoritative state store records effect dispatch attempts and observed
outcomes separately. A dispatch transition increments attempts; an external
result is explicitly `succeeded`, `failed`, `unknown`, or `reconciling` and
retains observed/reconciliation evidence. Restart enumerates recoverable
records from SQLite; an unknown outcome is never inferred as success and cannot
be retried without reconciliation or verified idempotency.

This closes no original-intent claim by itself. Every mutation/effect entry
surface still must use the canonical command/effect boundary, and external
outcomes still require independent integration and adversarial evidence.
