# SPEC-052: Authority Model v6 Routing Issuance

Status: Accepted, reconciled

Authority: ADR-092 (reconciled); ADR-094; ADR-089; ADR-090

Reconciliation (2026-09-18): the routing issuance model is `praxis.authority-model`
version `v6` with digest `AuthorityModelRoutingDigest()`, the global successor
of v3, reached only by the canonical v3-to-v6 adoption ceremony (ADR-094). The "Root migration" section below
describes the superseded redesign mechanism and is retained as history; the
canonical implementation uses `ValidateBuiltinRoutingDelegation`,
`Repository.SaveRoutingIssuance` (requires adopted v6 and a v6-labelled
delegated child), and `ValidateAuthorityGenerationLineage` to the exact
current installation root. Exact-dispatch grants persist through migration
`0019_exact_dispatch_grants.sql`.

## Model identity

V2 records use `praxis.authority-model`, version `v2`, and the distinct canonical v2 digest. The v1 digest and validation table are frozen. A model version is never inferred or upcast from field similarity.

## Root migration

`AuthorityModelMigration` is immutable and content-addressed. It binds the exact active v1 installation-root ref/version/digest, exact proposed v2 root ref/version/digest, protected bootstrap digest, policy ref/version/digest, transform ID, migrating installation-owner principal, and effective time.

The transition SHALL:

1. reload and validate the sole bootstrap-derived active v1 root;
2. require an explicit installation-owner decision over the exact migration payload;
3. atomically persist the migration and v2 root while locking the v1 root;
4. return the existing successor only when the complete migration and root bytes match;
5. reject a second or conflicting successor;
6. grant the v2 root exactly `authority.delegate`, with no routing authority and no migrated descendants.

There is no automatic or transitive migration.

## V2 delegation table

The only new edges are exact, expiring controller delegations from the active v2 installation root:

```text
authority.delegate -> routing.target-contribution.issue
authority.delegate -> routing.surface-eligibility.issue
```

The requested authority, child `Authorities`, requested capabilities, operation, target kind/digest, canonical scope, policy identity, and expiry SHALL match exactly. Routing children cannot delegate and carry no runtime capability. `authority.delegate` is never accepted as either routing issuance authority.

## Validation

Every authority decision SHALL prove that its `RequestedAuthority` is present in the exact active issuing generation's `Authorities`, except a delegation decision where the generation must contain `authority.delegate` in `Capabilities`. Requested capabilities must be a subset of the generation capabilities and the closed policy table.

Generation validation recursively reloads every parent and delegation decision to the bootstrap-derived root, verifies exact digests and model envelopes, enforces scope and containment at each edge, checks effective and expiry times using core time, checks revocation and supersession at every generation and decision, and rejects cycles.

## Typed issuance

Target-contribution issuance binds one canonical contribution payload digest, target authority class, exact target scope, issuing decision/generation, effective time, and optional expiry. Surface-eligibility issuance binds one frozen route request ID, canonical surface digest, complete eligibility payload digest, issuing decision/generation, evaluation time, and expiry.

Writes are atomic with decision/generation revocation checks. Reads revalidate content, decision, generation lineage, expiry, and revocation. Authoritative merge and selection consume issuance IDs and reconstruct their inputs from protected storage. Pre-v2 caller-asserted contributions and eligibility evidence have no migration and remain non-authoritative.
