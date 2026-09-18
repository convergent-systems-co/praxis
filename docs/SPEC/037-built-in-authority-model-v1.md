# SPEC-037: Built-in Praxis Authority Model v1

Status: Proposed — pending architecture-owner decision

Authority: ADR-074

## Model envelope

Every v1 authority-bearing record MUST contain:

```text
authority_model = praxis.authority-model
authority_model_version = v1
authority_model_digest = <exact model digest>
```

The digest covers the canonical vocabulary and closed delegation table. A
record missing this envelope is legacy evidence, not a v1 authority grant.

## Canonical vocabulary

Governance capability: `authority.delegate` only for the enrolled installation
root. Governed authority: `workplan.accept` only for the current downstream
path. Runtime capabilities remain `CapabilityLease` values and may be dynamic
package-defined names; they are never inferred to be governance authority.

`authority.decide`, `invocation.execute`, and `package.activate` are distinct
control/effect operations, not intrinsic root capabilities. They require their
existing exact authority, package, lease, approval, and mediation paths.

## Typed v1 delegation containment

A delegation is contained only when every predicate below is true:

```text
parent == active installation generation by exact ref/version/digest
decider == parent principal == authenticated installation human
requested authority == authority.delegate
child principal == exact controller:<stable-id>
child authority == workplan.accept
target == exact protected Goal/Baseline/Proposal/Review identity tuple
scope == canonical goals-workplan scope derived from those exact digests
operation == accept
child expiry > now and <= decision expiry
model identity/version/digest == v1 exact values
parent and decision revocation lineage is effective
```

The child principal MUST NOT equal the root principal. No other capability,
operation, target, scope, principal kind, or authority class is recognized in
v1. Unknown values fail closed. This is an explicit closed table, not set
intersection or lexical scope matching.

## Runtime capability boundary

Invocation contracts and verified package manifests may declare runtime
capabilities such as workspace, research, VCS, presentation, inference, or
provider capabilities. Those declarations are discoverability and eligibility
inputs. A current authenticated lease with exact principal, operation, typed
resource scope, expiry, revocation, enforcement, and package/instance binding
is still required at runtime. No v1 governance generation mints such leases.

## Persistence and recovery

The existing encrypted protected-object store remains authoritative. A v1
child generation MUST bind parent generation, request, decision, target,
scope, expiry, model envelope, and revocation lineage with its own digest.
Revocation and supersession remain append-only and exact-generation-bound.
Restart MUST reload and revalidate the complete lineage; stale, substituted,
expired, revoked, model-mismatched, or legacy records fail closed.

Historical open-string authority records are retained unchanged and require
explicit successor semantics before use as v1 authority. No automatic
migration is defined by this specification.

General policy publication, arbitrary scope hierarchies, custom authority
classes, and package-defined governance edges are explicitly parked.
