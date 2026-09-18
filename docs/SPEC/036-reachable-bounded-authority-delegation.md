# SPEC-036: Reachable Bounded Authority Delegation

Status: Proposed — pending architecture-owner decision

Authority: ADR-073

## Root capability

The first active installation governance generation MUST include exactly the
intrinsic governance capability `authority.delegate`. This is a governance
capability only. It MUST NOT satisfy or imply general `authority.decide`,
execution, invocation, package activation, repository, Goal, WorkPlan,
provider, or arbitrary capability-minting authority.

The root capability is usable only by the authenticated root principal through
the supported authority surface and only for an exact delegation request.

## Delegation request

The generic request MUST carry a typed delegation payload containing:

```text
parent_ref, parent_version, parent_digest,
delegated_principal,
target_kind, target_identity, target_constraints,
requested_capabilities, requested_operations,
requested_scope, expires_at, reason,
policy_ref, policy_version, policy_digest
```

The request MUST identify `authority.delegate` as its requested authority,
remain pending until decided, and bind the exact root generation when the
root is the parent. The authenticated root principal is the only first-child
decider supplied by clean enrollment; no nonexistent external authority may
be assumed.

## Decision and atomic child creation

The supported authority surface MUST create an `AuthorityDecision` that binds
the exact request identity/version/digest, exact active parent generation
reference/version/digest, authenticated deciding principal, requested and
approved delegation fields, policy relation identity, outcome, issue time,
and expiry.

The child-generation transition MUST atomically reload and validate the exact
request, decision, parent, and policy relation before persisting the child.
It MUST reject an approval that is conversational, copied, stale, expired,
revoked, self-issued by an unauthorized principal, or mismatched on any
delegation field.

## Normative containment

Containment is a policy-registry operation over typed values, not a lexical
operation over strings. The registry MUST return one of `contained`,
`not_contained`, or `unknown`; only `contained` permits child creation.

The policy relation MUST establish all of the following:

```text
principal_allowed(parent_policy, child_principal)
capability_allowed(parent_policy, child_capability)
operation_allowed(child_capability, child_operation, target)
target_contained(parent_policy, child_target_constraints)
scope_contained(parent_policy, child_scope)
expiry_contained(parent_expiry, decision_expiry, child_expiry)
revocation_lineage_bound(parent, decision, child)
```

No relation may be implemented as a string-prefix scope check, wildcard
default, naïve set intersection, or inference from package/invocation/Goal
identity. An undefined relation fails closed. The policy relation's exact
version and digest are durable provenance on request, decision, and child.

By default, a child principal equal to the root is not permitted. Any
self-targeted exception MUST be an explicit policy rule and MUST still pass
all capability, operation, target, scope, expiry, and lineage checks.

## Child generation and recovery

The child MUST be an immutable generation with its own digest and exact
parent, principal, capabilities, operations, target constraints, scope,
expiry, request, decision, delegator, policy relation, and revocation lineage.
It MUST expire no later than its parent or decision. Parent/decision
revocation or supersession MUST make the child ineffective.

After restart, the system MUST recover and verify the complete lineage and
continue only when every bound record and policy relation remains effective.
No mutable latest pointer, package metadata, provider output, or conversation
may substitute authority.

This specification does not authorize implementation or bootstrap until
ADR-073 is accepted.
