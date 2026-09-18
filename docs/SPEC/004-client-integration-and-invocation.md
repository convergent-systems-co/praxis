# SPEC-004: Client Integration and Invocation

- Status: Draft
- Governing ADRs: 004, 013, 014, 025, 028, 029, 035, 036, 037, 038

## Purpose

Define how Praxis graphs/packages are exposed through heterogeneous LLM clients without coupling graph semantics to any client's skill, hook, slash-command, MCP, plugin, or instruction format.

## Core invariant

The Praxis runtime owns graph identity, invocation semantics, state, governance, permissions, and execution. Client integrations are replaceable adapters.

No client-specific artifact is authoritative for graph behavior.

**No model-visible instruction is an enforcement control.** Any rule whose correctness depends on whether an LLM chooses to comply is advisory only.

Authoritative permissions, lifecycle transitions, and side effects SHALL be enforced in deterministic code below the LLM.

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
- required enforcement properties
- execution mode: interactive, unattended, or either
- resume policy
- output/result classes
- requested/optional presentation capabilities
- degradation rules

Aliases SHALL resolve to stable IDs before an authoritative command is created.

## ClientCapabilityProfile

Each client adapter SHALL expose a versioned capability profile describing what the host can technically provide, including skills/commands, hooks, structured tools, MCP, shell invocation, instruction projection, presentation surfaces, lifecycle callbacks, and authentication/session reuse.

Technical capability SHALL NOT imply authorization or enforceability.

## ClientEnforcementProfile

Each client adapter SHALL also expose a separate versioned `ClientEnforcementProfile` describing what the host can enforce independently of model compliance.

At minimum it SHALL declare:

- whether Praxis invocation can require an explicit user action;
- whether lifecycle hooks are observation-only, trigger-capable, or side-effect-capable;
- whether unattended invocation can be separately authorized or denied;
- whether MCP/tool calls are mediated by a host permission boundary outside the model;
- whether shell/process execution can be constrained;
- whether filesystem/network access can be constrained;
- whether host permissions can be inspected deterministically;
- approval persistence: per-action, per-session, durable, or unavailable;
- whether the model has alternate host-native paths that can bypass Praxis mediation.

`unknown` or unverifiable enforcement SHALL be interpreted as `not_enforced`.

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

## Deterministic enforcement path

Every authoritative mutation or external side effect initiated through a client SHALL follow this path:

```text
client/model request
      |
      v
adapter normalization
      |
      v
contract validation
      |
      v
Praxis authority + package capability + enforcement-policy evaluation
      |
      +---- denied ---> typed denial; no mutation or side effect
      |
      v
mediated runtime/tool boundary
      |
      v
state mutation / side effect
```

The authorization decision and the execution gate SHALL run without an LLM.

A model MAY propose an action. It SHALL NOT be the component that determines whether that action is authorized.

## Resolution flow

Invocation SHALL execute this sequence:

1. discover/resolve entry-point alias;
2. pin package and graph versions according to installation/update policy;
3. parse and type-check arguments/options;
4. resolve preference defaults and explicit overrides;
5. resolve client capabilities;
6. resolve and verify client enforcement properties;
7. perform deterministic authority, package permission, capability, and enforcement checks;
8. fail closed when any required enforcement property is unavailable;
9. create a canonical Praxis `CommandEnvelope` for start/resume;
10. persist the resulting authoritative events;
11. return a `RunReference`;
12. attach requested presentation surfaces when available.

No step may silently expand authority because the host client supports a richer extension mechanism.

## Adapter modes

A client adapter SHALL declare a `ClientCapabilityProfile` and `ClientEnforcementProfile` and MAY implement any combination of:

- structured tool/MCP invocation;
- client-native skill/slash-command invocation;
- lifecycle hooks;
- context/instruction projection;
- shell/CLI bridge;
- manual copy/paste bridge.

The installer SHOULD choose the richest safe combination, but graph execution MUST depend only on declared required canonical capabilities and required enforcement properties.

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

A skill, slash command, command palette entry, or generated instruction is a UX entry point. It SHALL NOT be counted as enforcement unless the client exposes a verifiable non-model permission boundary around that invocation.

## Lifecycle hooks

Hooks MAY provide:

- session-start/session-stop observations;
- pre/post prompt observations;
- pre/post tool observations;
- compaction/context-loss notifications;
- idle notifications;
- side-effect interception where supported;
- automatic state/context refresh.

Hooks SHALL be classified as either:

1. `observational`: may emit observations only and cannot directly create authoritative mutations or external side effects;
2. `triggering`: may request a Praxis command, but the request SHALL pass through the full deterministic enforcement path.

A hook firing SHALL NOT itself constitute authorization.

