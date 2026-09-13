# SPEC-002: Command, Query, and Event Boundary

- Status: Draft
- Governing ADRs: 022, 023, 031, 032, 035, 036

## Purpose

Provide one authoritative path for durable state mutation and governed external effects regardless of whether intent originates from a graph, CLI, API, plugin, hook, skill, LLM client, or human interface.

## Command pipeline

Every accepted command SHALL pass through:

`ingress -> normalize envelope -> authenticate/identify actor -> authorize -> validate invariants -> idempotency check -> execute handler -> append durable event(s) -> update/rebuild projection -> return result`

A command SHALL include command ID, command type/version, actor/provenance, scope, correlation ID, causation ID when applicable, and payload.

Commands that can be retried SHALL have deterministic idempotency behavior. Duplicate command delivery must not duplicate authoritative effects.

## Query pipeline

Queries SHALL read projections/read models and SHALL NOT mutate authoritative state. If a read discovers information that should become learned/durable state, the caller must submit an explicit command representing that observation.

## External effects

Effects outside the authoritative store SHALL have a durable effect record containing at minimum effect ID, originating command/event, target adapter, requested operation, idempotency key when supported, state, attempts, and observed result.

Effect execution SHALL support reconciliation after process interruption. The runtime SHALL NOT claim atomicity across SQLite and an external service.

## Event requirements

Accepted authoritative transitions SHALL append versioned domain events. Events are immutable. Corrections are represented by later events. Projections SHALL be rebuildable from authoritative persisted state/events according to ADR-031/032.

## Failure behavior

- Validation/authorization failure: no domain mutation event.
- Handler failure before durable transition: command fails with no partial authoritative mutation.
- Failure after durable intent but before external completion: effect remains reconcilable/pending.
- Projection failure: authoritative event remains valid; projection is recoverable/rebuildable.

## Observability

Logs/traces SHALL propagate command ID, correlation ID, causation ID, graph/agent/slice identity where present, and effect ID where present.

## Acceptance tests

- identical idempotent command submitted twice produces one authoritative transition;
- query cannot mutate durable state;
- projection can be rebuilt after deletion/corruption;
- simulated crash between effect intent and external completion is reconciled;
- every ingress surface exercises the same authorization/mutation boundary;
- unauthorized hook/plugin/client invocation cannot bypass command governance.

## Non-goals

This is not a requirement for separate command/query services or databases.