# SPEC-006: Authoritative State Provider, Event Store, and Projections

- Status: Draft
- Governing ADRs: 011, 024, 031, 032, 035, 036, 038, 040, 041, 042, 043, 047
- Depends on: SPEC-001, SPEC-002, SPEC-005

## Purpose

Define the authoritative persistence semantics for Praxis 2 independently of a specific database product: durable state, immutable event history, projections/read models, migrations, recovery, integrity, transactional authority consumption, and provider conformance.

SQLite SHALL be the initial/reference local implementation, not the architectural contract.

The persistence layer SHALL preserve enough authority and provenance state to restart safely without relying on an LLM, client session, plugin memory, or model-generated reconstruction.

## AuthoritativeStateProvider

Praxis SHALL expose semantic state-provider capabilities rather than raw database primitives. Runtime/domain code SHALL depend on the smallest semantic interface it consumes.

Provider capability groups SHALL include at least:

- command/event append with optimistic aggregate versions;
- aggregate/global replay;
- projection checkpoints;
- approval/capability-lease lookup and atomic consumption;
- effect intent/outbox/reconciliation state;
- secure encrypted-object storage;
- installed package generation and invocation-registry activation;
- run/control state derived from authoritative events;
- provider/schema/migration version metadata;
- transactional execution for invariants spanning multiple authoritative records.

The provider SHALL publish deterministic capability/consistency properties. Unknown or insufficient semantics fail closed for operations requiring stronger guarantees.

A provider is compatible only when it passes the authoritative-provider conformance suite. Implementing CRUD methods is insufficient.

## Reference provider

SQLite SHALL be the initial local provider because it supplies strong transactions, WAL/crash recovery, optimistic version enforcement, portability, and low operational overhead.

One local SQLite database per Praxis installation/profile is the default reference topology unless isolation policy requires separation.

SQLite-specific schema, WAL, locking, and migration details SHALL remain implementation concerns behind the provider boundary.

Derived workspace indexes, caches, embeddings, context packs, dashboards, and reproducible projections SHALL NOT become authoritative merely because a provider stores them.

## Logical storage topology

Every compatible provider SHALL preserve these logical responsibilities, even if physical layout differs:

1. authoritative entity/state records;
2. append-only domain event log;
3. projection/read-model state;
4. effect/outbox/reconciliation state;
5. migration/provider metadata;
6. security authority state;
7. synchronization/export metadata;
8. encrypted sensitive-object storage/references;
9. installed package generations and invocation registry.

## Transaction boundary

A local authoritative transition SHALL execute inside one atomic provider transaction when all participating authoritative records are local.

The transaction SHALL atomically include, where applicable:

- command idempotency claim/result metadata;
- entity state/version transition;
- approval consumption/change;
- capability-lease consumption/change;
- effect intent creation;
- immutable event append;
- package generation and invocation-registry activation/deactivation;
- projection checkpoint/update only when projection coupling is intentionally transactional.

External side effects SHALL NOT occur inside a local transaction in a way that assumes distributed atomicity.

If a provider cannot satisfy an operation's required atomicity, that operation/mode is unsupported rather than weakened.

## Optimistic concurrency

Mutable authoritative aggregates/entities SHALL carry a monotonic version or equivalent compare-and-set token.

Commands SHALL state expected version/precondition where stale execution would be unsafe.

A mismatched expected version SHALL fail deterministically rather than silently overwrite newer state.

## Domain event log

The authoritative event log SHALL be append-only at the application contract level.

Each event SHALL contain at minimum:

- event ID;
- monotonic sequence;
- aggregate/entity identity;
- aggregate version where applicable;
- event type/schema version;
- canonical payload;
- actor/principal identity;
- command ID;
- correlation/causation IDs;
- provenance references;
- trust/evidence classification where material;
- deterministic-runtime timestamp;
- policy/approval/lease references where applicable;
- cryptographic/integrity metadata where policy requires.

Events SHALL NOT be edited to correct history. Corrections append compensating/corrective events.

## Event integrity

The provider SHALL preserve canonical event bytes/metadata needed for integrity verification and SHOULD support tamper-evident protection for security-relevant installations.

Integrity protection SHALL use SPEC-005 profiles/suites when required. Verification failure on protected authoritative state places affected state in fail-closed/recovery-required condition.

## Command idempotency

Accepted or terminally rejected commands SHALL be recordable by stable command ID/idempotency key according to SPEC-002.

Concurrent delivery of the same idempotent command SHALL result in one authoritative transition. Recovery after commit-before-response SHALL not duplicate the command.

## Approval and anti-replay persistence

One-shot approval consumption SHALL be represented by authoritative compare-and-set/transaction semantics. Two concurrent consumers SHALL NOT both consume the same approval.

Expiry, revocation, remaining uses, intent/policy binding, and principal identity SHALL survive restart.

## Capability-lease persistence

Durable capability leases SHALL retain issuance authority, principal, scope, operation, expiry, revocation, delegation, enforcement requirements, and any exact plugin instance/session binding.

Finite-use lease consumption that protects a mutation/effect SHALL be atomic with the protected authoritative boundary whenever replay would be unsafe.

After restart, external/client enforcement conditions SHALL be revalidated rather than inferred from prior lease existence.

## Effect/outbox state

The provider SHALL persist `EffectIntent` before dispatch when an external effect requires crash-safe reconciliation.

