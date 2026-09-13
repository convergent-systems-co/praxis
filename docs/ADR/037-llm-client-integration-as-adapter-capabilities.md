# ADR-037: LLM Client Integration as Adapter Capabilities

- Status: Draft
- Date: 2026-09-13

## Context

Praxis graphs must be usable from heterogeneous LLM clients such as coding CLIs, desktop assistants, IDE agents, local model shells, and future clients. These clients expose different extension mechanisms: skills, slash commands, hooks, MCP/tool servers, plugins, instruction files, lifecycle callbacks, subprocess invocation, or no native extension mechanism at all.

Binding a graph's semantics directly to one client's skill or hook format would make the client the architecture. Praxis instead needs clients to be interchangeable surfaces over persistent graphs and agents.

## Decision

LLM clients are integration adapters. Praxis graphs and agents remain client-neutral and execute under the Praxis runtime.

A client adapter advertises a machine-readable `ClientCapabilityProfile` describing supported integration mechanisms and lifecycle events. Praxis selects the richest safe integration available for that client and graph.

### Integration modes

The canonical modes, in descending preference when supported and appropriate, are:

1. **Tool/protocol integration**: MCP, native plugin/tool protocol, or equivalent structured invocation. Preferred for commands, queries, graph execution, state inspection, and governed tool access.
2. **Lifecycle hooks**: pre/post prompt, pre/post tool, session start/stop, idle, compaction, or equivalent callbacks. Preferred for observation, context synchronization, event capture, policy checks, and automatic graph continuation where the client exposes reliable hooks.
3. **Skills/commands**: generated client-native skills, commands, or command palettes. Preferred as discoverable user entry points and thin wrappers over Praxis commands, not as the authoritative graph implementation.
4. **Context/instruction projection**: generated minimal instruction/context files when the client supports persistent project/user instructions. Used to teach the client how and when to invoke Praxis, not to encode the graph itself.
5. **CLI/subprocess bridge**: `praxis` CLI invocation for clients with shell/tool access but no richer extension API.
6. **Manual bridge**: explicit user invocation as the compatibility floor.

These modes may be composed. For example, a client can expose a skill that invokes an MCP tool while hooks capture lifecycle observations.

### Generated integration artifacts

Praxis may generate client-specific skills, hook configuration, MCP manifests, command definitions, or instruction projections from a graph/package's integration contract. Generated artifacts are adapters and may be regenerated; they are not authoritative state.

### Graph integration contract

A graph/package may declare:

- user-invocable entry points;
- automatic trigger opportunities;
- lifecycle events it can consume;
- required and optional client capabilities;
- context it needs projected into the client;
- commands/queries it exposes;
- permissions and external effects;
- degradation behavior when capabilities are absent.

Installation resolves this contract against the selected client's capability profile and presents any required permissions or unsupported capabilities.

### Authority and security

Client adapters cannot bypass ADR-035 command authority, package permissions, graph governance, or capability controls. Hooks are observations/triggers, not unrestricted mutation channels. Skills are UX surfaces, not trusted code merely because the client calls them.

Client credentials remain owned by the client where possible. Praxis should use the user's existing authenticated client session/CLI rather than require provider API keys when an adapter can invoke that client safely.

### Portability

A graph that requires only canonical Praxis capabilities should move between clients without modification. Client-specific optimization belongs in adapter metadata or optional integration overlays, never in the graph's core semantics.

## Consequences

Praxis can integrate deeply with clients that expose hooks/MCP while remaining usable through skills or CLI on less capable clients. A catalog graph can declare what it needs once and installation can materialize the appropriate client integration.

This also creates a clean place to support Claude Code, Codex, Copilot, OpenCode, local model tooling, and future clients without branching the graph architecture.

## Non-goals

Praxis does not attempt to normalize every proprietary client feature. It normalizes the capabilities graphs need and adapts available client mechanisms to them.