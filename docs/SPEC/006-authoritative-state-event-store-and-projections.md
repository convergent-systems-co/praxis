# SPEC-006: Authoritative State, Event Store, and Projections

- Status: Draft
- Governing ADRs: 011, 024, 031, 032, 035, 036, 038, 040, 041, 042, 043
- Depends on: SPEC-001, SPEC-002, SPEC-005

## Purpose

Define the local authoritative persistence model for Praxis 2: SQLite-backed durable state, immutable event history, projections/read models, migration, recovery, and integrity rules.

The persistence layer SHALL preserve enough authority and provenance state to restart safely without relying on an LLM, client session, plugin memory, or model-generated reconstruction.

## Authority model

SQLite SHALL be the initial local authoritative store for Praxis 2.

The database SHALL persist canonical authoritative state required to determine:

- graph/agent/run/slice lifecycle;
- package/version/lineage state;
- principals and trusted identity references;
- policy references and policy versions;
- capability leases and revocation state;
- approvals and anti-replay/consumption state;
- ActionIntent/effect lifecycle;
- provenance/trust metadata;
- cryptographic profile/suite/key references;
- synchronization/export metadata;
- event sequence and projection checkpoints.

Derived workspace indexes, caches, embeddings, context packs, dashboards, and other reproducible projections SHALL NOT become authoritative merely because they are stored in SQLite or another local database.

## Storage topology

The implementation SHOULD use one local SQLite database per Praxis installation/profile unless isolation policy requires separate stores.

The logical schema SHALL separate at least:

1. authoritative entity/state tables;
2. append-only domain event log;
3. projection/read-model tables;
4. effect/outbox/reconciliation state;
5. migration/schema metadata;
6. security authority state;
7. synchronization/export metadata;
8. optional encrypted sensitive-object storage or references.

Physical table layout may evolve, but the logical responsibilities SHALL remain distinct.

## Transaction boundary

A local authoritative transition SHALL execute inside one SQLite transaction when all mutated authoritative records are local.

The transaction SHALL atomically include, where applicable:

- command idempotency claim/result metadata;
- entity state/version transition;
- approval consumption/change;
- capability-lease state change when part of the same local action;
- effect intent creation;
- immutable event append;
- projection checkpoint/update only when projection coupling is intentionally transactional.

External side effects SHALL NOT occur inside a database transaction in a way that assumes distributed atomicity.

## Optimistic concurrency

Mutable authoritative aggregates/entities SHALL carry a monotonic version or equivalent compare-and-set token.

Commands SHALL state expected version/precondition where stale execution would be unsafe.

A mismatched expected version SHALL fail deterministically rather than silently overwrite newer state.

## Domain event log

The event log SHALL be append-only at the application contract level.

Each event SHALL contain at minimum:

- event ID;
- global/local monotonic sequence;
- aggregate/entity identity;
- aggregate version where applicable;
- event type and schema version;
- canonical payload;
- actor/principal identity;
- command ID;
- correlation ID;
- causation ID where applicable;
- provenance reference(s);
- trust/evidence classification where material;
- timestamp generated/accepted by deterministic runtime boundary;
- policy/approval/lease references for security-sensitive transitions where applicable;
- cryptographic/integrity metadata where policy requires.

Events SHALL NOT be edited to correct history. Corrections SHALL append compensating/corrective events.

## Event integrity

The persistence layer SHALL detect accidental corruption and SHOULD support tamper-evident integrity for security-relevant installations.

The initial design SHALL support an integrity chain or equivalent mechanism over canonical event records without making one specific cryptographic construction part of event semantics.

When enabled, integrity protection SHALL use SPEC-005 profiles/suites and record verification status.

Failure to validate required integrity SHALL place affected authoritative state in a fail-closed/recovery-required condition.

## Command idempotency

Accepted or terminally rejected commands SHALL be recordable by stable command ID/idempotency key according to SPEC-002.

Concurrent delivery of the same idempotent command SHALL result in one authoritative transition.

The idempotency record SHALL permit safe recovery after a crash between command acceptance and caller response.

## Approval and anti-replay persistence

One-shot approval consumption SHALL be represented by authoritative state protected by compare-and-set/transaction semantics.

Two concurrent consumers SHALL NOT both successfully consume the same one-shot approval.

Approval expiry, revocation, remaining-use count, intent/policy binding, and principal identity SHALL survive restart.

Imported/synchronized approvals SHALL follow the portability classification defined by the portable-state specification; runtime-local one-shot authority SHALL not become portable by accident.

## Capability-lease persistence

Durable capability leases SHALL retain issuance authority, principal, scope, operation, expiry, revocation, delegation, and enforcement requirements.

Ephemeral client/runtime capability leases MAY be stored separately or reconstructed only when their governing contract explicitly marks them non-portable and non-durable.

After restart, Praxis SHALL NOT assume that an external/client enforcement condition still exists merely because a prior lease record exists. Required external conditions SHALL be revalidated.

## Effect/outbox state

The database SHALL persist `EffectIntent` before dispatch when an external effect requires crash-safe reconciliation.

Effect state transitions SHALL follow SPEC-002 and preserve:

- ActionIntent digest;
- target adapter/principal;
- capability/approval references;
- idempotency key;
- target preconditions;
- dispatch attempts;
- observed response/result;
- unknown-outcome state;
- reconciliation evidence;
- terminal disposition.

A restart SHALL resume/reconcile pending or unknown effects without blindly reissuing non-idempotent operations.

## Projections

