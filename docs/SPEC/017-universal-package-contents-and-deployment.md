# SPEC-017: Universal Package Contents and Deployment

- Status: Draft
- Governing ADRs: 003, 004, 013, 020, 021, 025, 041, 046, 048
- Depends on: SPEC-004, SPEC-007, SPEC-010, SPEC-011, SPEC-015

## Purpose

Define one immutable package format that can distribute graphs, persistent-agent definitions, domain assets, invocation contracts, preferences, and optional executable plugins without forcing every reusable artifact into plugin code.

## Core invariants

1. Package is the universal distribution/install/update/rollback unit.
2. Plugin is one executable content type inside a package, not the package system.
3. Graph-only and agent-definition-only packages require no executable plugin.
4. Every content item is typed, versioned, digest-bound, and referenced from the immutable manifest.
5. Installing content registers definitions; it does not grant runtime authority.
6. Agent definition installation does not overwrite local agent identity, memory, lineage, or learned state.
7. Package update preserves prior immutable generations for rollback according to policy.
8. A content reference is deployable only when its exact digest-bound bytes were verified from the signed package archive and durably retained; a type/ID/path row alone is not implementation evidence.

## Content reference

Each package content item SHALL include:

- `kind`: graph, agent_definition, plugin, invocation, preference_contract, behavioral_profile, template, migration, documentation, or extension-defined compatible kind;
- stable `id`;
- content/schema version;
- immutable digest;
- artifact path/ref inside the package;
- compatibility range where applicable;
- optional dependency references.

Unknown security-relevant content kinds fail closed. Unknown non-executable informational kinds may be ignored only when compatibility policy explicitly allows it.

## Graph contents

A graph content item SHALL bind graph ID/version/digest and canonical graph artifact.

Activation SHALL register the exact immutable graph generation. Subgraph references resolve by graph ID/version through the graph registry, never mutable filesystem discovery.

Uninstall disables new resolution of removed graph generations but SHALL preserve historical run references needed for replay/audit according to retention policy.

## Agent-definition contents

An agent definition SHALL describe the bootstrap/template contract for creating a local persistent agent identity, including graph binding, supported memory/profile contract, default preference seed, and requested capabilities.

Instantiation creates a new local governed identity/generation. Multiple identities may be instantiated from one installed definition.

Instantiation authority binds the exact installed package generation, definition content digest, definition-declared graph set, new local agent/generation IDs, owner scope, and governance reference. SQLite rechecks those immutable bytes and active graph bindings, consumes the intent-bound approval, and appends the initial agent event in one transaction. Restart reconstructs the same independent identities through the normal agent event runtime.

Updating an agent-definition package SHALL NOT automatically migrate active agent generations. Migration/adoption is an explicit governed operation preserving lineage.

## Plugin contents

Executable plugin contents SHALL include verified artifact digest, protocol compatibility, executable/launch metadata, isolation requirements, capability advertisement constraints, and platform compatibility.

Activation of the package does not grant the plugin any capability lease. Runtime plugin activation remains governed by SPEC-007.

## Invocation contents

Invocation contracts are first-class package contents. Active contracts are registered atomically with their owning package generation according to SPEC-015.

Aliases/options/help/completion derive from these contracts; core CLI contains no domain-specific command table.

## Catalog metadata

Distribution adapters SHALL expose enough metadata to filter/discover packages by content type without executing package artifacts. Catalog metadata may advertise graph IDs, agent definitions, plugin providers, entry-point aliases, domain tags, compatibility, capabilities, enforcement requirements, and crypto profile.

Catalog metadata is discovery evidence, not trust or authority.

## Install transaction

After artifact verification and authorization review, activation SHALL atomically publish the applicable registration set:

- installed immutable package generation;
- graph definitions;
- agent definitions;
- invocation contracts/aliases;
- preference/profile metadata;
- plugin definitions as inactive/available providers;
- package dependency lock.

The same transaction SHALL consume the exact intent-bound local approval and persist a content-addressed activation receipt. A bare manifest, signature-valid package, caller-selected trust enum, or command-line acceptance flag is not activation authority.

If any required content fails validation or registration collision checks, the previous active generation remains unchanged.

The install transaction SHALL retain both the signed archive bytes and each manifest-selected content body. On resolution after restart, the runtime SHALL verify the stored body against the immutable content digest before decoding or executing it. Archive paths are canonical relative paths; traversal, links/special files, duplicate paths, missing declared files, and unmanifested regular files fail closed.

## Update

Update SHALL compare old/new:

- content set and digests;
- requested/transitive capabilities;
- enforcement requirements;
- crypto profile;
- invocation aliases/options;
- graph/agent compatibility;
- plugin protocol/isolation requirements;
- state migration requirements.

Capability/enforcement/crypto expansion follows reauthorization policy. Existing persistent agent identities are never replaced merely because their source definition updated.

## Uninstall

Uninstall SHALL remove/deactivate discoverable active registrations and prevent new execution from removed content. Historical event/run/agent lineage needed for audit/replay remains unless separately deleted under explicit retention policy.

Disable and uninstall are authoritative mutations. Each binds an exact installed generation and operation to a persisted local approval, consumes that approval atomically with registry removal, and retains an immutable transition receipt across restart. A caller supplying only a package ID cannot disable or remove content.

## GitHub Releases packaging profile

Initial release assets SHOULD contain:

- canonical `praxis-package.json` manifest;
- immutable package artifact/archive;
- digest metadata;
- signature envelope(s) according to policy;
- dependency lock where dependencies exist.

Future distribution adapters consume the same package format.

## Acceptance tests

1. install and invoke a graph-only package with no plugin executable;
2. install an agent-definition-only package and create two independent local identities;
3. update the agent-definition package and prove existing identities/memory are unchanged;
4. install a mixed graph+plugin package and prove plugin authority remains lease-gated;
5. activation collision leaves prior generation/registrations intact;
6. uninstall removes active graph/invocation discovery while historical run replay remains possible;
7. catalog discovery can filter graph-only, agent-only, plugin-only, and mixed packages;
8. subgraph resolution pins exact installed graph version/digest;
9. package update cannot silently expand capabilities or weaken cryptographic profile;
10. core CLI contains no imports of domain packages to expose their commands.

## Exit criteria

Fixture packages for graph-only, agent-only, and mixed graph+plugin content can be discovered, installed, activated, updated, rolled back, and uninstalled through the common package lifecycle without recompiling Praxis core.
