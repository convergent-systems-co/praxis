# ADR-074: Built-in Praxis Authority Model v1

- Status: Proposed — architecture-owner decision required
- Date: 2026-09-15
- Related: ADR-041, ADR-042, ADR-064, ADR-069, ADR-071, ADR-073
- Clarifies: the minimum authority model for the current Goals/Japetella path

## Decision requested

Adopt one closed, versioned built-in authority model for the current
installation-governance and governed WorkPlan path. General runtime policy
publication, arbitrary custom authority classes, and user-defined containment
relations remain future work.

The model identity is `praxis.authority-model/v1`. Every v1 authority request,
decision, generation, and revocation MUST carry the exact model version and
content digest. Historical records without that binding remain historical and
are not silently interpreted as v1.

## Authority vocabulary

V1 distinguishes three things:

1. Governance capabilities permit governance transitions. The only intrinsic
   installation-root capability is `authority.delegate`.
2. Governed authorities identify specific control-plane decisions. The only
   downstream v1 authority required here is `workplan.accept`.
3. Runtime capabilities are package/provider operations granted by
   `CapabilityLease`, such as `workspace.search.text`, `vcs.write`, or
   `presentation.dashboard`. They are not governance capabilities and are not
   promoted into the root by package or invocation declaration.

`authority.decide` is not a grantable capability in v1. It is the authenticated
decision act over an exact request, and its permission comes from the exact
governed authority bound to that request. `invocation.execute` is not a v1
governance authority; execution requires the dynamic InvocationContract plus
current runtime leases, enforcement, and effect mediation. `package.activate`
is a package lifecycle operation governed by its existing package transition
authority, not by the installation root.

## Canonical principals

V1 accepts only exact, source-bound principal forms:

- `installation-owner:<sha256-digest>` with kind `human`, derived from the
  protected bootstrap record;
- `controller:<stable-id>` with kind `controller`, for a durable Praxis
  control-plane actor;
- `package:<package-id>@<version>#<content-digest>` with kind `package`, for
  an exact verified package generation; and
- `plugin:<package-id>@<version>#<content-digest>/instance:<instance-id>` with
  kind `plugin`, for an authenticated runtime instance.

The ID is the identity; display names, owner labels, model output, repository
paths, and mutable aliases are not identity. `model` principals cannot issue
v1 governance decisions. Unknown kinds and malformed IDs fail closed.

## Canonical targets and scopes

V1 authority targets are protected-object references:

```text
<namespace>/<object-id>/<object-version>/<object-digest>
```

The tuple is compared component-by-component; omitted, wildcard, or mutable
latest targets are invalid. A WorkPlan acceptance target is the exact
proposal object plus its exact Goal Baseline identity and digest, represented
by the existing bound request/proposal/review records.

V1 scopes are typed values, not arbitrary strings:

- installation governance: `installation-governance:<BootstrapRecord.Digest()>`;
- WorkPlan acceptance: `goals-workplan/<goal-id>/<baseline-version>/<baseline-digest>/<proposal-version>/<proposal-digest>`;
- package lifecycle: `package/<package-id>/<package-version>/<content-digest>`;
- invocation runtime: `invocation/<package-digest>/<entry-point-id>/<graph-version>`; and
- runtime workspace/effect scopes remain lease-owned typed resources and are
  not governance-root scopes.

Only the installation scope and WorkPlan scope participate in the current
delegation path. Equality is required for a protected target's scope. No
string-prefix, wildcard, or inferred hierarchy is containment.

## Closed delegation rule

The only v1 delegation edge is:

```text
installation root [authority.delegate]
  -> controller principal
  -> governed authority: workplan.accept
  -> exact WorkPlan target and exact goals-workplan scope
```

The edge requires an exact parent generation, request, authenticated root
decision, model digest, and policy rule digest. The root cannot delegate
itself, `authority.delegate`, `authority.decide`, `invocation.execute`,
package activation, provider authority, repository authority, or arbitrary
runtime capabilities. Package/provider capability needs continue through
existing lease and package policy paths.

No other principal, authority, target, scope, capability, or operation is
contained by v1. Unknown values fail closed. A future edge requires a
successor authority-model version.

## Expiry, revocation, and recovery

Child expiry is mandatory and no later than the delegation decision expiry.
The root is not made execution-expiring by this rule. A child binds the exact
root generation, request, decision, and model digest. Root invalidation,
decision revocation, child revocation, or model supersession makes the child
ineffective according to the existing exact-generation validator and
append-only invalidation records. Descendant checks traverse the recorded
parent digest; missing ancestry fails closed.

Restart reloads exact encrypted protected objects and verifies every digest,
identity, model binding, expiry, and revocation before use. No historical
authority is migrated in place or resurrected.

## Package extensibility boundary

Dynamic packages may declare new runtime capability names and invocation
requirements. Discovery and activation remain provenance-bound but are not
authority grants. A package capability becomes usable only through its
existing verified manifest, authenticated runtime instance, applicable policy,
and explicit scoped `CapabilityLease`. It cannot become a v1 governance edge
without a successor authority-model decision.

This proposal does not authorize implementation, bootstrap, delegation, or
Japetella qualification until accepted.
