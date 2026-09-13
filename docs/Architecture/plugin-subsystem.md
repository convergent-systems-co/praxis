# Praxis 2 Plugin and Subsystem Architecture

Status: Draft

## Purpose

Praxis 2 keeps a small domain-neutral kernel and composes optional behavior through a first-class plugin subsystem. A plugin may provide a domain graph, executor, state transport, catalog provider, user interface, evaluator, storage adapter, learning extension, or other service without requiring that concrete subsystem to be imported by the kernel.

## Why plugins

The current repository already contains strong subsystem seams: runtime, contracts, evidence, executors, policy, evaluation, learning, orchestration, dashboard, and domain overlays. The redesign should preserve these reusable components while replacing hard-coded composition with explicit lifecycle and service contracts.

The plugin layer exists to prevent three forms of coupling:

1. the Praxis kernel knowing concrete domains such as software delivery;
2. the CLI knowing every executor or graph implementation;
3. optional interfaces such as dashboards becoming runtime dependencies.

## Kernel vs plugin

Kernel responsibilities remain narrow:

- graph and state semantics;
- contracts and validation;
- policy and authority enforcement;
- evidence and evaluation primitives;
- plugin discovery, lifecycle, dependency ordering, and service registration;
- portable state and governance primitives as they are implemented.

Candidate plugins include:

- development domain;
- executor adapters;
- catalog providers;
- synchronization transports;
- state backends;
- dashboard and observability surfaces;
- goal-classification/domain packages;
- optional learning strategies.

## Service namespaces

Plugins export named services. Initial namespaces include:

- `executor/<id>`
- `graph/<id>`
- `overlay/<id>` during migration
- `observability/<id>`

Namespaces are intentionally open so new plugin classes do not require a kernel enum change.

## Professional dashboard as the first reference plugin

The dashboard should be the first user-facing reference plugin and should be built early enough to aid implementation and testing, even if final visual polish belongs later in the redesign.

Its primary mode should be a live **node-and-edge graph view** showing execution in motion.

The graph view should visualize:

- graph nodes and directed edges;
- current/active node state;
- completed, failed, waiting, blocked, and skipped nodes;
- edge traversal and transition events;
- concurrent branches and joins;
- persistent agent graphs and goal/process graphs;
- nested/subgraph execution;
- executor selection and execution state at each node;
- evidence and verification outcomes attached to nodes;
- policy and authority gates;
- inference nodes versus deterministic nodes;
- retries, repairs, escalation, rollback, and demotion;
- graph and agent generation/lineage changes;
- learning observations and candidate graph mutations;
- preference/context resolution affecting graph routing;
- plugin lifecycle and health;
- multi-machine lineage/reconciliation when implemented.

### Debugging value

This should not wait until the final product phase because a live graph view provides direct architectural feedback during implementation. It can expose incorrect transitions, unexpected loops, excessive inference, hidden retries, blocked joins, stale state, and learning changes that would otherwise require log reconstruction.

The dashboard must remain read-only with respect to authoritative runtime state. It observes and explains; mutation belongs to governed APIs and workflows.

The existing Python dashboard is reusable as an observation/query backend and behavioral reference. Its presentation layer can be replaced or substantially refactored without discarding its tested observation semantics.

## Lifecycle

Plugins declare stable identity, version, open-ended kind, dependencies, and named services. Activation is dependency ordered and fail-closed. Service collisions, missing dependencies, dependency cycles, and undeclared exports are errors. Partial activation is rolled back.

## Discovery

External Python distributions may advertise plugins through the `praxis.plugins` entry-point group. Praxis-owned built-ins may be registered explicitly by the composition root. The kernel does not scan arbitrary modules.

## Migration strategy

`praxis_overlay` remains temporarily as a compatibility API because the development overlay already has useful, tested manifest/resource/grader behavior. New subsystem composition should target `praxis_plugins`. Existing overlays become domain plugins that may internally adapt their old overlay components until those APIs are generalized or retired.

The same approach applies to executor adapters: their existing `Executor` abstraction is retained; plugins provide executor instances rather than replacing the executor contract.

## Deletion rule

During Praxis 2 salvage, source survives if it is one of:

1. kernel-grade and domain neutral;
2. useful behind a plugin boundary;
3. a temporary compatibility shim with an explicit migration path;
4. a test fixture supporting retained behavior.

Code that satisfies none of these conditions is redesign-dead and should be removed.
