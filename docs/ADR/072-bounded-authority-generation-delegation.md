# ADR-072: Bounded Authority-Generation Delegation

- Status: Proposed — architecture-owner decision required
- Date: 2026-09-15
- Related: ADR-041, ADR-042, ADR-064, ADR-069, ADR-071
- Clarifies: the missing downstream transition identified by ADR-071/SPEC-034

## Decision requested

Add one generic, durable delegation transition to the existing authority
generation registry. The transition creates a bounded child
`AuthorityGeneration` only after an explicit governed delegation decision. It
does not add a Goals-specific authority path or make an installation root an
execution authority.

## Authority boundary

The existence of an installation root is necessary but not sufficient to mint
a child. A delegation request MUST bind the exact parent generation reference,
version, digest, requested principal, target identity, capability/operation
set, scope, expiry, and reason. The request is pending state and grants no
authority.

The existing generic `AuthorityRequest`/`AuthorityDecision` path MUST carry
the delegation request. The decision MUST bind the exact request digest and
the active parent generation. The supported authority surface MUST require an
authenticated human or separately authorized governance authority to approve
the delegation. A recommendation, package, provider, model, or WorkPlan MUST
not approve it.

The durable transition MUST atomically reload and validate the exact request,
decision, parent generation, and all referenced policy registries before
persisting the child. The child is the only downstream authority artifact;
the parent is never rewritten and is not reused as proof for child scope.

## Child-generation contract

The child generation MUST bind:

- parent reference, version, and digest;
- delegated principal and authenticated delegator;
- exact capability and operation set;
- exact target/subject constraints;
- an explicitly bounded child scope;
- issue time, expiry, and revocation/supersession lineage;
- delegation request/decision identity and digests; and
- its own content-derived generation digest.

The current v1 generation record lacks these fields. The successor MUST use a
versioned extension of the existing `AuthorityGeneration` contract or a
contract-owned envelope that the existing generation validator can resolve;
it MUST NOT create a parallel authority model or accept an unbound legacy
record as a delegated generation.

## Non-escalation

Delegation is valid only when the authority policy registries return an
explicit containment result for every requested capability, operation, target
constraint, and child scope. Undefined containment fails closed. Scope
containment MUST be supplied by a versioned scope policy/registry; core MUST
not infer hierarchy from path-like strings. Capability containment MUST be
supplied by the capability vocabulary/policy; this decision does not assume
set intersection as its semantics.

The child MUST not outlive the effective parent or any governing decision. A
parent revocation/supersession, expired decision, expired parent, or failed
policy revalidation makes the child ineffective. Child revocation is an
append-only record bound to the exact child generation and does not rewrite
history.

## Recovery and matching

Every use reloads the exact child by reference and version, verifies its
content digest, parent digest, decision binding, expiry, policy containment,
and revocation lineage. Restart recovery uses encrypted authoritative records,
never a mutable latest pointer, current work context, package metadata, or
conversation state.

The existing downstream validator remains strict: an `AuthorityDecision` must
match the exact generation reference/version/digest, principal, and granted
scope. A child scope change, capability expansion, target change, parent
change, or expiry change creates a new governed child lineage and cannot mutate
or resurrect the prior generation.

## Consequences

The root can govern a specific delegation without inheriting execution
authority. Downstream WorkPlan, package, invocation, provider, repository, and
execution authority remains represented by the exact child generation and
separate approval/lease records. This proposal is not accepted and does not
authorize bootstrapping, delegation, or qualification.
