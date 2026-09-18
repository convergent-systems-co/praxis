# ADR 028: Go Core and Language-Agnostic Plugin Boundary

## Status

Draft

## Context

Praxis 2 is evolving from a Python-heavy implementation toward a long-lived local-first agent runtime that must support persistent graph execution, self-improving agents and goal graphs, durable state, plugin supervision, event streaming, observability, CLI/TUI interaction, multi-machine portability, and independently installable integrations.

The current Python implementation contains substantial reusable behavior, including runtime state transitions, executor abstractions, evidence handling, policy, evaluation, learning, orchestration, compatibility logic, and provider adapters. However, implementation language and architectural role have become coupled in places. If Praxis remains Python-first at the core, the system inherits Python environment management, packaging friction, interpreter/runtime dependencies, and weaker single-binary distribution characteristics for functionality that is not inherently Python-specific.

Praxis also needs to integrate with heterogeneous LLM tooling and subsystems, including Claude Code, Codex, GitHub Copilot, Ollama, MLX, local models, MCP/tool servers, domain plugins, dashboard/observability components, and future third-party extensions. Those integrations should not force the core runtime or plugin ecosystem to share one implementation language.

The design therefore needs to settle two related questions:

1. What implementation language should own the Praxis core/runtime?
2. What architectural boundary should separate the core from plugins and provider integrations?

## Alternatives considered

### 1. Keep Praxis core Python-first

Continue evolving the existing Python packages into the long-term core and use Python packaging and entry-point discovery for plugins.

Advantages:

- minimizes near-term migration work;
- preserves direct reuse of the current implementation;
- provides strong access to AI/ML libraries;
- keeps experimentation fast.

Costs:

- interpreter and environment management remain part of core installation and operation;
- single-binary distribution is difficult;
- long-running daemon/supervisor behavior depends on Python runtime deployment quality;
- concurrency-heavy orchestration and process supervision are less natural than in Go;
- plugin/runtime implementation language remains unnecessarily coupled.

### 2. Rewrite all of Praxis and all plugins in Go

Move the entire platform, including provider integrations and model-specific functionality, into Go.

Advantages:

- uniform implementation language;
- simple deployment model;
- strong concurrency and process-management primitives;
- easier static binary distribution.

Costs:

- discards useful Python implementations prematurely;
- forces ML/ecosystem-specific integrations away from their strongest libraries;
- makes experimentation slower where Python is the native ecosystem;
- creates unnecessary rewrite risk.

### 3. Go core with a language-agnostic process/plugin boundary

Implement the Praxis core/runtime in Go while defining a versioned inter-process plugin protocol that can be implemented in Go, Python, or another language.

Advantages:

- single-binary core and simpler deployment;
- strong concurrency, process supervision, daemon, CLI, and event-streaming characteristics;
- plugins remain free to use the language best suited to their domain;
- existing Python provider adapters can migrate incrementally rather than being discarded;
- repository topology and plugin implementation language remain independent;
- third-party plugins can be isolated from core process failures;
- plugin protocol becomes a durable compatibility contract rather than a Python import convention.

Costs:

- introduces an inter-process contract and serialization boundary;
- requires protocol versioning, lifecycle management, health checks, cancellation, capability negotiation, and error semantics;
- migration temporarily operates both Python and Go implementations;
- debugging crosses process boundaries.

### 4. Go core using Go's native `plugin` package

Implement core and plugins in Go and load compiled `.so` plugins directly.

Advantages:

- in-process execution;
- low serialization overhead.

Costs:

- platform limitations;
- strong coupling to Go toolchain/compiler versions and build environment;
- poor fit for Windows and cross-platform distribution;
- prevents Python/other-language plugins;
- plugin crashes and dependency issues affect the core process.

This option does not meet Praxis portability and language-independence goals.

## Decision

Praxis 2 will use **Go as the implementation language for the core runtime**.

The Go core will own capabilities that are foundational to the platform rather than specific to an AI provider or domain, including:

- graph execution and scheduling;
- durable runtime state and state transitions;
- event publication and event sequencing;
- plugin discovery, lifecycle, health, supervision, and restart behavior;
- persistence and state portability;
- learning/governance coordination;
- capability resolution;
- policy and authority enforcement boundaries;
- CLI/TUI foundations;
- observability/event APIs;
- cancellation, deadlines, and resource control.

