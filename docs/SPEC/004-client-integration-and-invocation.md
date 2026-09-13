# SPEC-004: Client Integration and Invocation

- Status: Draft
- Governing ADRs: 004, 013, 014, 025, 028, 029, 035, 036, 037

## Purpose

Define how Praxis graphs/packages are exposed through heterogeneous LLM clients without coupling graph semantics to any client's skill, hook, slash-command, MCP, plugin, or instruction format.

## Core invariant

The Praxis runtime owns graph identity, invocation semantics, state, governance, permissions, and execution. Client integrations are replaceable adapters.

No client-specific artifact is authoritative for graph behavior.

## Canonical user invocation

The normalized interactive form SHALL be:

```text
/praxis <entry-point> [arguments] [options]
```

A CLI or shell-capable client MAY expose the equivalent form:

```text
praxis <entry-point> [arguments] [options]
```

Structured clients MAY call a Praxis invocation tool/API directly, but the resulting normalized invocation SHALL be semantically identical.

### Full proving example

The software-development proving package SHALL expose an entry point equivalent to:

```text
/praxis develop <options> --dashboard
```

`develop` is a package/graph alias. It SHALL NOT become part of the core ontology.

`--dashboard` requests the `presentation.dashboard` capability. It SHALL NOT alter the development graph's execution semantics.

## InvocationContract

Each user-facing entry point SHALL resolve to a versioned `InvocationContract` containing:

- `contract_version`
- `package_id`
- `package_version`
- `graph_id`
- `graph_version`
- stable `entry_point_id`
- display name and aliases
- synopsis/help metadata
- typed positional arguments
- typed named options
- defaults and validation rules
- mutually exclusive/required-together option groups
- required and optional capabilities
- authority/approval requirements
- execution mode: interactive, unattended, or either
- resume policy
- output/result classes
- requested/optional presentation capabilities
- degradation rules

Aliases SHALL resolve to stable IDs before an authoritative command is created.

## Normalized invocation envelope

Every client adapter SHALL convert host input into a canonical envelope conceptually equivalent to:

```yaml
invocation_version: v1
entry_point_id: praxis.package.develop.default
package_ref: praxis/develop@1
arguments: {}
options:
  dashboard: true
client:
  id: claude-code
  session_id: <opaque>
context_refs: []
requested_capabilities:
  - presentation.dashboard
correlation_id: <uuid>
```

The adapter MAY preserve the raw client input for audit/provenance, but raw natural-language or slash-command text SHALL NOT be used as authoritative execution state.

## Resolution flow

Invocation SHALL execute this sequence:

1. discover/resolve entry-point alias;
2. pin package and graph versions according to installation/update policy;
3. parse and type-check arguments/options;
4. resolve preference defaults and explicit overrides;
5. resolve client capabilities;
6. perform authority and permission checks;
7. create a canonical Praxis `CommandEnvelope` for start/resume;
8. persist the resulting authoritative events;
9. return a `RunReference`;
10. attach requested presentation surfaces when available.

No step may silently expand authority because the host client supports a richer extension mechanism.

## Adapter modes

A client adapter SHALL declare a `ClientCapabilityProfile` and MAY implement any combination of:

- structured tool/MCP invocation;
- client-native skill/slash-command invocation;
- lifecycle hooks;
- context/instruction projection;
- shell/CLI bridge;
- manual copy/paste bridge.

The installer SHOULD choose the richest safe combination, but graph execution MUST depend only on declared required canonical capabilities.

## Skills and slash commands

Skills/slash commands are the preferred discoverable UX where supported.

A generated Praxis skill SHOULD expose one top-level namespace, `/praxis`, rather than generating unrelated global commands for every graph.

The namespace SHALL provide:

```text
/praxis <entry-point> ...
/praxis help [entry-point]
/praxis list
/praxis status [run]
/praxis resume <run> [options]
/praxis cancel <run>
```

Clients that cannot implement nested commands MAY generate equivalent native command names while preserving the canonical invocation envelope.

Command help and completion SHOULD be generated from installed `InvocationContract` metadata so the client UI cannot drift from runtime validation.

## Lifecycle hooks

