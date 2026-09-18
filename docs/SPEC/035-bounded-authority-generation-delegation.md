# SPEC-035: Bounded Authority-Generation Delegation

Status: Proposed — pending architecture-owner decision

Authority: ADR-072

## Required transition

The implementation MUST expose one generic supported delegation transition
owned by the authority package/control plane. It MUST operate on the existing
encrypted `AuthorityGeneration`, `AuthorityRequest`, and `AuthorityDecision`
registries and MUST be discoverable through the supported CLI/control surface.

The transition MUST reject a missing parent, stale parent digest, unapproved
request, non-active/revoked/expired parent, mismatched decision, ambiguous
policy result, or any request that lacks exact target, capability, operation,
scope, expiry, and principal identity.

## Request and decision binding

The delegation request MUST include:

```text
parent_ref, parent_version, parent_digest,
delegated_principal,
target_kind, target_identity,
requested_capabilities, requested_operations,
requested_scope, expires_at, reason
```

The request MUST be represented by the generic `AuthorityRequest` contract or
an exact versioned extension of it. Its requested authority identifies
delegation, not execution. The human/governance decision MUST bind the exact
request identity/version/digest, parent generation identity/version/digest,
deciding principal, granted capabilities/operations/target/scope, outcome,
authority evidence, issue time, and expiry.

## Child generation

The child MUST be an immutable successor generation with its own digest and
the following durable fields:

```text
parent_generation_ref/version/digest
delegated_principal
capabilities and operations
target constraints
scope
issued_at / expires_at
delegation_request_ref/version/digest
delegation_decision_ref/version/digest
delegated_by
state and revocation/supersession lineage
```

The parent generation remains unchanged. The child MUST NOT be represented by
changing the root scope or by copying the root as an unbound authority.

## Containment and revocation

Before persistence, versioned authority policy MUST explicitly establish
containment for each capability, operation, target constraint, and scope. An
unknown or unsupported containment relation MUST fail closed. Scope hierarchy
MUST NOT be inferred from string prefixes. The chosen capability and scope
containment semantics are an architecture-owner decision under ADR-072.

The child MUST expire no later than the effective parent/delegation decision,
and parent revocation/supersession MUST invalidate effective child authority.
Child revocation MUST be append-only, exact-generation-bound, and durable.

## Restart qualification

After restart, the control plane MUST recover the same parent and child
identities, verify all digests and bindings, re-evaluate expiry/revocation,
and continue only with effective authority. It MUST reject stale generation
substitution, bootstrap-digest substitution, root reuse for a child scope,
capability widening, target widening, mutable-pointer recovery, and
conversation/provider/package-supplied authority.

This specification does not authorize implementation or installation
bootstrap until ADR-072 is accepted.
