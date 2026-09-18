# ADR 029: gRPC and Protocol Buffers as the Canonical Plugin Transport

**Status:** Draft

## Context

ADR 028 establishes Go as the target implementation language for the Praxis core/runtime and requires the plugin boundary to remain language-agnostic and process-based. It intentionally defers the canonical transport decision.

Praxis plugins are not limited to synchronous tool calls. Plugins may implement LLM executors, persistent agents, graph providers, learning subsystems, observability/dashboard services, domain workflows, storage integrations, or future capabilities. The boundary therefore needs to support:

- typed capability advertisement and version negotiation;
- concurrent requests;
- long-running execution;
- server and bidirectional streaming where appropriate;
- cancellation and deadlines;
- health and lifecycle operations;
- event and telemetry streams;
- language-neutral SDK generation;
- local process isolation;
- future remote execution without changing graph semantics;
- explicit compatibility evolution.

JSON-RPC over stdio is attractive because it is simple, debuggable, and widely implementable. It is also appropriate for MCP and lightweight external integrations. However, making it the canonical Praxis plugin protocol would require Praxis to define and maintain substantial additional conventions for typed schemas, streaming, flow control, health, cancellation, compatibility, and generated SDKs.

gRPC with Protocol Buffers provides those mechanisms as an established language-neutral RPC substrate. It also allows the same service contract to operate over local IPC today and authenticated network transport later.

Praxis must not conflate its internal plugin lifecycle protocol with MCP. MCP remains valuable for tool/resource interoperability, while the Praxis plugin protocol governs the relationship between the Praxis runtime and a Praxis subsystem/plugin.

## Decision

Praxis 2 SHALL use **Protocol Buffers and gRPC as the canonical language-neutral plugin protocol**.

### Process boundary

Concrete plugins SHALL normally execute outside the Praxis core process. Praxis SHALL supervise plugin processes and communicate with them through the canonical plugin protocol rather than dynamically loading implementation code into the core address space.

Praxis SHALL NOT use Go's native `plugin` package as the canonical extension mechanism.

### Local transport

Local plugins SHALL normally communicate over operating-system-local IPC.

On Unix-like systems, the preferred transport is a **Unix domain socket carrying gRPC**.

Equivalent secure local IPC mechanisms MAY be used on platforms where Unix domain sockets are unavailable or inappropriate. Loopback TCP MAY be used as a compatibility fallback, but it is not the preferred local transport.

### Remote transport

The protobuf service contract SHALL remain transport-location independent. A future remote plugin MAY expose the same gRPC contract over an authenticated and encrypted network transport, including mTLS, without requiring graph definitions to change merely because execution moved off-host.

Remote plugin authorization, trust establishment, and deployment policy are separate governance concerns and are not authorized merely by this ADR.

### Bootstrap handshake

Praxis SHALL use a minimal bootstrap mechanism to establish the plugin connection. The bootstrap exchange MUST provide enough information to locate and validate the RPC endpoint and MUST NOT become a second application protocol.

At minimum the resulting connection context SHALL identify:

- plugin identity;
- plugin version;
- Praxis plugin protocol/API version;
- endpoint/transport information;
- compatibility information needed before activation.

Capability advertisement SHALL occur through the typed plugin protocol after the connection is established rather than relying on unstructured bootstrap output as the authoritative capability contract.

### Canonical service semantics

The plugin protocol SHALL provide typed operations or equivalent service contracts for at least:

- handshake/version negotiation;
- capability advertisement;
- execution or invocation;
- execution/event streaming where required;
- cancellation;
- health/readiness;
- lifecycle/shutdown;
- telemetry correlation;
- structured error reporting.

Additional services MAY be defined for graph providers, state snapshots, observability subscriptions, learning subsystems, or other plugin families. These SHALL evolve through versioned protobuf contracts rather than provider-specific conditionals in the Praxis kernel.

### Capability-based selection

Graphs SHALL request capabilities rather than concrete providers whenever practical. The plugin transport MUST preserve this abstraction.

A graph may require a provider-specific capability only when the desired behavior genuinely depends on that provider-specific feature.

The canonical transport therefore SHALL NOT encode Claude, Codex, Copilot, Ollama, MLX, or another model/tooling provider as a kernel-level architectural dependency.

### Language neutrality

Protocol Buffers are the source of truth for the canonical wire contract. Praxis SHOULD generate SDK/client/server bindings for supported plugin languages.

Go is the primary core implementation language under ADR 028, but plugins MAY be implemented in Go, Python, Rust, TypeScript, or another language capable of implementing the protocol.

### JSON-RPC and stdio

JSON-RPC over stdio MAY be supported through adapters for:

- MCP interoperability;
- lightweight external integrations;
- legacy tools;
- environments where implementing the canonical gRPC contract is impractical.

JSON-RPC/stdio SHALL NOT define the canonical Praxis plugin contract.

