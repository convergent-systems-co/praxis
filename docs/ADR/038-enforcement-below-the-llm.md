# ADR-038: Enforcement Below the LLM

- Status: Draft
- Date: 2026-09-13

## Context

Praxis intentionally moves important execution decisions into deterministic contracts, state machines, schemas, commands, and capability checks. That work is defeated if the final enforcement step is represented only as instructions to an LLM client.

Prompt text, skills, system instructions, generated context, and model-visible policy are advisory from the perspective of the execution boundary. A model can misunderstand, ignore, or route around them. Therefore any security-, authority-, lifecycle-, or side-effect-related rule that depends on model compliance is not an enforcement control.

Praxis must preserve deterministic authority all the way to the action boundary.

## Decision

All authoritative permission and side-effect enforcement SHALL occur outside the LLM, in deterministic code that the model cannot bypass through prompting.

The LLM may propose an action. It may never be the component that decides whether an authoritative action is permitted.

### Required enforcement path

Every authoritative or externally effectful operation SHALL traverse a deterministic enforcement path conceptually equivalent to:

```text
LLM/client intent
      |
      v
Client Adapter
      |
      v
Normalized Praxis Command
      |
      v
Policy / Authority / Capability Gate
      |
      +---- denied ---> typed denial, no side effect
      |
      v
Enforced Tool / Runtime Boundary
      |
      v
Side effect / state mutation
```

The gate and the enforced boundary MUST be executable without an LLM.

### Client integration is not trusted merely because it is client-native

A slash command, skill, hook, MCP server, plugin, or generated instruction file is an integration mechanism, not an authority mechanism.

Client-native artifacts MAY improve UX and MAY participate in enforcement when the client exposes a real non-model permission boundary, but Praxis SHALL NOT assume such enforcement exists.

### ClientEnforcementProfile

Each client adapter SHALL report a machine-readable `ClientEnforcementProfile` distinct from its general capability profile.

The profile SHALL describe, at minimum:

- whether Praxis invocation can be restricted to explicit user gestures;
- whether hooks are observation-only or can trigger execution;
- whether unattended invocation can be blocked or separately authorized;
- whether tool/MCP execution is mediated by a non-LLM permission system;
- whether shell/process execution can be constrained by the host;
- whether filesystem/network access can be constrained by the host;
- whether client permission grants can be inspected and verified deterministically;
- whether approvals are durable, per-session, per-command, or unavailable;
- whether a model can invoke alternate host tools that bypass Praxis enforcement.

Unknown or unverifiable enforcement SHALL be treated as absent.

### Authority composition

An operation is permitted only when all required layers agree:

```text
Praxis policy authorization
AND
package/graph capability authorization
AND
runtime/tool boundary authorization
AND
required client-side enforcement, when the graph declares it mandatory
```

A richer client capability never expands Praxis authority.

### Hooks

Hooks are divided into two classes:

1. **Observational hooks**: emit lifecycle/context observations into Praxis. They SHALL NOT directly mutate authoritative state or cause side effects.
2. **Trigger hooks**: request a Praxis command be evaluated. The request SHALL pass through the same deterministic authority path as an explicit user invocation.

A hook firing is never itself authorization.

Graphs/packages SHALL declare whether automatic trigger execution is optional or required. If required enforcement cannot be guaranteed, installation or invocation SHALL fail closed.

### Explicit user invocation

Clients MAY expose `/praxis ...`, a skill, command palette action, tool call, or equivalent. An explicit invocation establishes user intent to request the operation, but it does not automatically authorize all downstream capabilities.

Downstream side effects remain subject to deterministic Praxis/runtime policy.

### Tool and side-effect mediation

Praxis SHALL prefer execution surfaces it can mediate directly or that provide enforceable host controls.

Where a client gives the model broad shell/filesystem/network access outside Praxis, Praxis cannot claim that the overall client session is sandboxed by Praxis. In that case:

- Praxis SHALL still deterministically enforce all actions routed through Praxis;
- the adapter SHALL report the bypass risk;
- packages requiring exclusive mediation SHALL refuse that client/mode;
- documentation/UI SHALL distinguish `Praxis-enforced` from `client-unconstrained` execution.

### No prompt-only controls

The following SHALL NOT satisfy an enforcement requirement by themselves:

- system prompts;
- `CLAUDE.md`, `AGENTS.md`, Copilot instructions, or equivalent context files;
- skills containing prose rules;
- natural-language approval text;
- model self-reported compliance;
- hook instructions that depend on the model choosing to invoke them;
- conventions such as "always call Praxis before writing files" unless the write path is technically mediated.

These mechanisms MAY communicate policy but are never sufficient evidence that policy was enforced.

### Fail-closed rule

If a package or graph requires an enforcement property and neither Praxis nor the host client can provide it deterministically, Praxis SHALL fail closed before authoritative execution begins.

Degradation to a weaker mode is allowed only when the requirement is explicitly optional in the package contract.

### Audit evidence

For each authoritative action, Praxis SHALL be able to record:

- requested action;
- resolved actor/session;
- applicable policy/capability requirements;
- enforcement decision;
- enforcement component;
- client enforcement evidence when relevant;
- resulting side effect or denial;
- correlation/causation IDs.

The audit record SHALL not depend on the LLM explaining what it believed happened.

## Consequences

Praxis remains deterministic at the actual action boundary rather than only at the planning layer.

Some LLM clients will support strong integration but weak host-level enforcement. They remain usable for advisory or mediated modes, but packages that require stronger guarantees may reject those modes.

This creates a hard distinction between UX integration and security/authority enforcement, preventing generated skills or hooks from being mistaken for controls.

## Non-goals

Praxis does not attempt to sandbox or control arbitrary client-native tools it cannot intercept. It instead detects and declares that limitation and refuses modes whose required guarantees cannot be provided.