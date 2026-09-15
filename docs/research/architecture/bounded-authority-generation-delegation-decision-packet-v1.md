# Bounded Authority-Generation Delegation Decision Packet v1

Status: Proposed — architecture-owner decision required

Date: 2026-09-15

## Finding

The accepted architecture has an encrypted immutable v1
`AuthorityGeneration`, generic `AuthorityRequest` and `AuthorityDecision`
records, exact generation/principal/scope matching, expiry on decisions, and
append-only generation invalidation. It has no production `authority delegate`
surface and no delegated-generation record or transition.

The existing `CapabilityLease` supports capability, operation, scope, expiry,
revocation, and use limits, but does not bind an issuer, parent generation,
delegation permission, target constraints, or delegation lineage. Existing
plugin/delegation language is architectural guidance and does not supply this
missing authority-generation transition.

## Minimum proposed successor

Accept ADR-072 and SPEC-035 as the smallest generic successor architecture.
It adds a governed delegation request/decision transition and a versioned
child-generation binding while retaining existing exact decision validation,
encrypted persistence, invalidation, and WorkPlan acceptance boundaries.

The owner must choose and record the authoritative containment semantics for
capabilities, operations, target constraints, and scopes. No implementation
may infer containment from path-like strings or use set intersection by
assumption.

## Advisory challenge

Architecture challenge: root existence must not mint arbitrary authority;
request/decision must bind the exact parent; child authority must be narrower
under authoritative policy; parent revocation must affect descendants; and
restart must recover immutable lineage. ADR-072/SPEC-035 address each as a
required invariant and explicitly leave containment policy to the owner.

Security challenge: model/provider/package output, WorkPlan review, bootstrap
identity, or a copied root scope cannot authorize delegation. Undefined
containment, stale/expired/revoked lineage, target substitution, and mutable
pointer recovery fail closed.

## Decision requested

The architecture owner must either ACCEPT ADR-072/SPEC-035 with the stated
containment-policy decision, or reject/revise them and specify the canonical
generic delegation contract. This packet does not accept the proposal and
does not authorize bootstrap, implementation, or Japetella qualification.
