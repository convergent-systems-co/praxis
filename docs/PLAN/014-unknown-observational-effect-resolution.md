# PLAN-014: Goals `/4` observational-effect resolution

Status: Proposed
Predecessors: PLAN-013; PLAN-012

Implement and qualify the narrow resolution contract in ADR-085 and
SPEC-048. This plan does not modify the existing UNKNOWN execution, ordinary
reconciliation, or provider adapter semantics.

The implementation denominator is:

1. Add a dedicated recovery-only resolver and CLI surface:
   `goals-publication-recovery-resolve-observation`.
2. Resolve exactly the `/4` `verify-draft` observational effect after one
   attempt, no later effects, and no abandonment.
3. Validate exact request, ActionIntent, execution, effect, parent/child
   authority lineage, temporal validity, active current authority, and both
   immutable observations.
4. Canonicalize observation equality by asset identity/name, never provider
   enumeration order. Reject swapped content, missing/extra/duplicate assets,
   altered release identity, changed lineage, and any mutation-capable step.
5. Compute a deterministic resolution identity and require exact human
   confirmation. Append a versioned resolution command/event and atomically
   invoke the state-store terminal reconciliation transition. Preserve the
   original UNKNOWN result, unresolved reconciliation event, and attempt count.
6. Make identical resolution idempotent and conflicting resolution fail
   closed. Never call adapter Dispatch or create publish/verification effects.
7. Permit later execution to consider only `publish-existing-release`, with
   fresh authority and exact current-state checks.

Focused and adversarial tests must prove positive persisted end-to-end
resolution; changed provider order with equal identity-bound semantics;
swapped content; substituted observations or reconciliation events; wrong
lineage; expired current authority; mutating steps; later effects; conflicting
or repeated resolution; dispatch not called; atomic command/event plus state
transition; restart-safe continuation; and preservation of all historical
UNKNOWN and unresolved evidence.

Run focused recovery/state/CLI tests, race tests for affected packages,
documentation validation, `git diff --check`, and the full suite. The existing
unrelated conformance attestation and schema-16 migration failures remain
outside this denominator. Do not add a generic UNKNOWN resolver, expiry
bypass, retry path, or retention framework.
