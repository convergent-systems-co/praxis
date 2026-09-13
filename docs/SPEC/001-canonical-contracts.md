# SPEC-001: Canonical Contracts

- Status: Draft
- Governing ADRs: 002, 003, 004, 010, 014, 025, 026, 028, 029, 030, 031, 034, 035, 036, 037, 038, 040, 041, 042, 043

## Purpose

Define the authoritative semantic contracts shared by Praxis core, persistence, plugins, packages, synchronization, client adapters, public interfaces, and security enforcement.

These contracts are the stable boundary between deterministic Praxis behavior and replaceable implementations. A model, plugin, client, or package may propose values for a contract but SHALL NOT redefine the contract semantics.

## Required contract families

The implementation SHALL define versioned canonical contracts for:

1. `Identity`, `Principal`, `Actor`, `Scope`, and scope inheritance.
2. `Goal` and goal classification.
3. `Graph`, `GraphVersion`, node/edge definitions, entry points, triggers, and declared capabilities.
4. `Agent`, `AgentGeneration`, lineage, and graph binding.
5. `Slice`, `SliceExecution`, dependencies, state, cancellation, retry, and result.
6. `ResourceRequirement`, `ResourceLease`, capacity, priority, quota class, and deadline.
7. `CommandEnvelope` and `QueryEnvelope`.
8. `DomainEventEnvelope` including event ID, type, schema version, aggregate/entity reference, actor, timestamp, correlation ID, causation ID, payload, and provenance reference.
9. `ExecutorBinding`, `CapabilityClaim`, `CapabilityEvidence`, and `CapabilityLease`.
10. `PackageManifest`, immutable package version, dependency constraints, provenance, digest/signature metadata, permissions, requested capabilities, and cryptographic requirements.
11. `PreferenceContract`, `PreferenceSlot`, and `PreferenceRecord` including scope and provenance.
12. `BehavioralProfile`, dimensions, evidence class, confidence, provenance, and observation window.
13. `PluginManifest`, `PluginPrincipal`, isolation requirements, and plugin capability declarations.
14. `ClientCapabilityProfile`, `ClientEnforcementProfile`, and graph client-integration requirements.
15. `InvocationContract`, normalized invocation envelope, and run reference.
16. `ProvenanceRecord`, `TrustClass`, `EvidenceClass`, and derivation ancestry.
17. `ActionIntent`, `ApprovalGrant`, `ApprovalBinding`, `EffectIntent`, and `EffectResult`.
18. `CryptoProfile`, `CryptoSuiteRef`, `KeyReference`, `SignatureEnvelope`, `EncryptionEnvelope`, and algorithm/provider metadata.
19. state export/import/synchronization envelopes including portability class and authority class.
20. `SensitiveDataLabel`, release/destination policy reference, and redaction/exclusion metadata.

## Stable identity rules

Durable or cross-boundary entities SHALL use stable opaque identifiers independent of display names, aliases, file paths, provider names, or client-local IDs.

Where an entity has mutable versions, identity and version SHALL be separate fields. Version transitions SHALL preserve lineage where defined by the governing ADR.

Aliases SHALL be resolved to stable identity before an authoritative command, event, approval, signature, or effect intent is produced.

## Provenance and trust contracts

Every external or derived datum capable of influencing execution, memory, learning, routing, authorization inputs, context selection, or user-visible evidence SHALL be representable with provenance.

`TrustClass` SHALL distinguish at least:

- `authoritative_local`
- `user_confirmed`
- `policy_derived`
- `verified_external`
- `untrusted_external`
- `model_generated`
- `derived_deterministic`
- `derived_inferred`
- `unknown`

Trust classification is descriptive input to policy. It is not itself authorization.

A derived record SHALL preserve ancestry sufficient to identify its material source records. Repeated copies of the same source SHALL NOT be represented as independent corroboration.

Untrusted content SHALL NOT contain a field capable of directly granting permissions, approvals, capability leases, trust promotion, policy mutation, or durable authority.

## Capability lease contract

`CapabilityLease` SHALL contain at minimum:

- lease ID;
- principal ID;
- capability ID and version;
- permitted operation(s);
- scope/resource constraints;
- issuance authority;
- issued-at and optional expiry;
- revocation state/reference;
- delegation policy;
- maximum use count when applicable;
- correlation/purpose binding where applicable;
- enforcement requirements.

Possession of a capability declaration or technical ability SHALL NOT imply possession of a capability lease.

Unknown, expired, revoked, out-of-scope, or unverifiable leases SHALL be denied.

## Action intent and approval contract

`ActionIntent` SHALL be a canonical, hashable representation of a proposed authoritative mutation or external side effect.

It SHALL include enough data to prevent approval ambiguity, including as applicable:

