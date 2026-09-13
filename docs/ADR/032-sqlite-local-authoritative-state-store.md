# ADR 032: SQLite as the Local Authoritative State Store

**Status:** Draft

## Context

Praxis is local-first, must survive crashes, operate offline, move between machines, and support durable event history plus efficient projections. Requiring PostgreSQL, Redis, Kafka, a vector database, or cloud infrastructure would make installation and recovery materially more complex.

The runtime needs transactional consistency between accepted commands, durable events, and critical projection/checkpoint updates. It also needs migrations, integrity checks, backups, and deterministic export/import.

## Decision

Praxis SHALL use **SQLite as the default local authoritative metadata/state database**.

### Responsibilities

SQLite SHALL initially store:

- durable event log metadata and event payloads of reasonable size;
- canonical entity metadata and immutable version records;
- projection/read-model state;
- run/checkpoint state;
- plugin registration/installation metadata;
- preferences, evidence metadata, decisions and provenance;
- synchronization/reconciliation metadata;
- artifact metadata and content references.

Large/binary artifacts SHALL live outside SQLite in a Praxis-managed content/artifact store and be referenced transactionally by metadata/integrity identifiers.

### Transaction boundary

A consequential state transition SHALL persist its authoritative event and any required atomic metadata/checkpoint changes in one SQLite transaction where practical.

Derived projections that can be rebuilt MAY update asynchronously after the authoritative transaction.

### Database mode

Praxis SHOULD use WAL mode for normal local operation, with explicit busy timeouts and bounded transaction duration. The runtime SHALL avoid treating SQLite as a network-shared multi-host database.

### Single authoritative writer

`praxisd` SHALL own authoritative writes during normal operation. CLI, dashboard, and plugins SHALL use the Praxis control/plugin APIs rather than opening the database for arbitrary writes.

Read-only diagnostic tooling MAY open safe snapshots/read connections where explicitly supported.

### Schema migrations

Database schema changes SHALL use ordered, versioned migrations. Startup SHALL refuse unsafe downgrade or unknown-newer schema conditions rather than guessing.

Migrations affecting durable contracts SHALL provide backup/rollback or forward-recovery strategy appropriate to the change.

### Integrity and recovery

Praxis SHALL support:

- integrity checks;
- clean snapshot/backup while running;
- recovery from interrupted process execution;
- detection of incomplete/corrupt migrations;
- export of durable state into a portable versioned format independent of raw SQLite file copying.

Raw database copying SHALL NOT be the canonical machine-migration protocol.

### Artifact store

The default local artifact store SHALL be a Praxis-managed filesystem tree using immutable/content-addressed objects where practical. Metadata SHALL include integrity hashes, media/type information, provenance and retention/sensitivity policy.

### Search/vector indexes

FTS/vector/search indexes SHALL be treated as rebuildable derived indexes unless an ADR explicitly promotes one to authoritative state. Praxis SHALL NOT require an external vector database for core operation.

### External databases

Future plugins MAY provide alternate/remote storage, replication, archival or enterprise integrations. Core semantics SHALL remain defined independently of those systems.

## Alternatives considered

### PostgreSQL as default

Powerful and scalable, but imposes service installation/operation inconsistent with a local-first single-user daemon. It remains viable for future enterprise/remote plugins.

### Embedded key-value store

Good for simple persistence but weaker for relational projections, migrations, ad-hoc diagnostics and transactional multi-entity updates.

### Pure event files

Simple append behavior but complicates indexing, transactional metadata, projection checkpoints, migrations and concurrent readers.

## Consequences

- Praxis can ship as a Go binary with no mandatory external database service.
- Event history and current-state projections can share local transactional durability.
- Operational backup and inspection are straightforward.
- Large artifacts and derived search indexes remain separable.
- The daemon becomes the authoritative write boundary, which simplifies concurrency but requires a well-defined control API.
- Multi-machine operation must synchronize/export semantic state rather than sharing the SQLite file.

## Follow-up

Failure/recovery and multi-machine synchronization ADRs SHALL build on this single-authoritative-local-writer model.