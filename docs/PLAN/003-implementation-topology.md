# PLAN-003: Praxis 2 Implementation Topology

- Status: Draft
- Governing plan: PLAN-001
- Branch: `redesign/praxis2`

## Purpose

Translate the architecture/SPEC set into a concrete Go-first repository topology without treating the existing Python tree as architectural authority.

## Target topology

```text
cmd/
  praxis/                 # user CLI
  praxisd/                # long-running runtime/daemon

internal/
  kernel/                 # command dispatch, authority, lifecycle coordination
  domain/                 # internal domain implementations of canonical contracts
  eventstore/             # append-only event persistence
  state/                  # SQLite repositories/migrations
  projection/             # read models/checkpoints/rebuild
  effect/                 # ActionIntent/effect dispatch/reconciliation
  policy/                 # deterministic policy evaluation
  approval/               # approval binding/consumption
  capability/             # capability registry/leases
  scheduler/              # slice/resource scheduling
  plugin/                 # plugin supervisor/instance lifecycle
  crypto/                 # profile resolution/provider abstraction
  sync/                   # portable-state boundary
  observability/          # logs/metrics/tracing contracts

pkg/
  contracts/              # public stable Go contract types where appropriate
  pluginapi/              # plugin SDK-facing types/helpers

proto/
  praxis/v1/              # canonical protobuf boundary
    common.proto
    principal.proto
    provenance.proto
    command.proto
    event.proto
    capability.proto
    plugin.proto
    crypto.proto
    invocation.proto
    workspace.proto

schema/
  json/                   # human-authored/config JSON schemas
  registry/               # stable IDs/enums/crypto suites/capability metadata

migrations/
  sqlite/                 # ordered Praxis 2 migrations

plugins/
  workspace/              # Workspace Intelligence implementation
  executors/              # executor/provider plugins/adapters
  clients/                # client integration materializers/adapters as appropriate

packages/
  develop/                # software-development proving package; later wave

conformance/
  contracts/
  plugins/
  security/
  workspace/

compat/
  praxis1/                # explicit temporary compatibility only
```

## Boundary rules

- `internal/*` is not a plugin SDK.
- Plugins communicate through versioned protocol/capability contracts, not Go internal imports.
- `pkg/contracts` exposes only semantics intentionally supported as a public Go API.
- Protobuf is canonical for cross-process plugin/client protocol; human-authored config validates to the same semantic contracts.
- SQLite row layout is private to `internal/state`; synchronization never copies DB rows as protocol.
- Domain packages such as `develop` consume core/plugin capabilities and cannot import internal implementation details.
- Existing Python components are classified/ported/bridged under ADR-027; no new Praxis 2 contract is defined by Python module shape.

## First implementation bundles

### B0.1 Contract skeleton
Create Go module/package topology, stable contract primitives, canonical enum/ID registries, protobuf package layout, deterministic canonical serialization/digest helpers, and contract tests.

### B0.2 Authority primitives
Implement PrincipalRef, ProvenanceRef, TrustClass, CapabilityLease, ActionIntent, ApprovalBinding, EffectIntent, CryptoProfile/CryptoSuiteRef types and validators.

### B0.3 SQLite bootstrap
Create migration runner and schema v1 for metadata, command idempotency, events, authority state, approvals, capability leases, effects, and projection checkpoints.

### B0.4 Command/event transaction
Implement transaction boundary for idempotent command acceptance, optimistic version checks, authoritative event append, approval consumption, and effect-intent creation.

### B0.5 Projection/recovery
Implement projection checkpoints/rebuild and startup recovery skeleton.

### B0.6 Plugin protocol skeleton
Implement protobuf service contracts, plugin manifest/instance identity, capability discovery, health, and lease-bound request context.

## Definition of done for each bundle

A bundle is not done because types compile. It must include governing ADR/SPEC references, deterministic validation, unit tests, negative/security tests, migration/compatibility impact, and observable evidence satisfying the mapped SPEC acceptance criteria.

## Legacy containment

During implementation, existing `src/` Python code remains available as behavioral reference. New Go core code SHALL not be forced into `src/` merely to match the legacy layout. Compatibility bridges must be visibly temporary and placed under `compat/` or an equivalently explicit boundary.
