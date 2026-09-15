# SPEC-034: Installation Governance-Root Scope

Status: Proposed — pending architecture-owner decision

Authority: ADR-071

## Contract

The first active installation governance generation MUST use the exact scope:

```text
installation-governance:<BootstrapRecord.Digest()>
```

`BootstrapRecord.Digest()` MUST be computed from the canonical non-secret
bootstrap metadata. The scope MUST preserve the `sha256:` digest form and MUST
not be normalized from Goal, project, repository, package, invocation,
provider, qualification, or user-entered work labels.

The installation principal MUST remain:

```text
installation-owner:<BootstrapRecord.Digest()>
```

Enrollment MUST persist one immutable version-1 `AuthorityGeneration` whose
principal, reference, scope, provenance digest, and effective time are
consistent with that binding. It MUST NOT create a WorkPlan,
AuthorityRequest, AuthorityDecision, capability lease, provider grant,
repository grant, package approval, invocation authority, or external effect.

## Delegation

Downstream authority MUST be represented by an exact delegated generation or
equivalent governed authority record. It MUST bind the parent installation
generation, target kind/identity, requested capability and operation, narrower
scope, authenticated delegator, expiry, and revocation lineage.

The root generation MUST NOT be reused as proof for a downstream scope. A
decision is valid only when its `AuthorityRef`, version, generation digest,
principal, and granted scope match the exact active generation. This preserves
the current `ValidateAuthorityGeneration` equality rule rather than weakening
it.

## Lifecycle and recovery

Root and delegated generations MUST be encrypted, immutable, digest-verified,
and reloadable after restart. Root scope changes require successor enrollment
lineage; they MUST NOT rewrite or resurrect the prior root. Delegated scope
changes require a new delegated generation or decision lineage.

## Required adversarial qualification

Qualification MUST reject work-specific root scopes, bootstrap-digest
substitution, stale generation references, root-scope widening, root reuse for
downstream work authority, duplicate roots, mutable latest-pointer recovery,
and any enrollment path that creates execution authority.
