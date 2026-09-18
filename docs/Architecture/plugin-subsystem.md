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

It should have three tightly linked views over the same authoritative observation state.

### Graph view

A live **node-and-edge graph view** shows execution in motion. It should visualize:

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

### Team view

A generalized team/agent view preserves the useful team model from the earlier development workflow without coupling it to software development. It answers **who or what is performing the work and how responsibility is flowing**.

For every active agent, worker, or delegated subsystem it should show:

- stable identity and current generation;
- role and current goal/sub-goal;
- graph and active node;
- current status: idle, reasoning, executing, waiting, blocked, failed, or complete;
- selected model/executor/tool surface;
- authority scope and relevant policy limits;
- context scope and applicable user preferences;
- inputs/dependencies and downstream handoffs;
- work delegated to or received from other agents;
- current evidence obligations;
- retries, repairs, escalation, and human-intervention state;
- token/inference usage where measurable;
- deterministic work versus inference work;
- lineage to the catalog seed or prior agent generation;
- learned adaptations that distinguish this agent from its original seed.

The team view should support selecting an agent and pivoting directly to the corresponding node/subgraph in the graph view. Selecting a node in the graph view should identify the responsible agent or subsystem in the team view.

### Timeline / log view

A time-oriented event view is required for debugging and performance analysis. It should combine an ordered event log with duration-aware visualization so the user can answer **what happened, in what order, and where time was spent**.

At minimum it should show:

- absolute timestamp and elapsed run time for every event;
- node enter/exit times and total node residency duration;
- edge traversal times;
- agent active, reasoning, executing, waiting, blocked, and idle durations;
- executor/model/tool invocation start/end and latency;
- inference latency separately from deterministic execution latency;
- retries and repair-loop duration;
- escalation and human-wait duration;
- handoff time between agents/subsystems;
- evidence production and verification latency;
- queue/wait time versus actual execution time;
- parallel branch overlap and critical-path duration;
- graph/subgraph total duration;
- plugin activation/deactivation time where relevant;
- learning observation, candidate creation, promotion, rollback, and drift timestamps;
- context/preference changes that altered routing or behavior.

The log should support filtering by graph, subgraph, node, agent, plugin, executor, event type, status, and time range. Selecting a log event should highlight the corresponding node and responsible agent. Selecting a node or agent should filter the timeline to the relevant events.

For each completed run, the dashboard should derive a concise timing summary:

- wall-clock duration;
- critical path;
- total inference time;
- total deterministic execution time;
- total waiting/blocked time;
- total human-wait time;
- retry/repair overhead;
- avoidable repeated-work estimate where measurable;
- top time-consuming nodes/agents/executors.

This timing data is also input to Praxis learning: repeated high-latency inference, repeated retries, or stable expensive decision paths are candidates for deterministic extraction or graph redesign.

### Debugging value

These views should not wait until the final product phase. Together they expose incorrect transitions, unexpected loops, excessive inference, hidden retries, blocked joins, poor delegation, stale state, authority mistakes, performance bottlenecks, and learning changes that would otherwise require reconstructing behavior from logs.

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