Hooks SHALL NOT be required to start, continue, or complete a graph unless the package explicitly declares a canonical capability and enforcement property that cannot be satisfied another way.

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

Tool and MCP adapters SHALL terminate at a deterministic Praxis boundary. Model-visible tool descriptions MAY explain policy but SHALL NOT be relied upon to enforce it.

## Side-effect mediation

Praxis SHALL distinguish between:

- `praxis_enforced`: the side-effect path is deterministically mediated by Praxis or a verifiable host control;
- `client_enforced`: the host client provides an independently verifiable permission boundary required by the graph;
- `client_unconstrained`: the model has alternate native paths capable of the same side effect outside Praxis mediation.

If a package requires exclusive mediation and the client is `client_unconstrained` for that capability, installation or invocation SHALL fail closed.

Praxis SHALL NOT claim to sandbox or govern actions that occur through client-native tools it cannot intercept.

## Context projection

Generated `AGENTS.md`, `CLAUDE.md`, Copilot instructions, client skills, or equivalent files MAY explain how to invoke Praxis and expose current package metadata.

They SHALL NOT contain the authoritative graph definition or durable adaptive state.

They SHALL NOT satisfy a permission, authority, or enforcement requirement.

Generated context SHOULD be minimal to reduce token cost and stale-instruction risk.

## Dashboard semantics

`--dashboard` SHALL mean `request presentation.dashboard`.

If available, the adapter/runtime SHALL launch or attach the dashboard to the canonical run reference. If unavailable and optional, execution continues and returns the run reference plus a capability warning. If explicitly required, invocation fails before execution with a capability-resolution error.

The dashboard SHALL consume authoritative projections/events and SHALL NOT become a parallel state store.

## Install and materialization

Installing a package into a client SHALL:

1. inspect package invocation/integration contracts;
2. inspect the client's capability profile;
3. inspect the client's enforcement profile;
4. determine supported materializations;
5. verify all required enforcement properties;
6. show required permissions, capability gaps, and bypass risks;
7. generate native skills/hooks/tool manifests/instructions as applicable;
8. record generated-artifact provenance and package version;
9. support deterministic update and uninstall.

Generated files SHALL be replaceable without losing Praxis state.

Installation SHALL fail when a required enforcement property cannot be provided deterministically.

## Security requirements

- Client input is untrusted until contract validation succeeds.
- A client adapter cannot grant capabilities the user/runtime policy did not authorize.
- Skills and hooks cannot call internal mutation paths that bypass `CommandEnvelope` validation.
- Client-owned credentials SHOULD remain client-owned.
- Secrets SHALL NOT be projected into prompts/instruction files merely to make an adapter convenient.
- Hook/context inputs SHALL carry provenance sufficient to distinguish client observation from user-confirmed state.
- Prompt text, generated instructions, model self-report, or natural-language policy acknowledgements SHALL NOT be considered enforcement evidence.
- Unknown enforcement state SHALL fail closed when the property is required.
- Every authoritative action SHALL record which deterministic component allowed or denied it.

## Failure behavior

Adapters SHALL return typed failures for at least:

- unknown entry point;
- invalid argument/option;
- incompatible package version;
- missing required capability;
- missing required enforcement property;
- denied permission/authority;
- unavailable runtime;
- unavailable optional presentation capability;
- stale or unknown run reference;
- client adapter protocol mismatch;
- unenforceable client bypass path when exclusive mediation is required.

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
10. run status/resume/cancel work across different clients against the same persistent run;
11. prompt-only permission instructions do not satisfy an enforcement requirement;
12. a triggering hook passes through the same deterministic authority path as explicit invocation;
13. a client reporting `unknown` enforcement fails closed for packages requiring that enforcement;
14. a model request denied by the policy gate produces no state mutation or external side effect;
15. a package requiring exclusive side-effect mediation refuses a client with an unmediated bypass path;
16. audit evidence identifies the actual enforcing component without relying on model self-report.

## Deliverables

- `InvocationContract` schema and bindings;
- `ClientCapabilityProfile` schema and bindings;
- `ClientEnforcementProfile` schema and bindings;
- normalized invocation parser/resolver;
- deterministic authority/enforcement gate;
- adapter SDK/interface;
- structured tool/MCP adapter;
- at least one skill/slash-command adapter;
- CLI compatibility adapter;
- generated-artifact inventory/update/uninstall mechanism;
- dashboard presentation-capability adapter;
- enforcement conformance fixtures;
- conformance tests and fixtures.

## Non-goals

This specification does not define the internal content of the `develop` graph. Software development is a proving/domain package and will have its own specification.

Praxis does not claim to sandbox arbitrary client-native tools that it cannot intercept. It detects and declares that limitation and refuses any execution mode whose required guarantees cannot be provided.