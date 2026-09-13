# ADR-036: Versioned Canonical Contracts and Schema Evolution

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 has a canonical domain model, durable events, plugins, packages, graphs, preferences, behavioral profiles, and portable state. Parallel implementation cannot safely begin if each subsystem invents structurally different representations for the same concepts.

## Decision

Praxis defines versioned canonical contracts for all data that crosses durable, process, package, plugin, synchronization, or public interface boundaries.

The first contract set must cover at minimum:

- Identity and Scope;
- Goal;
- Graph and GraphVersion;
- Agent and AgentGeneration;
- Slice and SliceExecution;
- ResourceRequirement and Lease;
- Command and Query envelopes;
- DomainEvent envelope;
- Executor/Runtime binding and capability evidence;
- PackageManifest and dependency constraints;
- PreferenceContract and PreferenceRecord;
- BehavioralProfile and evidence/provenance;
- Plugin manifest/capability declarations;
- state export/import and synchronization envelopes.

### Contract rules

Every externally or durably represented contract has:

- a stable semantic name;
- explicit schema/version identifier;
- stable identifiers for referenced entities;
- documented required/optional fields;
- invariants and validation rules;
- forward/unknown-field handling where transport permits it;
- migration behavior for persisted representations.

Internal implementation structs may differ, but translation to canonical contracts occurs at the boundary. Internal convenience types must not silently become public contracts.

### Evolution

Additive compatible changes are preferred. Breaking semantic changes require a new contract version and explicit migration/adapter behavior. Persisted data is never silently reinterpreted under a new schema.

Protobuf is canonical for the gRPC plugin boundary per ADR-029. Human-authored package/graph configuration may use a text representation such as YAML or JSON, but it must validate into the same canonical semantic model rather than becoming a second ontology.

### Source of truth

Contract definitions and generated artifacts must have one authoritative source location. Generated language bindings are derived artifacts and must not be hand-edited.

## Consequences

Implementation streams can proceed independently without semantic drift. Packages, plugins, clients, synchronization, and storage can evolve while preserving explicit compatibility rules.

The initial contract work becomes a gating delivery wave because downstream code depends on it.

## Non-goals

This ADR does not require every internal function argument to use protobuf or serialized schemas.