Praxis plugins will communicate with the core through a **versioned language-agnostic process boundary**. Plugins may be implemented in Go, Python, or another language when justified by their ecosystem or capability.

The architecture will **not use Go's native `plugin` package as the canonical extension mechanism**.

The plugin protocol must support, at minimum:

- plugin identity and semantic version;
- protocol/API version negotiation;
- declared capabilities and services;
- startup/activation and shutdown/deactivation lifecycle;
- health and readiness checks;
- request/response operations;
- streamed events where needed;
- cancellation and deadlines;
- structured errors;
- telemetry and timing metadata;
- authentication/authorization context where applicable;
- schema evolution and backward-compatibility rules;
- explicit handling of plugin crashes and reconnect/restart behavior.

Graphs must continue to request **capabilities**, not concrete providers, unless a graph explicitly requires a provider-specific capability. Provider integrations such as Claude Code, Codex, GitHub Copilot, Ollama, and MLX are plugins/adapters rather than dependencies of the core.

Existing Python code is not considered disposable solely because of this decision. Each existing package will be classified as one of:

1. behavior to port into the Go core;
2. a temporary/reference implementation used to validate the Go port;
3. a Python plugin that remains Python because the ecosystem benefits from it;
4. a compatibility bridge during migration;
5. obsolete code to remove once its behavior is covered elsewhere.

The preferred distribution model is a **single Praxis core binary plus independently installable plugins**. Plugins may initially live in the Praxis monorepo as separately versioned/installable components and may later move to independent repositories without changing the protocol contract.

### Canonical plugin transport remains open

This ADR intentionally does **not** select the final wire transport. A follow-up ADR must choose the canonical transport after evaluating at least:

- gRPC + Protocol Buffers;
- JSON-RPC over stdio;
- whether one is canonical and the other is an optional compatibility transport.

The evaluation must consider local process supervision, streaming, schema/version guarantees, implementation complexity, debugging ergonomics, startup overhead, third-party language support, security boundaries, and plugin distribution.

Until that follow-up ADR is accepted, implementation must avoid baking transport-specific assumptions into graph, capability, plugin, or state contracts.

## Consequences

### Improvements

- Praxis core becomes easier to distribute as a self-contained executable.
- The long-running runtime, scheduler, event system, and plugin supervisor gain a concurrency and systems-programming model well suited to their responsibilities.
- Python remains available where it is actually advantageous rather than becoming a platform-wide dependency.
- LLM/provider integrations become replaceable plugins instead of architectural dependencies.
- Third-party plugin development is not constrained to the implementation language of the core.
- Provider/tool failures can be isolated behind supervised process boundaries.
- The architecture supports moving plugins between monorepo and separate repositories without redesigning core interfaces.

### Costs and risks

- The current Python core behavior must be migrated deliberately rather than simply extended indefinitely.
- During migration, duplicate implementations and compatibility layers will exist temporarily.
- Cross-process contracts require stronger schema discipline than Python in-process interfaces.
- Plugin startup, transport, event streaming, cancellation, and failure semantics become first-class engineering concerns.
- Some low-latency operations may pay serialization/process-boundary overhead, which must be measured rather than assumed negligible.

### Migration constraints

- No working Python behavior should be rewritten without first identifying its contract and tests.
- Existing Python tests should be reused as behavioral specifications where practical.
- Go ports should be validated against existing runtime behavior and durable fixtures before Python implementations are removed.
- Provider adapters should migrate behind the language-agnostic plugin boundary before they are rewritten solely for language consistency.
- Development remains one plugin/domain and must not shape the core Go API around software-development-specific concepts.

## Follow-up decisions

1. Select canonical plugin transport and schema technology.
2. Define the Go module/package topology for the Praxis kernel.
3. Define plugin packaging, installation, discovery, and family/component versioning.
4. Define migration sequencing from Python packages to Go core modules.
5. Define which existing Python provider adapters remain Python plugins versus later Go rewrites.
6. Define conformance tests that every plugin implementation must pass regardless of language.
