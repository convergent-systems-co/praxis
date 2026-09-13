# SPEC-011: Package, Catalog, and Dependency Trust

- Status: Draft
- Governing ADRs: 003, 004, 013, 020, 021, 025, 027, 036, 040, 041, 043, 046, 048
- Depends on: SPEC-001, SPEC-004, SPEC-005, SPEC-007, SPEC-015

## Purpose

Define installable Praxis packages as the universal distribution unit for graphs, agent definitions, plugins, domain assets, preferences, and invocation contracts, with immutable identity, dependency resolution, provenance/signatures, transitive capability review, updates, rollback, forks, and replaceable catalog transport.

Distribution location or signature validity SHALL NOT be treated as execution authority.

## Core invariants

1. Package identity binds immutable manifest plus artifact/content digest.
2. A valid signature proves integrity/publisher association, not safety or authorization.
3. Installation and execution authorization are separate operations.
4. Requested capabilities are deny-by-default and aggregated transitively.
5. Updates cannot silently broaden capabilities, weaken crypto, replace local descendants, or overwrite persistent agent identity/history.
6. Catalog transport is replaceable; GitHub Releases is the initial adapter, not root of trust.
7. Local modification creates a new descendant identity/digest while preserving lineage.
8. Graphs and agent definitions are first-class package contents and SHALL NOT require executable plugin code.
9. Plugins are executable provider contents carried by packages, not the package/distribution architecture itself.
10. Active CLI/client entry points derive from installed package `InvocationContract`s under SPEC-015.

## Package content model

A canonical package manifest SHALL include stable package ID/version, immutable content digest, publisher/provenance, signature envelope(s), dependency constraints/lock data, requested capabilities, required enforcement properties, compatibility range, cryptographic minimum/profile, migration metadata, and lineage/upstream references.

The manifest SHALL also enumerate typed package contents by immutable identity/digest. Supported content classes include at least:

- graph definitions;
- persistent-agent definitions/bootstrap templates;
- executable plugin/provider definitions;
- `InvocationContract`s;
- preference contracts/profile seeds;
- domain-specific templates/models/spec assets;
- documentation/examples;
- provider/package migrations where allowed.

Pure graph/agent packages SHALL require no native executable plugin.

## Graph package contents

A graph content entry SHALL identify graph ID/version/digest, compatible runtime/contract version, required dependencies, declared capability requirements, and optional invocation entry points.

Installing an active graph package SHALL register the immutable graph definition in the graph registry and publish applicable invocation contracts atomically with package activation.

Installation never grants the graph additional execution authority.

## Agent package contents

An agent content entry SHALL define an immutable agent definition/template including definition ID/version/digest, graph binding, bootstrap/default preference/profile metadata, compatibility requirements, and declared capability requests.

Installing an agent package registers the definition/template only. Instantiating a persistent agent SHALL create a governed local identity/generation bound to that immutable definition.

Two installations/instances from the same definition SHALL have independent identity, memory, learning state, grants, and lineage.

Updating an agent-definition package SHALL NOT silently replace or rewrite existing agent identity/history. Adoption/migration to a newer definition occurs through governed lineage/generation semantics.

## Plugin package contents

Executable plugin contents follow SPEC-007 for identity, protocol negotiation, isolation, supervision, capability advertisement, runtime-session binding, leases, and failure containment.

Package verification/installation SHALL NOT activate ambient plugin authority.

## Invocation contents

User-facing graph/agent/package entry points SHALL be represented by versioned `InvocationContract`s bound to the exact package generation.

Activation/deactivation/update of package entry points follows SPEC-015. The core CLI SHALL NOT contain package-specific command definitions.

## Dependency lock

Execution SHALL resolve against an immutable dependency lock containing exact package IDs/versions/digests. Floating refs MAY be used during discovery/resolution but SHALL NOT define active execution.

The resolver SHALL detect cycles, incompatible constraints, duplicate/conflicting identities, and digest mismatches.