An adapter MUST translate external semantics into the canonical Praxis capability, lifecycle, error, cancellation, and telemetry model rather than leaking transport-specific assumptions into graph definitions or the core runtime.

### MCP boundary

MCP and the Praxis plugin protocol serve different architectural purposes.

- **Praxis plugin protocol:** Praxis runtime to Praxis plugin/subsystem.
- **MCP:** agent/plugin/tooling integration with MCP tools, resources, prompts, and related capabilities.

A Praxis plugin MAY itself act as an MCP client or bridge. MCP SHALL NOT implicitly become the lifecycle/control protocol for Praxis plugins.

### Observability

Every plugin invocation SHALL carry stable correlation identifiers sufficient to connect plugin activity to Praxis run, graph, node, agent, and timeline telemetry where applicable.

The protocol SHALL make it possible for the dashboard plugin to observe execution duration, event chronology, retries, failures, cancellation, handoffs, provider latency, and other graph/team/timeline information without scraping provider-specific output.

### Failure behavior

Plugin process failure SHALL be observable and SHALL NOT silently corrupt kernel state.

The supervisor and protocol SHALL distinguish, at minimum, transport failure, plugin crash, timeout/deadline, cancellation, incompatible protocol version, unavailable capability, and plugin-reported execution failure.

Retry policy belongs to the caller/supervisor and graph semantics. The transport SHALL NOT hide repeated execution attempts in a way that makes side effects or cost invisible.

### Security

Local IPC endpoints SHALL be scoped and permissioned to the Praxis user/session where supported by the operating system.

Plugins SHALL receive only the authority and resources required for their declared capabilities. Using gRPC does not grant a plugin implicit filesystem, network, credential, model, or user authority.

Remote gRPC endpoints SHALL require an explicit security architecture before being treated as trusted Praxis plugins.

## Alternatives considered

### JSON-RPC over stdio as the canonical protocol

This provides excellent simplicity and inspectability and remains useful as an adapter boundary. It was rejected as the canonical protocol because Praxis would need to design substantial additional machinery for typed compatibility, streaming, health, flow control, cancellation, and SDK generation as the plugin ecosystem grows.

### MCP as the canonical Praxis plugin protocol

This would reuse an increasingly common interoperability standard. It was rejected because MCP primarily addresses model/tool/resource interoperability and does not define the complete lifecycle, supervision, subsystem, graph, learning, and observability contract Praxis requires. Praxis SHOULD integrate MCP rather than redefine it or misuse it as a kernel plugin lifecycle protocol.

### Go native plugins

This could make in-process calls simple for Go-only extensions. It was rejected because it couples plugins to Go implementation details and build/runtime compatibility, weakens language neutrality and process isolation, and complicates cross-platform distribution.

### Custom socket protocol

This could be optimized precisely for Praxis. It was rejected because it would require Praxis to own framing, schemas, compatibility, streaming, cancellation, tooling, and SDK generation that gRPC/protobuf already provide.

## Consequences

### Improvements

- Praxis gains a typed, versionable, language-neutral plugin boundary.
- Go core development does not force plugins to be written in Go.
- Streaming execution and observability become first-class protocol capabilities.
- Local and future remote plugins can share service contracts.
- Provider-specific implementation details remain outside the kernel.
- SDK generation reduces duplicated protocol code across plugin languages.
- Dashboard graph/team/timeline observability can consume structured events rather than provider-specific logs.
- Existing Python integrations can be migrated behind generated Python gRPC bindings rather than rewritten solely because the core moves to Go.

### Costs and constraints

- Protobuf schema evolution becomes an architectural compatibility responsibility.
- Plugin development requires generated bindings and gRPC tooling.
- Process supervision, endpoint bootstrap, lifecycle, and local socket cleanup must be implemented correctly.
- Very small plugins may find JSON-RPC/stdio simpler, requiring an adapter or SDK to reduce friction.
- gRPC alone does not solve plugin trust, sandboxing, authorization, credential delegation, or remote security; those remain explicit architecture concerns.

## Relationship to ADR 028

This ADR resolves the transport decision intentionally deferred by ADR 028.

ADR 028 remains authoritative for the Go-core and language-agnostic process-plugin decision. ADR 029 makes **gRPC + Protocol Buffers** the canonical implementation of that process boundary and retains JSON-RPC/stdio as an adapter mechanism rather than the core contract.

## Follow-up work

Implementation should define, in order:

1. the versioned protobuf package and compatibility policy;
2. the minimal bootstrap/endpoint handshake;
3. core plugin lifecycle and health services;
4. capability advertisement and invocation contracts;
5. event/correlation envelopes for graph, agent, and timeline observability;
6. Go and Python SDKs first;
7. a compatibility adapter for JSON-RPC/stdio and MCP-facing plugins;
8. migration of one existing executor plugin as the first end-to-end reference implementation;
9. migration of the dashboard plugin as the first streaming observability consumer.
