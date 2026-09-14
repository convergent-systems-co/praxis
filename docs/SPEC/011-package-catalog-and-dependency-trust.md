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
11. Verification evidence is content-addressed and binds the exact downloaded manifest bytes, artifact bytes, signature envelope, verified dependency evidence, transitive capability request, enforcement request, source, and time. A caller-authored `verified` field cannot mint it.
12. Local activation authority binds the exact verification evidence and reviewed capability/enforcement surface. The authority is consumed in the same transaction that records the durable activation receipt and publishes registries.
13. Archive verification binds every typed content reference to exact regular-file bytes, rejects missing, mismatched, duplicate-path, unsafe-path, special-file, and unmanifested archive entries, and preserves those bytes as evidence rather than interpreting registration metadata as deployed behavior.
14. Installation activates the complete verified dependency closure in deterministic dependency-first order under one closure-bound local authorization and one database transaction. A missing, extra, reordered, lock-mismatched, or verification-lineage-mismatched package fails closed; a later collision rolls back earlier dependency writes and authority consumption.

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

The package definition is versioned immutable data containing exact graph ID/version bindings and optional bootstrap preference metadata. Instantiation SHALL re-resolve the active definition bytes and every bound graph inside the authoritative transaction, compare the exact package/definition/graph/local-identity intent, consume one persisted approval, and append the first agent generation atomically. A caller-selected principal, agent ID, generation ID, graph, owner scope, or governance reference cannot mint confirmed creation evidence. Generic event append may retain an observation but SHALL NOT label it `user_confirmed` without the authority boundary.

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

A lock MAY carry a source-kind/source-reference locator pair so a replaceable catalog adapter can retrieve the pinned bytes. The locator is transport metadata, not package identity or trust. Automatic resolution requires both fields and fails closed when the adapter is unavailable; pre-resolved verification may omit them only when the caller already supplies verifier-minted packages matching every lock.

The resolver SHALL detect cycles, incompatible constraints, duplicate/conflicting identities, and digest mismatches.

Resolution walks dependency-first in deterministic package-identity order, rechecks every adapter result against the signed exact ID/version/content digest, verifies every signature and typed artifact, and binds the transitive verified evidence/capability surface into the root verification. Dependency traversal derives from the manifest bytes covered by the signature. If an adapter also supplies a decoded convenience manifest, semantic disagreement with those bytes fails closed rather than creating a second identity, dependency, capability, or enforcement authority. Different catalog transports use the same resolver and trust boundary. A catalog response, source locator, or popularity signal cannot override a lock or mint verification.

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

The runtime activation boundary SHALL accept a verifier-minted package capability plus an exact intent-bound approval. It SHALL NOT accept a bare manifest. Replay SHALL retain the original manifest bytes, signature envelope, verification evidence, activation intent, authority identity, and approval identity so the decision remains auditable after restart.

Activation SHALL publish all typed registries required by the package (graph, agent-definition, plugin definition, invocation) atomically from the runtime/user perspective.

When typed contents exist, the signed artifact SHALL be a gzip tar whose regular-file surface exactly matches the manifest content inventory. Core validates canonical relative paths, archive entry type, per-content digest, and inventory completeness without assigning domain meaning to the bytes. Activation persists the verified archive and individual content bytes atomically. Restart resolution rechecks the content digest before decoding a graph/agent/plugin-specific contract; metadata-only registration is not executable package evidence. Pre-release rows created before artifact retention cannot manufacture missing bytes during migration and therefore fail closed if selected for execution.

Package signature, verification-evidence, and activation-intent formats are durable versioned contracts governed by first-class version registries. Compatibility behavior belongs to that metadata; unknown or rejected versions fail closed and consumers do not carry historical-version lists.

The package contract family owns one catalog of explicit policies for package manifests, signature envelopes, verification evidence, single-generation activation intents, closure deployment intents, disable/remove transition intents, and closure rollback intents. The generic client-contract family separately owns `InvocationContract` schema compatibility. Repository history contains semantically identical package-owned invocation records using both `v1` and `1` before that ownership was centralized; the policy designates `v1` as current and `1` as supported historical state rather than rewriting persisted package data. Package/version and graph/version fields in a manifest are immutable content identities, not schema-version policy. Install-state vocabulary and activation/transition/rollback receipt tables evolve through the centralized SQLite schema migration boundary; they SHALL NOT be misclassified as independently upcastable byte contracts. Adding v2 to any durable package record changes its one owning policy and optional named upcaster, never a consumer-local helper argument.

## Update and rollback

An update SHALL pin a new immutable package generation and run compatibility/migration checks before activation.

Capability expansion, cryptographic downgrade, incompatible state migration, changed enforcement requirements, or changed executable-plugin presence SHALL require policy/user review as defined by governing specs.

Rollback SHALL derive a dependency-first target closure from retained immutable manifests and bind both the complete current-generation precondition set and exact target IDs/versions/digests to distinct local authority. Before mutation, retained manifest/artifact/signature/verification/content/invocation evidence is revalidated. The entire closure changes atomically, prior active generations remain as `rolled_back` history, aliases follow only the restored generation, and the authority-bound receipt replays after restart. Rollback SHALL fail rather than change a dependency generation required by an unaffected active package, and SHALL NOT restore consumed approvals, leases, or runtime authority.

Rollback SHALL restore a previously verified compatible package generation and its typed registrations/invocation surface. It SHALL NOT reactivate consumed/expired/revoked approvals or runtime leases and SHALL NOT rewind persistent agent identity/history.

## Local descendants/forks

Any local modification changes content digest and creates descendant lineage. Upstream updates MAY be evaluated/merged/migrated but SHALL NOT overwrite a local descendant silently.

## Catalog and discovery

Catalog entries are discovery metadata referring to immutable package artifacts. Popularity, stars, repository ownership, catalog presence, or release publication are not execution trust.

The governed catalog-contribution operation SHALL accept only a transfer-lifecycle-minted published artifact whose generalized bytes match its immutable digest. A successful result SHALL contain the package-policy-mapped generalized content and a reserved safe provenance content item binding the transfer evaluation/publication and mapping-policy identities. Those items are selected by kind, stable ID, and version; manifest position is not semantic. Other valid package operations may produce an empty typed-content inventory, and compatible contribution profiles may add safe content, so neither global non-emptiness nor an exact contribution item count is a package-manifest invariant. Construction fails closed when required semantic content or its exact archive bytes are absent.

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
