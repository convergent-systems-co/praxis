# ADR-071: Installation-Derived Governance-Root Scope

- Status: Proposed — architecture-owner decision required
- Date: 2026-09-15
- Supersedes: none
- Clarifies: ADR-069, ADR-064

## Decision requested

Adopt an installation-derived scope for the first governance generation. The
installation root must not inherit the scope of the Goal, package, invocation,
repository, or qualification that happens to trigger enrollment.

## Proposed semantics

The canonical root scope is:

```text
installation-governance:<bootstrap-record-digest>
```

The suffix is the exact non-secret `BootstrapRecord.Digest()` value, including
its `sha256:` prefix. The value is derived from the protected bootstrap
binding's canonical metadata, not from a Goal, repository, package, invocation,
operator-provided label, or current work item.

The root principal remains the existing deterministic identity:

```text
installation-owner:<bootstrap-record-digest>
```

The scope and principal are related by the same protected installation digest,
but they remain distinct identities. The existing generation reference is also
installation-bound:

```text
installation-governance:<bootstrap-record-digest>
```

This proposal makes that existing reference the canonical root-scope value.

## Authority boundary

Root enrollment creates exactly one immutable version-1 `AuthorityGeneration`
with the installation-derived scope. It creates no capability lease and grants
no implicit permission to install packages, invoke providers, access a
repository, execute a Goal, accept a WorkPlan, or mutate an external system.

The root may govern bounded downstream delegation only through a separate,
durable authority transition. A delegated generation must bind:

- the exact parent root generation reference, version, and digest;
- the exact downstream target kind and identity;
- a narrower explicit scope;
- the requested capability/operation set;
- expiry and revocation lineage; and
- the authenticated delegating principal.

Work-, package-, invocation-, provider-, and repository-specific authority
belongs to the delegated generation or exact approval/lease records. It must
never be represented by changing the root generation's scope.

Because the current validator requires a decision's granted scope to equal the
scope of its exact authority generation, a downstream decision cannot simply
reuse the root generation while claiming a narrower scope. The delegation
record/generation boundary is therefore required before this architecture can
qualify end-to-end.

## Matching and recovery

Every use reloads the exact generation by reference and version, verifies its
digest and effective state, and verifies the scope equality. Restart must
reconstruct the same root generation from encrypted authoritative state; it
must not derive a new scope from current Goal or repository context.

Changing the protected bootstrap binding, installation identity, or canonical
root scope requires a new installation enrollment/generation lineage. It may
not mutate version 1 or resurrect an invalidated generation. A change to a
downstream work scope creates or updates only the governed delegated lineage.

## Adversarial cases

The following must fail closed:

1. enrolling with `goals-lifecycle:qualification` as the root scope;
2. replacing the bootstrap digest while retaining the old root reference;
3. using a Goal ID, package digest, repository path, or invocation ID as root
   scope;
4. reusing the root generation for a narrower downstream decision;
5. accepting a decision whose granted scope differs from its generation scope;
6. creating a second root for the same installation or a competing root for a
   different bootstrap binding;
7. recovering a root from a mutable latest pointer or conversational state;
8. treating root enrollment as package, provider, repository, or execution
   authority.

## Consequence and implementation boundary

This is a successor architecture proposal, not an implementation. ADR-069 and
SPEC-032 remain unchanged until accepted. The minimum implementation follow-up
is a separately owned delegated-generation contract and its atomic persistence,
validation, revocation, and restart evidence. No qualification command may use
a work-specific root scope before this proposal is accepted.