Dependencies MAY supply graphs, agents, plugins, templates, or mixed content without changing lock semantics.

## Transitive authority

The effective capability request of a package is the union of direct and transitive requested capabilities, reduced by local policy/grants. A dependency cannot obtain capability merely because the parent package was installed or authorized for different capabilities.

Any update that broadens transitive capabilities or required enforcement properties creates a new authorization decision before activation.

## Trust dimensions

Evaluation SHALL keep integrity, provenance, capability risk, compatibility, evidence quality, local trust decision, cryptographic strength, vulnerability/advisory status, and executable-code presence separate rather than collapsing them into one trusted boolean.

Pure declarative graph/agent content SHOULD be distinguishable from packages that include executable plugins.

## Signing

Distributed packages SHOULD use post-quantum-capable signatures per SPEC-005; `pq-preferred` is the Praxis-native default target. Policy MAY require `pq-required` or hybrid signatures.

Signatures SHALL cover the canonical manifest, immutable content digest, dependency lock digest, typed content inventory/digests, capability declarations, enforcement requirements, invocation contracts, and cryptographic requirements.

## Install lifecycle

Install states SHOULD distinguish discovered, downloaded, verified, inspected, authorized, installed, active, disabled, updating, rolled_back, locally_modified, and removed.

Verification alone SHALL NOT transition to authorized/active.

Activation SHALL publish all typed registries required by the package (graph, agent-definition, plugin definition, invocation) atomically from the runtime/user perspective.

## Update and rollback

An update SHALL pin a new immutable package generation and run compatibility/migration checks before activation.

Capability expansion, cryptographic downgrade, incompatible state migration, changed enforcement requirements, or changed executable-plugin presence SHALL require policy/user review as defined by governing specs.

Rollback SHALL restore a previously verified compatible package generation and its typed registrations/invocation surface. It SHALL NOT reactivate consumed/expired/revoked approvals or runtime leases and SHALL NOT rewind persistent agent identity/history.

## Local descendants/forks

Any local modification changes content digest and creates descendant lineage. Upstream updates MAY be evaluated/merged/migrated but SHALL NOT overwrite a local descendant silently.

## Catalog and discovery

Catalog entries are discovery metadata referring to immutable package artifacts. Popularity, stars, repository ownership, catalog presence, or release publication are not execution trust.

Catalog metadata SHOULD expose:

- package ID/version/publisher/description/tags;
- typed content classes/counts;
- graph IDs/entry points;
- agent-definition IDs/behavioral profile metadata;
- executable plugin presence/providers;
- dependencies/compatibility;
- requested capabilities/enforcement requirements;
- crypto/signature/provenance data.

Discovery SHALL be able to filter packages by content class such as graph, agent, plugin, or mixed package.

GitHub Releases is the first transport under SPEC-015. Private/local/organization/future catalogs SHALL use the same canonical package identity/trust semantics.

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
10. catalog transport change does not alter package trust semantics;
11. pure graph package installs and becomes invocable without executable plugin code;
12. pure agent-definition package installs and can instantiate two independent governed local agents;
13. agent package update does not overwrite existing agent memory/identity;
14. mixed graph+plugin package enforces plugin isolation independently of graph registration;
15. package update atomically replaces graph/agent/invocation registrations;
16. catalog discovery can filter graph/agent/plugin package contents;
17. uninstall removes active registrations while preserving historical run/agent lineage according to retention policy.

## Deliverables

- universal typed package manifest schema;
- graph/agent/plugin content descriptors;
- graph registry and agent-definition registry integration;
- dependency lock format/resolver;
- digest/signature verifier;
- transitive capability analyzer;
- install/update/rollback state machine;
- typed registry activation/deactivation transaction;
- lineage/local-descendant model;
- replaceable distribution/catalog adapter interface and GitHub Releases initial adapter;
- dynamic invocation registration integration;
- adversarial package/dependency fixtures.
