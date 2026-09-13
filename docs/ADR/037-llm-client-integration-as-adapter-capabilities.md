# ADR-037: LLM Client Integration as Adapter Capabilities

- Status: Draft
- Date: 2026-09-13

## Context

Praxis graphs must be usable from heterogeneous LLM clients such as coding CLIs, desktop assistants, IDE agents, local model shells, and future clients. These clients expose different extension mechanisms: skills, slash commands, hooks, MCP/tool servers, plugins, instruction files, lifecycle callbacks, subprocess invocation, or no native extension mechanism at all.

Binding a graph's semantics directly to one client's skill or hook format would make the client the architecture. Praxis instead needs clients to be interchangeable surfaces over persistent graphs and agents.

The user-facing interaction should still feel native to the host client. For example, a client may expose a Praxis skill or slash command such as:

```text
/praxis develop <options> --dashboard
```

In this example, `develop` is an installed graph/package entry-point alias. It is not a core Praxis concept. `--dashboard` is an invocation affordance requesting that the Praxis dashboard be launched or attached to the resulting run.

## Decision

LLM clients are integration adapters. Praxis graphs and agents remain client-neutral and execute under the Praxis runtime.

A client adapter advertises a machine-readable `ClientCapabilityProfile` describing supported integration mechanisms and lifecycle events. Praxis selects the richest safe integration available for that client and graph.

### Canonical invocation model

Praxis SHALL expose a client-neutral typed invocation contract. Client-native skills, slash commands, command palettes, MCP tools, and CLI commands materialize that contract rather than defining independent behavior.

The canonical user-facing shape is:

```text
/praxis <entry-point> [arguments] [options]
```

Equivalent non-slash surfaces MAY use:

```text
praxis <entry-point> [arguments] [options]
```

or a structured tool invocation carrying the same canonical fields.

An installed graph/package MAY declare one or more entry-point aliases, for example:

```text
/praxis develop --dashboard
/praxis research --depth deep
/praxis incident --target production
```

Each entry point resolves to a stable graph/package identity and version before execution. Aliases are UX conveniences, not durable identity.

### Invocation contract

Each user-invocable graph/package entry point SHALL declare a versioned invocation contract containing at minimum:

- stable package/graph identity and version constraints;
- entry-point name and aliases;
- synopsis and help text;
- typed positional arguments;
- typed named options and defaults;
- required and optional capabilities;
- required authority or approval gates;
- interactive versus unattended execution rules;
- supported presentation affordances such as `--dashboard`;
- degradation behavior when an affordance is unavailable;
- whether the entry point may resume an existing run;
- expected output/result classes.

Client adapters generate native UX from this contract. They MUST NOT invent semantic options that the underlying contract does not understand.

### Integration modes

The canonical modes, in descending preference when supported and appropriate, are:

1. **Tool/protocol integration**: MCP, native plugin/tool protocol, or equivalent structured invocation. Preferred for commands, queries, graph execution, state inspection, and governed tool access.
2. **Lifecycle hooks**: pre/post prompt, pre/post tool, session start/stop, idle, compaction, or equivalent callbacks. Preferred for observation, context synchronization, event capture, policy checks, and automatic graph continuation where the client exposes reliable hooks.
3. **Skills/commands**: generated client-native skills, slash commands, or command palettes. Preferred as discoverable user entry points and thin wrappers over Praxis invocation contracts, not as the authoritative graph implementation.
4. **Context/instruction projection**: generated minimal instruction/context files when the client supports persistent project/user instructions. Used to teach the client how and when to invoke Praxis, not to encode the graph itself.
5. **CLI/subprocess bridge**: `praxis` CLI invocation for clients with shell/tool access but no richer extension API.
6. **Manual bridge**: explicit user invocation as the compatibility floor.

These modes may be composed. For example, a client can expose `/praxis develop --dashboard` as a skill that invokes a structured Praxis tool while lifecycle hooks capture observations from the session.

### Hooks are optional accelerators

Hooks SHALL NOT be required for graph correctness. A graph must remain executable on a client without hooks if its required canonical capabilities are otherwise available.

Hooks may improve:

- automatic observation capture;
- continuity across client sessions;
- context synchronization;
- policy interception before side effects;
- idle detection and continuation;
- post-action evidence gathering.

When a hook is unavailable, the adapter degrades to explicit invocation, polling/query surfaces, or Praxis-owned lifecycle management. Missing hooks may reduce convenience or observability but must not silently change graph semantics.

### Generated integration artifacts

Praxis may generate client-specific skills, hook configuration, MCP manifests, command definitions, or instruction projections from a graph/package's integration contract. Generated artifacts are adapters and may be regenerated; they are not authoritative state.

Installation records which generated artifacts were materialized for each client and their source package/version so they can be upgraded or removed deterministically.

### Graph integration contract

A graph/package may additionally declare:

- automatic trigger opportunities;
- lifecycle events it can consume;
- required and optional client capabilities;
- context it needs projected into the client;
- commands/queries it exposes;
- permissions and external effects;
- degradation behavior when capabilities are absent.

Installation resolves this contract against the selected client's capability profile and presents any required permissions or unsupported capabilities.

### Presentation affordances

Presentation requests such as `--dashboard` are orthogonal to graph semantics.

For example:

```text
/praxis develop --dashboard
```

means:

1. resolve the `develop` entry point;
2. validate and normalize its invocation contract;
3. create or resume the canonical Praxis run;
4. request the `dashboard` presentation capability;
5. attach or launch the dashboard if available;
6. continue the same run even if the dashboard cannot be presented, unless the user or package explicitly made that capability required.

A graph must not contain special execution logic merely because it was launched from a dashboard-capable client.

### Authority and security

Client adapters cannot bypass ADR-035 command authority, package permissions, graph governance, or capability controls. Hooks are observations/triggers, not unrestricted mutation channels. Skills are UX surfaces, not trusted code merely because the client calls them.

Natural-language arguments and client-supplied context are untrusted input until parsed and validated into the typed invocation contract.

Client credentials remain owned by the client where possible. Praxis should use the user's existing authenticated client session/CLI rather than require provider API keys when an adapter can invoke that client safely.

### Portability

A graph that requires only canonical Praxis capabilities should move between clients without modification. Client-specific optimization belongs in adapter metadata or optional integration overlays, never in the graph's core semantics.

## Consequences

Praxis can integrate deeply with clients that expose hooks/MCP while remaining usable through skills, slash commands, or CLI on less capable clients. A catalog graph can declare what it needs once and installation can materialize the appropriate client integration.

`/praxis <entry-point>` becomes a consistent mental model across clients while still allowing each host to implement the UX in its native extension mechanism.

This also creates a clean place to support Claude Code, Codex, Copilot, OpenCode, local model tooling, and future clients without branching the graph architecture.

## Non-goals

Praxis does not attempt to normalize every proprietary client feature. It normalizes the capabilities and invocation semantics graphs need and adapts available client mechanisms to them.