Projections are disposable/read-optimized representations derived from authoritative records/events.

Every projection SHALL declare:

- projection type/version;
- source event/entity sequence/checkpoint;
- rebuild strategy;
- freshness semantics;
- whether it is safe for security-sensitive reads.

A security-sensitive decision SHALL NOT use a projection whose checkpoint/integrity cannot be proven sufficiently current for that decision.

Projection corruption or deletion SHALL be recoverable by rebuilding from authoritative source.

## Projection consistency classes

Praxis SHALL support at least:

- `authoritative-inline`: updated in the same transaction as the authoritative transition;
- `strong-checkpointed`: projection proves it has processed through a required authoritative sequence;
- `eventual`: suitable for dashboards/search/analytics but not authority decisions unless policy explicitly tolerates staleness.

The required class SHALL be explicit at each consuming boundary.

## Sensitive state encryption

Sensitive persisted objects SHALL support application-level encryption envelopes defined by SPEC-005.

The database SHALL store ciphertext plus `EncryptionEnvelope` metadata/key references rather than raw private keys/DEKs.

Fields requiring indexing/search while encrypted SHALL use an explicit design; the implementation SHALL NOT silently duplicate sensitive plaintext into an index for convenience.

## Secrets and credentials

Credentials and private keys SHOULD remain in dedicated secure stores/providers and be referenced by opaque IDs.

If sensitive secret material must be stored by Praxis, it SHALL use an approved encryption profile and be excluded from events, generic logs, projections, model context, and export by default.

## Migration

Schema migrations SHALL be explicit, ordered, versioned, and transactional where possible.

A migration SHALL define:

- source schema version(s);
- target schema version;
- preflight validation;
- transformation;
- post-migration invariants;
- rollback/restore strategy;
- handling of encrypted/cryptographically protected records;
- compatibility with older binaries where relevant.

Security semantics SHALL NOT be changed by a migration through silent reinterpretation.

Before irreversible migration, Praxis SHOULD create a recoverable backup/snapshot according to local policy.

## Backup and restore

Backup SHALL capture a transactionally consistent authoritative state.

Sensitive backups SHALL use an appropriate SPEC-005 encryption profile.

Restore SHALL validate database/schema/integrity metadata before permitting authoritative execution.

Restored runtime-local leases/approvals/effects SHALL undergo revalidation/reconciliation before use.

## Multi-process ownership

The first implementation SHALL define one authoritative writer model unless/until a stronger concurrent-writer design is specified.

If multiple local processes access the store, writer ownership/locking SHALL be deterministic and crash-recoverable.

SQLite locking behavior SHALL NOT be treated as business-level authority or capability enforcement.

## Crash recovery

On startup Praxis SHALL:

1. validate schema/migration state;
2. validate required database/integrity metadata;
3. establish authoritative writer ownership;
4. recover/revalidate resource/security leases as applicable;
5. locate nonterminal effects and classify for resume/reconciliation;
6. validate projection checkpoints or mark stale;
7. resume eligible graph/run state according to runtime policy.

No LLM is required for startup recovery.

## Synchronization boundary

Local database rows SHALL NOT be copied directly between machines as the synchronization contract.

Portable/multi-machine state SHALL export canonical versioned envelopes with explicit authority/portability classes.

Runtime-local locks, ephemeral leases, transient client sessions, and one-shot approvals SHALL be excluded unless a later specification explicitly defines safe portability semantics.

## Observability

Persistence telemetry SHALL include:

- command transaction latency;
- event append latency;
- projection lag;
- migration status;
- database size/growth;
- WAL/checkpoint health where applicable;
- pending/unknown effects;
- integrity verification failures;
- stale security projection attempts;
- encryption/provider failures.

Telemetry SHALL avoid plaintext sensitive data.

## Acceptance tests

The implementation SHALL prove:

1. process crash after authoritative commit but before response does not duplicate an idempotent command;
2. process crash after effect intent but before dispatch recovers correctly;
3. process crash after ambiguous external dispatch enters reconciliation without blind retry;
4. projection deletion/corruption can be rebuilt without changing authoritative state;
5. stale security projection cannot authorize an action;
6. concurrent consumers cannot both consume one-shot approval;
7. optimistic concurrency rejects stale writes;
8. schema migration preserves event/entity invariants;
9. migration failure does not leave partially reinterpreted authority state;
10. encrypted sensitive fixture never appears as plaintext in generic event/projection tables;
11. revoked/expired lease remains revoked/expired after restart;
12. restored backup revalidates runtime-local authority before execution;
13. required event-integrity failure blocks protected execution;
14. unknown future security-relevant schema value fails safely;
15. database/replay recovery completes without invoking an LLM.

## Deliverables

- SQLite schema v1;
- migration runner and initial migration set;
- transactional command persistence interface;
- append-only event-store interface;
- entity/version concurrency interface;
- approval/lease authority repositories;
- effect/outbox/reconciliation repository;
- projection framework/checkpoint metadata;
- encrypted-object persistence integration;
- startup recovery procedure;
- backup/restore procedure;
- corruption/recovery/adversarial fixture corpus.

## Exit criteria

SPEC-006 is implementation-ready when the schema topology, transaction boundaries, event fields, approval/lease/effect persistence state machines, projection consistency classes, migration contract, and startup recovery sequence can be implemented and tested without architectural interpretation.

## Non-goals

This specification does not define distributed consensus, a cloud database requirement, arbitrary multi-writer synchronization, or workspace-intelligence index storage.