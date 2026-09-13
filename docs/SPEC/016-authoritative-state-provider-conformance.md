# SPEC-016: Authoritative State Provider Conformance

- Status: Draft
- Governing ADRs: 011, 031, 032, 035, 036, 047
- Depends on: SPEC-002, SPEC-005, SPEC-006

## Purpose

Define the provider-neutral semantic persistence boundary for Praxis authoritative state. SQLite is the default/reference provider, not the architecture.

## Core invariants

1. Runtime/domain code depends on semantic repository/service interfaces, not SQL tables or SQLite behavior.
2. A provider is eligible only when it can prove every semantic capability required by the consuming runtime mode.
3. Provider substitution SHALL NOT weaken optimistic concurrency, authority consumption, append-only history, recovery, encryption metadata, or transactional package activation.
4. Unsupported consistency/transaction guarantees fail closed rather than degrade silently.
5. Portable state uses canonical envelopes, never raw provider pages/files as the interchange contract.

## Capability profile

Each provider SHALL publish a deterministic capability profile including at least:

- `events.append_optimistic`
- `events.replay`
- `events.global_sequence`
- `projections.checkpointed`
- `authority.approval_atomic_consume`
- `authority.lease_atomic_consume`
- `effects.outbox_reconciliation`
- `secure_blobs.immutable_encrypted`
- `packages.atomic_activation`
- `runs.durable_replay`
- `migrations.versioned`
- `transactions.multi_repository_atomic`

Each capability is `enforced`, `unsupported`, or `unknown`; `unknown` is treated as unsupported for authoritative operations.

## Semantic interfaces

Praxis SHALL expose narrow interfaces owned by consuming subsystems. At minimum the provider layer must support:

### Event authority

- append expected aggregate version + ordered events atomically;
- reject stale aggregate versions;
- load aggregate event stream;
- read global sequence/checkpoint ranges.

### Authority repositories

- load approval/lease by immutable ID;
- atomically consume bounded uses;
- preserve expiry/revocation/version state across restart;
- support combined transactional mutation when a security invariant spans authority consumption and the protected state change.

### Effects

- persist effect intent before dispatch where reconciliation is required;
- classify pending/unknown/completed effects;
- update reconciliation evidence without rewriting historical events.

### Secure objects

- persist ciphertext/envelope metadata immutably by object/version;
- preserve crypto profile/suite/key reference and integrity metadata;
- never require plaintext duplication for provider operation.

### Package/runtime registry

- persist immutable package generations;
- atomically replace active package generation + invocation/graph/agent registrations;
- preserve prior generations for rollback according to retention policy.

### Projections/checkpoints

- monotonic checkpoint update;
- explicit projection version/consistency class;
- stale/version-regressing writes fail deterministically.

## Transaction composition

Where Praxis requires multiple semantic repositories in one authority boundary, the provider SHALL expose a transaction/unit-of-work abstraction. A provider that cannot satisfy the required atomicity cannot support that execution mode.

No generic `Put/Get` abstraction may be used as a substitute for required security semantics.

## SQLite reference provider

The SQLite provider SHALL implement the complete local profile with:

- foreign keys enabled;
- WAL mode;
- bounded busy timeout;
- secure delete where supported;
- migration ledger;
- optimistic aggregate versions;
- one authoritative writer model/serialized authority writes as required;
- application-level encrypted secure blobs.

SQLite-specific pragmas/schema/migrations remain implementation details of this provider.

## In-memory test provider

A deterministic in-memory provider MAY implement a declared subset for unit tests. Tests requiring omitted semantics SHALL fail setup rather than accidentally pass with weaker behavior.

## Provider selection

Provider selection is deployment configuration. Domain graphs/packages SHALL NOT encode provider identity.

Before startup enters authoritative execution, Praxis SHALL compare required runtime capabilities against the selected provider profile and fail closed on mismatch.

## Acceptance tests

1. SQLite passes the full local provider conformance suite;
2. a fake provider missing optimistic concurrency is rejected for event mutation;
3. a fake provider missing atomic lease consumption is rejected for protected side effects;
4. provider swap does not alter canonical event/replay results;
5. package activation remains atomic through provider interface;
6. encrypted blob metadata round-trips through provider interface without plaintext persistence;
7. projection checkpoint regression fails identically across conforming providers;
8. canonical export/import does not expose raw SQLite pages as required format;
9. runtime/domain package tests can use provider interfaces without importing the SQLite driver;
10. `unknown` provider capabilities fail closed.

## Exit criteria

A second deterministic test provider can run the provider-neutral conformance fixture, while SQLite remains the default production/local provider, and runtime services do not require schema-specific knowledge outside the SQLite implementation layer.