- action type/version;
- actor/requesting principal;
- target identity and target version/state precondition;
- normalized parameters;
- effect scope;
- requested capability leases;
- destination/resource identifiers;
- content digest or artifact digest where the effect depends on specific bytes;
- policy version/reference;
- correlation ID;
- expiry/deadline;
- idempotency semantics.

An `ApprovalGrant` SHALL bind to either:

1. the digest of one exact `ActionIntent`; or
2. an explicit bounded policy describing a set of permitted intents.

Material mutation of the intent SHALL invalidate exact approval.

One-shot approvals SHALL carry anti-replay state.

## Cryptographic contract rules

Cryptographic algorithms SHALL NOT be embedded as unversioned free-form strings in durable semantics.

`CryptoProfile` SHALL express policy intent such as:

- `classical-compatible`
- `pq-preferred`
- `pq-required`
- `hybrid-high-assurance`

`CryptoSuiteRef` SHALL identify algorithm family, standardized identifier/version, parameter set/security category, provider implementation identity/version where relevant, and deprecation status.

Current Praxis-native preference SHALL support standardized post-quantum families including ML-KEM for key establishment, ML-DSA for signatures, and SLH-DSA as a supported hash-based signature alternative. Exact algorithm availability is resolved through policy and provider capability, not hard-coded into graphs.

A `pq-required` contract SHALL fail closed when the required post-quantum property cannot be satisfied. No silent downgrade is allowed.

Hybrid profiles SHALL encode both constituent algorithms and verification requirements so that one side cannot be silently omitted.

Key references SHALL identify key purpose, owner/principal, version, lifecycle status, storage/provider reference, creation time, expiry/rotation metadata, and allowed crypto profiles. Secret key material SHALL NOT be serialized into generic contract records.

## Serialization and schema evolution

- Every serialized contract SHALL have an explicit schema version.
- Unknown future fields SHALL be preserved or safely ignored where the transport supports forward compatibility.
- Breaking semantic changes SHALL create a new contract version.
- Persisted representations SHALL require explicit migration; silent reinterpretation is prohibited.
- Human-authored YAML/JSON and protobuf representations SHALL validate into the same semantic model.
- Generated bindings SHALL derive from one authoritative schema source.
- Internal structs may differ but SHALL NOT redefine boundary semantics.
- Security-relevant defaults SHALL be explicit in canonicalized form before signing, hashing, authorization, or persistence.

## Canonicalization

Contracts used for hashing, signatures, approvals, idempotency, content addressing, or equality SHALL define deterministic canonical serialization.

Canonicalization SHALL define at minimum:

- field ordering or canonical encoding rules;
- treatment of absent versus default values;
- normalized identifiers;
- string/Unicode normalization where applicable;
- timestamp representation;
- numeric encoding;
- map/set ordering;
- excluded non-semantic metadata.

Two semantically equal canonical contracts SHALL produce the same canonical digest.

## Validation

Contract validation SHALL be deterministic and executable without an LLM.

Validation errors SHALL identify the contract, field/path, violated rule, and schema version.

Security-sensitive validation SHALL fail closed on unknown enum values, unsupported crypto requirements, ambiguous principal identity, unverifiable authority, malformed provenance ancestry, or missing required enforcement properties.

## Compatibility tests

The contract suite SHALL include:

- round-trip serialization tests;
- canonicalization/digest golden fixtures;
- old-reader/new-writer and new-reader/old-writer tests for supported compatible versions;
- migration fixtures for every persisted breaking version;
- invalid-contract fixtures for each invariant;
- generated-binding conformance tests for each supported language;
- capability lease expiry/revocation/scope fixtures;
- trust/provenance ancestry fixtures;
- exact approval-binding and mutated-intent rejection fixtures;
- cryptographic profile downgrade-rejection fixtures;
- hybrid signature/encryption envelope fixtures;
- unknown-security-field fail-closed fixtures.

## Deliverables

- authoritative schema source tree;
- generated Go bindings required by the core;
- protobuf contracts required by plugin/client protocol surfaces;
- JSON Schema or equivalent validation for human-authored configuration;
- canonicalization library and test vectors;
- compatibility/migration test corpus;
- security contract fixtures;
- contract reference documentation.

## Exit criteria

SPEC-001 is implementation-ready when:

1. all required contract families have schemas with stable IDs and versions;
2. canonical serialization is defined for every signed/hashed/approved object;
3. trust/provenance cannot encode authority escalation;
4. capability leases are scopeable, expirable, and revocable;
5. exact approvals reject modified intents;
6. PQ-required and hybrid cryptographic profiles have deterministic downgrade behavior;
7. generated Go/protobuf/JSON bindings pass the same conformance corpus.

## Non-goals

This specification does not define graph execution algorithms, storage implementation details, operating-system sandbox mechanics, or concrete cryptographic library selection.