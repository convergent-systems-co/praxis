# SPEC-001: Canonical Contracts

- Status: Draft
- Governing ADRs: 002, 003, 004, 010, 014, 025, 026, 028, 029, 030, 031, 034, 035, 036

## Purpose

Define the authoritative semantic contracts shared by Praxis core, persistence, plugins, packages, synchronization, client adapters, and public interfaces.

## Required contract families

The implementation SHALL define versioned canonical contracts for:

1. `Identity`, `Scope`, and scope inheritance.
2. `Goal` and goal classification.
3. `Graph`, `GraphVersion`, node/edge definitions, entry points, triggers, and declared capabilities.
4. `Agent`, `AgentGeneration`, lineage, and graph binding.
5. `Slice`, `SliceExecution`, dependencies, state, cancellation, retry, and result.
6. `ResourceRequirement`, `ResourceLease`, capacity, priority, and deadline.
7. `CommandEnvelope` and `QueryEnvelope`.
8. `DomainEventEnvelope` including event ID, type, schema version, aggregate/entity reference, actor, timestamp, correlation ID, causation ID, and payload.
9. `ExecutorBinding`, `CapabilityClaim`, and `CapabilityEvidence`.
10. `PackageManifest`, immutable package version, dependency constraints, provenance, digest/signature metadata, and permissions.
11. `PreferenceContract`, `PreferenceSlot`, and `PreferenceRecord` including scope and provenance.
12. `BehavioralProfile`, dimensions, evidence class, confidence, provenance, and observation window.
13. `PluginManifest` and plugin capability declarations.
14. `ClientCapabilityProfile` and graph client-integration requirements.
15. state export/import/synchronization envelopes.

## Invariants

- Durable or cross-boundary entities use stable identifiers independent of display names.
- Every serialized contract has an explicit schema version.
- Unknown future fields are preserved or safely ignored where the transport supports forward compatibility.
- Breaking semantic changes create a new contract version.
- Persisted representations require explicit migration; silent reinterpretation is prohibited.
- Human-authored YAML/JSON and protobuf representations validate into the same semantic model.
- Generated bindings are derived from one authoritative schema source.
- Internal structs may differ but cannot redefine boundary semantics.

## Validation

Contract validation SHALL be deterministic and executable without an LLM. Validation errors SHALL identify the contract, field/path, violated rule, and schema version.

## Compatibility tests

The contract suite SHALL include:

- round-trip serialization tests;
- old-reader/new-writer and new-reader/old-writer tests for supported compatible versions;
- migration fixtures for every persisted breaking version;
- invalid-contract fixtures for each invariant;
- generated-binding conformance tests for each supported language.

## Deliverables

- authoritative schema source tree;
- generated Go bindings required by the core;
- protobuf contracts required by plugin/client protocol surfaces;
- JSON Schema or equivalent validation for human-authored configuration;
- compatibility/migration test corpus;
- contract reference documentation.

## Non-goals

This specification does not define graph execution algorithms or storage implementation details.