Hooks MAY provide:

- session-start/session-stop observations;
- pre/post prompt observations;
- pre/post tool observations;
- compaction/context-loss notifications;
- idle notifications;
- side-effect interception where supported;
- automatic state/context refresh.

Hooks SHALL NOT be required to start, continue, or complete a graph unless the package explicitly declares a canonical capability that cannot be satisfied another way.

Hook payloads SHALL be treated as observations until validated and promoted through normal Praxis learning/governance paths.

## Tool/MCP surface

A structured Praxis tool surface SHOULD expose canonical operations rather than one tool per graph, including at minimum:

- discover/list entry points;
- describe invocation contract;
- invoke/start;
- status/query run;
- resume;
- cancel;
- submit human decision/approval;
- request/attach presentation surface.

Graph-specific typed arguments SHALL be passed through the invocation contract rather than creating permanently bespoke protocol methods.

## Context projection

Generated `AGENTS.md`, `CLAUDE.md`, Copilot instructions, client skills, or equivalent files MAY explain how to invoke Praxis and expose current package metadata.

They SHALL NOT contain the authoritative graph definition or durable adaptive state.

Generated context SHOULD be minimal to reduce token cost and stale-instruction risk.

## Dashboard semantics

`--dashboard` SHALL mean `request presentation.dashboard`.

If available, the adapter/runtime SHALL launch or attach the dashboard to the canonical run reference. If unavailable and optional, execution continues and returns the run reference plus a capability warning. If explicitly required, invocation fails before execution with a capability-resolution error.

The dashboard SHALL consume authoritative projections/events and SHALL NOT become a parallel state store.

## Install and materialization

Installing a package into a client SHALL:

1. inspect package invocation/integration contracts;
2. inspect the client's capability profile;
3. determine supported materializations;
4. show required permissions/capability gaps;
5. generate native skills/hooks/tool manifests/instructions as applicable;
6. record generated-artifact provenance and package version;
7. support deterministic update and uninstall.

Generated files SHALL be replaceable without losing Praxis state.

## Security requirements

- Client input is untrusted until contract validation succeeds.
- A client adapter cannot grant capabilities the user/runtime policy did not authorize.
- Skills and hooks cannot call internal mutation paths that bypass `CommandEnvelope` validation.
- Client-owned credentials SHOULD remain client-owned.
- Secrets SHALL NOT be projected into prompts/instruction files merely to make an adapter convenient.
- Hook/context inputs SHALL carry provenance sufficient to distinguish client observation from user-confirmed state.

## Failure behavior

Adapters SHALL return typed failures for at least:

- unknown entry point;
- invalid argument/option;
- incompatible package version;
- missing required capability;
- denied permission/authority;
- unavailable runtime;
- unavailable optional presentation capability;
- stale or unknown run reference;
- client adapter protocol mismatch.

Optional integration loss SHALL degrade predictably rather than mutate graph behavior.

## Acceptance tests

The implementation SHALL prove:

1. one invocation contract can materialize into at least two different client integration mechanisms;
2. `/praxis develop --dashboard` and an equivalent structured tool call create the same normalized invocation semantics;
3. removing hook support does not change graph correctness;
4. a missing optional dashboard capability does not prevent the run;
5. a missing required capability fails before authoritative execution begins;
6. generated client artifacts can be removed/recreated without state loss;
7. an adapter cannot bypass package/runtime permissions;
8. help/completion metadata and runtime validation derive from the same invocation contract;
9. aliases resolve to stable graph/package identities in emitted events;
10. run status/resume/cancel work across different clients against the same persistent run.

## Deliverables

- `InvocationContract` schema and bindings;
- `ClientCapabilityProfile` schema and bindings;
- normalized invocation parser/resolver;
- adapter SDK/interface;
- structured tool/MCP adapter;
- at least one skill/slash-command adapter;
- CLI compatibility adapter;
- generated-artifact inventory/update/uninstall mechanism;
- dashboard presentation-capability adapter;
- conformance tests and fixtures.

## Non-goals

This specification does not define the internal content of the `develop` graph. Software development is a proving/domain package and will have its own specification.
