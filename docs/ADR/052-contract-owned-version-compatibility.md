# ADR-052: Contract-Owned Version Compatibility

- Status: Accepted
- Date: 2026-09-13

## Context

Praxis persists events and other contracts that may outlive the executable that created them. A consumer-local check such as passing current and historical version strings into a replay helper distributes semantic history across call sites. It cannot cleanly distinguish readable history, deterministic migration, intentionally unsupported experiments, revoked unsafe formats, and unknown future data. It also makes upgrades depend on finding every procedural branch.

ADR-036 requires explicit compatibility and migration behavior. ADR-051 applies that rule to the adaptive v2 boundary. A reusable model is required so compatibility policy belongs to the contract rather than the code path consuming it.

## Decision

Praxis defines a first-class `ContractVersionPolicy` and runtime `VersionRegistry`. A policy names the contract and its current version and assigns every recognized version exactly one disposition:

- `current`: canonical and readable as-is;
- `supported_historical`: recognized and readable as-is under the contract's documented semantics;
- `migratable`: readable only through a named deterministic upcaster targeting the current canonical representation;
- `unsupported_pre_release`: recognized implementation history that never acquired a durable compatibility commitment;
- `revoked_unsafe`: recognized but forbidden because its semantics cannot be trusted or safely interpreted.

An absent version is unknown and fails closed. Version names are opaque; ordering or a larger numeric suffix implies nothing.

A migratable definition binds a versioned upcaster identity and target. The canonicalized policy is content-addressed. Runtime migration records that policy digest, the contract, source/target versions, upcaster identity, and input/output content digests. Upcasters must be deterministic, provenance-preserving, explicitly tested, and non-lossy wherever the governing contract requires it. Old bytes are never unmarshaled directly into current semantics merely because fields happen to align.

Static contract policy is validated once and exposed read-only. Consumers ask their named registry to canonicalize an event/object version; they do not supply current versions or historical lists. Adding a version changes the contract policy and, when required, its registered migration—not arbitrary replay call sites.

Contracts that form one owning family SHALL register their explicit named policies in a `VersionCatalog` (or an equivalently strict owner). The catalog admits one policy per contract identity. Duplicate declarations—including identical duplicates—fail closed so apparently idempotent setup cannot conceal distributed semantic ownership. A helper that accepts a contract name and current version is not an authoritative definition when it can recreate policies independently; version literals remain legitimate inside the one named policy that owns the contract.

## Initial application and audit

Adaptive observation, profile, measurement, and analysis event replay now use named registries. Observation/profile v1 disposition is defined once as `unsupported_pre_release`; current versions are obtained from the same policy for freeze, validation, append, and replay.

The client invocation policy records an earlier repository split explicitly: package-owned records used both `v1` and `1` with the same schema semantics before one compatibility owner existed. `v1` is the canonical current write version and `1` is supported historical state. This is compatibility metadata at the owning definition, not consumer-selected leniency; unknown forms still fail closed.

The package-family catalog also owns distinct single-generation activation, verified-closure deployment, generation-transition, and closure-rollback intent policies. Deployment and rollback contracts are not inferred from call-site strings: their current write versions and future compatibility/migration dispositions have one named owner, while consumers construct instances from that metadata.

Package-installed agent definitions and their instantiation intents are owned by one agent-contract catalog. Package-definition consumers obtain the current version from that catalog and unknown persisted definition versions fail closed; graph identity versions inside a definition remain immutable content references rather than being mistaken for schema compatibility policy.

A repository review found additional durable/versioned surfaces that need explicit policy ownership as their contracts evolve:

- package signature envelopes currently compare one constant at verification;
- encrypted envelopes currently compare one version and cipher suite during open;
- conformance attestations currently validate one literal schema version;
- kernel continuation/checkpoint records carry versions whose compatibility behavior must remain tied to their owning contract;
- package manifests, graph/agent definitions, preference records, portable-state envelopes, and plugin protocol metadata need registry or protocol-native compatibility declarations before supporting historical formats.

Graph/package/agent *identity versions* and event-store aggregate sequence versions are not automatically schema versions and must not be forced into this abstraction. SQLite provider schema upgrades already have a centralized migration mechanism and should integrate at its boundary rather than at every query. Cryptographic suite negotiation/revocation may require richer policy than byte-schema upcasting; adopting this registry must not flatten those security semantics.

This audit is an adoption map, not evidence that the remaining surfaces are already migrated. Their historical/release commitments must be established independently before assigning dispositions or writing upcasters.

## Consequences

- Replay compatibility is centralized, deterministic, auditable, and fail-closed.
- Conflicting declarations for one contract identity fail at registration rather than creating multiple semantic truths.
- Recognized does not imply readable.
- Pre-release rejection and security revocation remain distinct.
- Migration behavior has explicit identity and evidence.
- Contract-specific policies can grow without scattering version literals through consumers.
- This architectural cleanup closes no original-intent finding without independent lifecycle evidence.
