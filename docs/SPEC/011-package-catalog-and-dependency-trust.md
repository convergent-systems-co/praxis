# SPEC-011: Package, Catalog, and Dependency Trust

- Status: Draft
- Governing ADRs: 013, 020, 021, 025, 027, 036, 040, 041, 043
- Depends on: SPEC-001, SPEC-005, SPEC-007

## Purpose

Define installable Praxis packages, immutable identity, dependency resolution, provenance/signatures, transitive capability review, updates, rollback, forks, and catalog transport without treating distribution location or signature validity as execution authority.

## Core invariants

1. Package identity binds immutable manifest plus artifact/content digest.
2. A valid signature proves integrity/publisher association, not safety or authorization.
3. Installation and execution authorization are separate operations.
4. Requested capabilities are deny-by-default and aggregated transitively.
5. Updates cannot silently broaden capabilities, weaken crypto, or replace local descendants.
6. Catalog transport is replaceable; GitHub is an initial transport, not root of trust.
7. Local modification creates a new descendant identity/digest while preserving lineage.

## Package manifest

A canonical manifest SHALL include stable package ID/version, package type, graph/agent/process contents, immutable content digest, publisher/provenance, signature envelope(s), dependency constraints/lock data, required/optional capabilities, required enforcement properties, client integration entry points, preference contract, behavioral profile metadata, compatibility range, cryptographic minimum/profile, migration hooks/metadata, and lineage/upstream references where applicable.

## Dependency lock

Execution SHALL resolve against an immutable dependency lock containing exact package IDs/versions/digests. Floating dependency refs MAY be used during resolution but SHALL NOT define an active installation.

The resolver SHALL detect cycles, incompatible version constraints, duplicate/conflicting identities, and digest mismatches.

## Transitive authority

The effective capability request of a package is the union of direct and transitive requested capabilities, reduced by local policy/grants. A dependency cannot obtain capability merely because the parent package was approved for installation.

Any update that broadens the transitive capability set or required enforcement properties creates a new authorization decision.

## Trust dimensions

Evaluation SHALL keep integrity, provenance, capability risk, compatibility, evidence quality, local trust decision, cryptographic strength, and vulnerability/advisory status separate rather than collapsing them to one trusted boolean.

## Signing

Distributed packages SHOULD use post-quantum-capable signatures per SPEC-005; `pq-preferred` is the Praxis-native default target. Package policy MAY require `pq-required` or hybrid signatures.

Signatures SHALL cover the canonical manifest, immutable content digest, dependency lock digest, capability declarations, enforcement requirements, and cryptographic requirements.

## Install lifecycle

Install states SHOULD distinguish discovered, downloaded, verified, inspected, authorized, installed, active, disabled, updating, rolled_back, locally_modified, and removed.

Verification alone SHALL NOT transition to authorized/active.

## Update and rollback

An update SHALL pin a new immutable package identity and run compatibility/migration checks before activation. Capability expansion, cryptographic downgrade, incompatible state migration, or changed enforcement requirements SHALL require explicit policy/user authorization.

Rollback SHALL restore a previously verified compatible package/state generation. It SHALL NOT reactivate expired/revoked runtime leases or one-shot approvals.

## Local descendants/forks

Any local modification changes content digest and creates descendant lineage. Upstream updates MAY be evaluated/merged/migrated but SHALL NOT overwrite a local descendant silently.

## Catalog

Catalog entries are discovery metadata referring to immutable package artifacts. Popularity, stars, repository ownership, or catalog presence are not execution trust.

Private/local/organization catalogs SHALL use the same package identity and trust semantics.

## Acceptance tests

1. modified package bytes fail digest/signature verification;
2. validly signed malicious fixture receives no capability grant automatically;
3. dependency capability expansion is surfaced before activation;
4. update with broader permissions requires new authorization;
5. `pq-required` signature policy rejects classical-only artifact;
6. local modification creates distinct descendant identity;
7. rollback does not restore consumed approval/runtime lease;
8. floating dependency resolves to immutable lock before execution;
9. dependency cycle/conflict fails deterministically;
10. catalog transport change does not alter package trust semantics.

## Deliverables

- package manifest schema;
- dependency lock format/resolver;
- digest/signature verifier;
- transitive capability analyzer;
- install/update/rollback state machine;
- lineage/local-descendant model;
- catalog adapter interface and GitHub initial adapter;
- adversarial package/dependency fixtures.
