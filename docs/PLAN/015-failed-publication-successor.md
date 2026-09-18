# PLAN-015: Goals `/5` failed-publication successor

Status: Proposed
Predecessors: PLAN-014; PLAN-013; PLAN-012

Implement and qualify only the closed `/5` successor defined by ADR-086 and
SPEC-049.

The implementation denominator is:

1. Add exact persisted reconstruction from the initial publication through
   abandoned `/1`, `/2`, failed `/3`, `/4` historical recovery, resolved
   `/4` verification, and failed pre-dispatch `/4` publish. Exclude the
   malformed `/4` preparation-only request.
2. Prove pre-dispatch failure from command/event/effect state, one attempt,
   absent result, absent reconciliation, and absence of provider transition.
3. Add deterministic `/5` predecessor projection, chain extension, ActionIntent,
   request, and ordered graph identities.
4. Add the read-only preparation CLI
   `goals-publication-recovery-failed-publication-prepare`.
5. Require fresh request, decision, delegated child, and bounded authority;
   preserve ADR-084 handling when `/4` authority is expired.
6. Implement only `publish` then `verify-published`; prohibit uploads,
   verification replay, release/tag creation, and retry of `/4`.
7. Revalidate exact draft release and three-way identity-bound asset state
   immediately before each effect. Preserve provider-order independence.
8. Preserve local failure, UNKNOWN publication, reconciliation, completion,
   and historical immutability semantics.

Tests must cover positive persisted preparation and execution, every lineage and
pre-dispatch substitution, changed release or asset state, expired historical
authority, graph/scope expansion, provider order, replay, restart, and
ambiguous publication. Tests must prove no upload or `/4` retry, and that
preparation performs no provider mutation.

Run focused Goals tests, affected race tests, documentation checks,
`git diff --check`, and the full suite. The known stale conformance attestation
and schema-16 migration failures remain outside this denominator. Do not add a
generic failed-workflow resume framework or retention system.
