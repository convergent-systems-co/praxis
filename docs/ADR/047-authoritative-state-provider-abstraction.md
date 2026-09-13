# ADR-047: Authoritative State Provider Abstraction

- Status: Draft
- Date: 2026-09-13
- Related: ADR-011, ADR-031, ADR-032, ADR-035, ADR-036

## Context

Praxis 2 currently uses SQLite as the local authoritative implementation because it provides strong transactional semantics, portability, crash recovery, and low operational burden. SQLite is an implementation choice, not the conceptual architecture.

If runtime/services directly depend on SQLite tables/SQL behavior, Praxis becomes difficult to evolve toward another local database, embedded replicated store, remote organizational control plane, or testing implementation even when a future backend can satisfy the same invariants.

The abstraction cannot be a lowest-common-denominator generic key/value API. Praxis depends on stronger semantics: optimistic aggregate versions, atomic authority consumption, append-only events, transactional package activation, projection checkpoints, durable waits, encrypted blobs, and fail-closed consistency.

## Decision

Praxis SHALL define an **AuthoritativeStateProvider** boundary expressing semantic operations and transactional invariants required by the runtime.

SQLite SHALL remain the default/reference local provider.

Runtime/domain packages SHALL depend on semantic provider interfaces/services, not SQLite-specific SQL or schema details. SQLite-specific code SHALL remain in the state-provider implementation layer.

The provider contract SHALL expose capability groups rather than one monolithic generic database API, including at least:

- command/event append and optimistic aggregate versions;
- replay/global event reads;
- projection checkpoints;
- approval/capability lease lookup and atomic consumption;
- effect-intent persistence/reconciliation state;
- secure blob persistence;
- installed package generations and invocation registry activation;
- persistent run/control state derived from events;
- migration/schema/provider version metadata;
- transactional execution for operations whose security invariant spans multiple writes.

## Semantic portability

A provider is compatible only if it can prove the required semantics. Praxis SHALL NOT silently weaken behavior to fit a backend.

Examples:

- if a backend cannot atomically consume a one-shot lease with the protected mutation/effect boundary, that mode is unsupported;
- if optimistic concurrency cannot be enforced, authoritative event mutation is unsupported;
- if durable encrypted-blob integrity metadata cannot be preserved, protected persistence is unsupported;
- eventual consistency SHALL NOT be treated as equivalent to local serializable authority for security-sensitive commands.

Provider capability/consistency properties SHALL be discoverable and checked deterministically.

## Provider interface layering

Prefer narrow interfaces owned by consuming subsystems over exposing database primitives. Conceptually:

```text
runtime/domain service
        ↓
semantic repository/service interface
        ↓
AuthoritativeStateProvider capability
        ↓
SQLite provider (default)
        ↓
SQLite
```

Tests MAY use deterministic in-memory providers where the tested semantics are faithfully reproduced.

## Migrations

Schema migrations are provider-specific implementation concerns. Canonical state/event/package contracts and migration intent remain provider-neutral.

Export/import and portable state formats SHALL NOT be raw SQLite database copies as the canonical interchange format.

## Consequences

- SQLite remains simple and immediately usable.
- Future providers can be added without rewriting graph/package/domain logic.
- Provider substitutions must pass the same conformance suite rather than merely implement CRUD methods.
- Some SQLite-specific optimizations may remain behind provider-specific interfaces where they do not alter semantics.
- The abstraction is intentionally richer than a generic persistence interface because security invariants depend on transactional behavior.

## Acceptance direction

1. core runtime/domain packages compile without importing an SQLite driver/package;
2. SQLite implements the full authoritative provider conformance suite;
3. an in-memory test provider can satisfy a bounded subset without changing runtime semantics;
4. provider capability mismatch fails closed;
5. export/import works through canonical state contracts rather than raw SQLite pages;
6. swapping provider implementations does not change command/event/authority semantics.
