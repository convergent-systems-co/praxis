# SPEC-015: Portable State and Multi-Machine Reconciliation

- Status: Draft
- Governing ADRs: 011, 024, 036, 040, 042, 043
- Depends on: SPEC-001, SPEC-005, SPEC-006, SPEC-013, SPEC-014

## Purpose

Define local-first export/import and multi-machine reconciliation without copying SQLite rows, replaying stale runtime authority, or requiring a cloud control plane.

## Core invariants

1. Local SQLite remains authoritative for one installation; synchronization exchanges canonical envelopes, not database rows.
2. Runtime-local locks, client sessions, one-shot approvals, and ephemeral capability leases are non-portable by default.
3. Policy/trust conflicts fail closed rather than last-write-wins.
4. Provenance, lineage, tombstones/supersession, and cryptographic protection survive synchronization.
5. Offline changes can coexist and reconcile without silently discarding conflict.
6. Sensitive state is encrypted for intended recipients using SPEC-005 profiles.

## Portability classes

Every synchronizable record type SHALL declare one of:

- `portable_mergeable`: may reconcile by defined deterministic merge semantics;
- `portable_versioned`: moves as immutable generations/versions with explicit active-selection conflict handling;
- `portable_single_writer`: import requires no conflicting authoritative writer or an explicit migration/ownership transfer;
- `local_ephemeral`: never exported as reusable authority;
- `local_sensitive_reference`: only provider/key references may transfer; material does not;
- `nonportable`: excluded unless a future contract explicitly enables migration.

## Export envelope

A `StateEnvelope` SHALL include envelope/version ID, source installation/device principal, creation sequence/time, included record families and schema versions, portability class per record, provenance/lineage, conflict/vector/version metadata, sensitivity classification, cryptographic envelope/signatures, and integrity digest.

## Authority exclusions

At minimum, the following SHALL NOT become portable reusable authority merely through export/import:

- one-shot approvals;
- active runtime capability leases;
- local process/resource locks;
- client session authorization;
- in-flight external-effect dispatch authority;
- cached host enforcement claims;
- raw private keys/credentials by default.

Historical records about those items MAY be portable as non-authoritative audit evidence.

## Merge semantics

Merge behavior SHALL be declared by record family. Immutable lineage records union by identity/digest. Preferences use scoped/source precedence while preserving conflicts/history. Memory preserves records and supersession/tombstone semantics. Agent generations union lineage and require explicit active-generation resolution on divergent heads. Policy/trust configuration conflicts require fail-closed/manual or higher-authority resolution unless deterministic policy says otherwise.

## Conflict representation

Unresolved conflicts SHALL be first-class records containing competing values/versions, provenance, source devices, timestamps/sequences, affected scope, security significance, and allowed resolution mechanisms.

Conflict is not resolved by hiding one value.

## Offline behavior

Machines MAY continue local work within locally valid authority while disconnected. Authority that depends on remote/current organizational state SHALL declare freshness requirements and fail when required freshness cannot be proven.

## Cryptography

Exports containing sensitive state SHALL use `pq-preferred` by default for Praxis-native recipients, with `pq-required`/hybrid profiles when policy requires. Recipient identities/key references are bound to encryption envelopes. Downgrade requires explicit policy and audit evidence.

## Import

Import SHALL validate envelope integrity/signature, sender/source identity/provenance, schema compatibility/migration, cryptographic profile, replay/sequence constraints, portability class, local conflicts, and policy before committing any authoritative change.

Import is a command through the canonical mutation boundary.

## Replay resistance

Envelope IDs and source sequence/version metadata SHALL detect duplicate/replayed imports. Older envelopes cannot overwrite newer revocation/tombstone/security state without an explicit reconciliation rule.

## Acceptance tests

1. duplicate state envelope import is idempotent;
2. one-shot approval exported as audit record cannot be reused on destination;
3. runtime capability lease is excluded/non-authoritative on destination;
4. divergent agent generations create explicit head conflict rather than overwrite;
5. preference records reconcile with scope/source precedence while preserving lineage;
6. policy/trust conflict fails closed;
7. stale envelope cannot resurrect revoked/tombstoned state;
8. sensitive export fails when `pq-required` recipient support is unavailable;
9. offline local work does not imply remote authority freshness;
10. synchronization works without direct SQLite row copying.

## Deliverables

- portability-class registry;
- StateEnvelope schema;
- export/import commands;
- record-family reconciliation interfaces;
- replay ledger;
- conflict model/resolution workflow;
- encrypted export integration;
- offline/freshness policy hooks;
- multi-machine adversarial fixtures.
