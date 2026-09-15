# ADR-073: Reachable Bounded Authority Delegation

- Status: Proposed — architecture-owner decision required
- Date: 2026-09-15
- Related: ADR-041, ADR-042, ADR-064, ADR-069, ADR-071, ADR-072
- Supersedes: the unresolved delegation-bootstrap portions of ADR-072

## Decision requested

Define the enrolled installation governance root as possessing exactly one
intrinsic governance capability: `authority.delegate`. This capability exists
only to establish a bounded downstream authority generation through the
transition below. It is not general decision authority and is not execution
authority.

The root MUST NOT implicitly possess `authority.decide`, invocation,
package-activation, repository, Goal, WorkPlan, provider, or arbitrary
capability-minting authority. Those authorities require their own governed
downstream generation or exact approval/lease.

## Reachable first-child transition

The first child is created by one generic authority transition:

1. root enrollment persists the exact immutable root generation and its
   intrinsic `authority.delegate` governance capability;
2. the authenticated root principal submits an exact delegation request;
3. the existing generic `AuthorityRequest` path persists that request as
   pending delegation state;
4. the supported authority surface creates a decision bound to the exact
   request and exact active root generation, with requested authority
   `authority.delegate`;
5. the transition verifies the root generation's intrinsic capability and
   authenticated principal, then validates containment;
6. the transition atomically persists the bounded child generation and its
   lineage.

The delegation decision is the governed exercise of `authority.delegate`; it
does not grant the root general `authority.decide`. The root principal may
approve only this exact delegation request through the authenticated root
authority surface. A model, provider, package, recommendation, WorkPlan
review, or child generation cannot issue the first delegation decision.

## Containment semantics

Containment is evaluated by a versioned authority-delegation policy registry,
not by core string heuristics. The registry MUST return an explicit
`contained`, `not_contained`, or `unknown` result for each dimension; `unknown`
fails closed.

- Principal: the child principal MUST be an authenticated, policy-eligible
  principal distinct from the root principal. Self-targeting is denied by
  default and is permitted only by an explicit policy rule that identifies the
  relationship and prevents authority class escalation.
- Capability: each child capability MUST be an allowed delegation output of
  the governing root policy. `authority.delegate` cannot delegate itself or
  any capability absent from that policy. No capability is inferred from a
  package, invocation, Goal, or requested operation name.
- Operation: each child operation MUST be explicitly allowed for the child
  capability and target by policy. The child receives no operations merely
  because the parent can delegate.
- Target: the child target kind and identity MUST satisfy an explicit typed
  target-constraint relation in the policy registry. An empty, wildcard, or
  unrecognized target constraint is not containment.
- Scope: the child scope MUST satisfy the policy registry's typed scope
  relation for the parent delegation authority. Path prefixes and lexical
  similarity are never scope containment.
- Expiry: child expiry MUST be present and no later than every applicable
  parent and delegation-decision expiry. Missing or incomparable expiry
  fails closed.
- Revocation lineage: the child MUST bind the exact parent generation and
  delegation decision. Parent or decision invalidation makes the child
  ineffective; child revocation is an append-only exact-generation record.

The policy registry and its relation version/digest MUST be recorded in the
delegation request, decision, and child generation. Policy changes do not
reinterpret an existing child silently; they require governed successor or
invalidation transitions.

## Child-generation lineage

The child generation MUST bind parent reference/version/digest, delegated
principal, capability and operation set, target constraints, typed scope,
expiry, authenticated delegator, exact request and decision identities/digests,
policy relation version/digest, state, and its own content digest. It MUST be
persisted atomically with the delegation transition and remain immutable.

The existing exact authority validator remains in force: downstream decisions
must match the child reference, version, digest, principal, and granted scope.
The root generation's scope is never changed and cannot prove child authority.

## Recovery and adversarial requirements

Restart MUST recover the exact root, request, decision, policy relation, and
child lineage from encrypted authoritative state. It MUST reject bootstrap
substitution, stale generation substitution, expired/revoked lineage,
policy-version ambiguity, root self-escalation, target widening, scope
widening, capability widening, and child records missing parent provenance.

This proposal does not authorize implementation, delegation, bootstrap, or
Japetella qualification until accepted.
