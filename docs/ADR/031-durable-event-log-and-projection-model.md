# ADR 031: Durable Event Log and Projection Model

**Status:** Draft

## Context

Praxis must explain, replay, debug, learn from, synchronize, and visualize graph and agent behavior over long periods. Dashboard graph/team/timeline views, crash recovery, learning evidence, multi-machine reconciliation, and provenance all require a common chronology.

Using mutable current-state records alone would make important transitions and causal history disappear. Using full event sourcing for every byte of application state would add unnecessary complexity and make large artifacts and derived indexes awkward.

## Decision

Praxis SHALL use a **durable append-only domain event log as the authoritative history of consequential state transitions**, combined with deterministic/materialized projections for efficient current-state queries.

Praxis is therefore event-led, but not every payload or artifact must be event-sourced.

### Canonical event envelope

Every durable domain event SHALL include at least:

- globally unique `event_id`;
- event schema/type and version;
- occurrence timestamp and durable-record timestamp where they differ;
- emitting component/plugin identity;
- human/context/goal scope when applicable;
- run, graph/version, node, agent/version and plugin correlation identifiers when applicable;
- `causation_id` identifying the event/command that directly caused it when known;
- `correlation_id` grouping the larger operation/goal/run;
- monotonic stream position assigned by the authoritative local store;
- payload;
- provenance/authority classification;
- sensitivity/redaction metadata where applicable.

### Ordering

Praxis SHALL guarantee total ordering only within an authoritative local event store/stream position. Distributed events SHALL NOT pretend to have a globally meaningful wall-clock order.

Causality SHALL be represented explicitly using identifiers and reconciliation metadata rather than inferred solely from timestamps.

### Events versus telemetry

Consequential domain transitions SHALL be durable events. High-volume diagnostic measurements MAY remain telemetry.

If deleting an observation would make Praxis unable to explain or reconstruct a consequential decision/state transition, it belongs in the durable event history or must be durably referenced by it.

### Projections

Current graph state, agent state, preference views, dashboard summaries, indexes, counters and similar read models SHALL be projections derived from durable events and/or canonical immutable records.

Projection schemas MAY evolve independently. A projection SHALL be rebuildable from its authoritative inputs or explicitly declare why it is not rebuildable.

### Large artifacts

Large files, transcripts, model outputs, binary artifacts and similar payloads SHALL NOT be embedded blindly into the event log. Events SHALL reference content-addressed or otherwise durable artifact records with integrity metadata.

### Replay

Replay SHALL reproduce deterministic state transitions without re-performing external side effects or re-spending model/tool calls unless an explicit execution command authorizes them.

Events SHALL distinguish facts that happened from commands requesting something to happen.

### Subscriptions

The runtime SHALL expose ordered event subscriptions for dashboard, learning, governance and plugin consumers. Consumers SHALL maintain checkpoints/cursors and tolerate restart/replay.

Slow consumers SHALL NOT block authoritative event persistence. Backpressure and bounded buffering SHALL be explicit at subscription boundaries.

### Retention

Consequential event history is durable by default. Retention/compaction MAY remove redundant telemetry or replace bulky payloads with retained summaries/references, but SHALL NOT destroy required provenance, governance, recovery or lineage history without an explicit policy.

### Plugin events

Plugins SHALL emit canonical/namespaced typed events through the Praxis event boundary. Plugins SHALL NOT write directly into core persistence tables.

Unknown plugin event types may be retained opaquely, but core behavior SHALL NOT infer semantics from unversioned arbitrary JSON.

## Alternatives considered

### Mutable state plus logs

Simpler initially, but logs are not reliable domain history and cannot safely drive replay, learning, reconciliation, or explanation.

### Full event sourcing for all state

Provides maximal historical reconstruction but unnecessarily forces artifacts, caches, static definitions and every data concern into event semantics. Praxis instead uses authoritative events for transitions plus canonical immutable records/projections where appropriate.

### External event broker as authority

Kafka/NATS/etc. could provide transport but would violate local-first simplicity and make core durability depend on external infrastructure. Such systems may later bridge events but are not the authoritative local store.

## Consequences

- Dashboard timeline becomes a native projection of system history.
- Learning receives attributable evidence rather than scraped logs.
- Crash recovery and debugging gain a durable causal record.
- Multi-machine reconciliation has explicit event identity/causality.
- Event schema compatibility becomes a core contract responsibility.
- Side-effect boundaries must be modeled explicitly to make replay safe.

## Follow-up

Persistence architecture SHALL define the local event store, transaction boundary, projections, artifact storage, migration and recovery mechanisms.