Effect state SHALL preserve ActionIntent digest, target, capability/approval refs, idempotency, preconditions, attempts, observed result, unknown-outcome state, reconciliation evidence, and terminal disposition.

Restart SHALL reconcile ambiguous effects instead of blindly retrying non-idempotent operations.

## Projections

Projections are disposable/read-optimized representations derived from authoritative records/events.

Every projection SHALL declare version, source checkpoint, rebuild strategy, freshness semantics, and whether it is safe for security-sensitive reads.

Supported consistency classes SHALL include:

- `authoritative-inline`;
- `strong-checkpointed`;
- `eventual`.

A security-sensitive decision SHALL NOT use a projection whose checkpoint/integrity is insufficiently current.

## Sensitive state encryption

Sensitive persisted objects SHALL support application-level encryption envelopes from SPEC-005.

The provider stores ciphertext plus encryption metadata/key references rather than raw private keys/DEKs. Search/index requirements over encrypted fields require explicit designs and SHALL NOT silently duplicate sensitive plaintext.

## Package/invocation registry semantics

Installed immutable package generations and active invocation contracts are authoritative state.

Activation/update/deactivation SHALL be atomic from the user/runtime perspective. Alias collision checks, generation activation, prior-generation deactivation, and invocation-registry publication SHALL either commit together or leave the prior active generation intact.

Historical package generations/contracts required for rollback/audit SHALL remain addressable according to retention policy.

## Provider-specific migrations

Physical schema migrations are provider-specific. Canonical contract migrations and migration intent remain provider-neutral.

A provider migration SHALL define source/target provider schema versions, preflight validation, transformation, postconditions, restore strategy, protected-record handling, and compatibility constraints.

Security semantics SHALL NOT change through silent reinterpretation.

## Backup and restore

Backup SHALL capture a provider-consistent authoritative snapshot. Sensitive backup content SHALL use an appropriate SPEC-005 profile.

Restore SHALL validate provider/schema/integrity metadata and revalidate runtime-local authority before execution.

## Writer/concurrency model

Each provider SHALL declare its supported writer/concurrency model. The SQLite reference provider initially uses one authoritative-writer model with deterministic locking/serialization.

Database locking primitives SHALL NOT themselves be treated as business-level authority.

## Crash recovery

On startup Praxis SHALL:

1. validate provider/schema/migration state;
2. validate required integrity metadata;
3. establish provider writer ownership/concurrency guarantees;
4. recover/revalidate security/resource leases;
5. locate nonterminal effects and classify resume/reconciliation;
6. validate projection checkpoints or mark stale;
7. resume eligible runs according to runtime policy.

No LLM is required for startup recovery.

## Synchronization and portability boundary

Provider rows/pages/files SHALL NOT be the canonical synchronization/export format.

Portable/multi-machine state SHALL use canonical versioned envelopes with explicit authority/portability classes. Runtime-local locks, transient sessions, and one-shot authority remain non-portable unless explicitly specified otherwise.

This allows future providers to coexist without making raw SQLite copies the architecture.

## Observability

Provider telemetry SHALL include command/event latency, projection lag, migration status, provider health/capabilities, storage growth, pending/unknown effects, integrity failures, stale security projection attempts, encryption/provider failures, and provider-specific health metrics.

Telemetry SHALL avoid plaintext sensitive data.

## Acceptance tests

The implementation SHALL prove:

1. process crash after commit but before response does not duplicate an idempotent command;
2. process crash after effect intent but before dispatch recovers correctly;
3. ambiguous external dispatch enters reconciliation without blind retry;
4. projection deletion/corruption can rebuild without changing authority;
5. stale security projection cannot authorize;
6. concurrent consumers cannot both consume one-shot approval;
7. optimistic concurrency rejects stale writes;
8. provider migration preserves event/entity invariants;
9. migration failure leaves no partially reinterpreted authority;
10. encrypted sensitive fixture never appears as plaintext in generic state/event/projection storage;
11. revoked/expired lease remains so after restart;
12. restored backup revalidates runtime-local authority;
13. required event-integrity failure blocks protected execution;
14. unknown future security-relevant schema value fails safely;
15. recovery completes without invoking an LLM;
16. runtime/domain package tests can execute against a semantic provider interface without importing SQLite;
17. SQLite passes the full AuthoritativeStateProvider conformance suite;
18. a provider declaring insufficient atomicity/consistency is rejected for the affected operation;
19. canonical export/import does not require raw SQLite pages/files;
20. package generation plus invocation-registry activation is atomic and rollback-safe.

## Deliverables

- AuthoritativeStateProvider capability contracts;
- narrow semantic repository interfaces;
- provider capability/consistency profile;
- SQLite reference provider and schema/migrations;
- provider conformance suite;
- transactional command/event persistence;
- approval/lease/effect repositories;
- projection framework/checkpoints;
- encrypted-object persistence integration;
- package/invocation registry persistence;
- startup recovery procedure;
- canonical backup/export integration;
- corruption/recovery/adversarial fixtures.

## Exit criteria

SPEC-006 is complete when authoritative runtime semantics can be exercised through provider-neutral interfaces, SQLite passes the provider conformance suite, and replacing the persistence implementation does not require changes to graph/domain/package semantics.

## Non-goals

This specification does not require distributed consensus, a cloud database, arbitrary multi-writer synchronization, or any particular future state provider beyond the SQLite reference implementation.
