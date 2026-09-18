# Customization Guide

## Safe extension model

Customize Praxis by adding versioned, digest-bound content behind the existing contracts. A customization can request authority, but it cannot redefine who owns authoritative state or silently turn model output into approval.

## Graphs

A graph is a JSON document with a `spec_version`, unique node IDs, legal edges, an entry node, terminal nodes, and optional node metadata for requirements, evidence, resources, and bounded execution parameters. Validate it before running:

```bash
praxis doctor --graph examples/sample-graph.json
```

Keep graph identity and version stable for replay. Create a new graph version when control-flow or evidence semantics change. Do not identify a graph by its position in a package or by a mutable filename alone.

## Agents and package contents

Agent definitions are package content that bind an agent template to exact graph generations, profiles, memory contracts, and requested capabilities. They are not shared identities. A local instantiation creates a distinct governed identity and lineage.

Supported package content kinds include graphs, agent definitions, plugins, plugin executables, preference contracts, behavioral profiles, templates, migrations, and documentation. Each content item needs a stable ID, version, digest, and canonical artifact reference.

## Invocation contracts

Domain commands are registered from package-owned `InvocationContract`s. The core CLI reserves lifecycle commands and rejects alias collisions. A package command must bind its package ID/version, graph ID/version, options, and content digest. Do not add a hard-coded domain command to `cmd/praxis`.

## Executors and providers

Implement the executor contract for a new provider, then expose it through the supported plugin/entry-point mechanism. An executor must report capabilities and health separately, preserve authentication ambiguity, and return bounded evidence. It must not claim readiness because a caller requested it. Test unavailable, unauthenticated, policy-denied, timeout, cancellation, and restart paths.

The built-in adapters are local subprocess, fake, Claude CLI, Codex CLI, Copilot CLI, Ollama, and MLX. Provider-specific credentials belong to the provider, not to graph or package content.

## Preferences, policies, overlays, and hooks

Preferences are scoped, versioned inputs with explicit precedence and correction semantics. Policies decide eligibility, budgets, isolation, authority, and effect legality. Overlays may add domain graphs, resources, graders, and capability vocabulary through a manifest. Hooks or skills are adapters and observations; they do not own state transitions or approval.

An overlay that declares a proof type must register a deterministic grader for it. Unknown security-relevant content, missing manifest declarations, incompatible schema versions, and unsupported authority semantics fail closed.

## Lifecycle and updates

Create a new immutable package generation for a material customization. Installation verifies signed bytes and persists the exact archive/content. Update reviews capability, enforcement, crypto, invocation, graph, plugin, and migration changes. Rollback switches package registrations atomically but does not rewind consumed approvals, leases, plugin sessions, or agent identity state.

## Testing customizations

At minimum test:

```bash
python -m pytest
go test ./...
python scripts/check_clean_install.py
```

Also test restart/replay, duplicate and stale operations, invalid and untrusted content, provider replacement, policy denial, package collisions, and evidence-gate failures. A green unit test is not proof that a customization may mint authority.
