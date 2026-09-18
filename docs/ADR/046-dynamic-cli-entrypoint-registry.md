# ADR-046: Dynamic CLI Entrypoint Registry

- Status: Draft
- Date: 2026-09-13
- Related: ADR-020, ADR-021, ADR-037, ADR-041, ADR-045

## Context

Praxis packages/plugins may expose user-facing entry points such as `develop`, `research`, `goals`, or future domain-specific commands. Hard-coding those subcommands and their options into the main `praxis` CLI would couple the control-plane binary to every package, require a CLI rebuild for plugin installation, and make package uninstall/disable semantics inconsistent with the visible command surface.

The existing `InvocationContract` already defines a package-owned command surface: entry-point identity, aliases, options, graph binding, required capabilities, and enforcement requirements.

## Decision

The main Praxis CLI SHALL NOT hard-code package/plugin subcommands, aliases, or package-defined options.

Installed/active packages register one or more versioned `InvocationContract`s with the local Praxis invocation registry. The main CLI dynamically loads active contracts and uses them to parse, validate, normalize, help-render, and dispatch package invocations.

Praxis-owned control-plane commands MAY remain built into the CLI. Examples include `help`, `status`, `resume`, `cancel`, `install`, `uninstall`, `doctor`, and registry/package administration. These operate the Praxis runtime itself rather than a package/domain graph.

## Registration lifecycle

Package installation SHALL bind invocation contracts to the exact verified package ID, package version, and content digest.

Expected lifecycle:

```text
verified package
    -> inspect invocation contracts
    -> detect alias/entrypoint conflicts
    -> authorize package/capability changes
    -> install package
    -> atomically register invocation contracts
    -> activate package
    -> contracts visible to CLI
```

Disable/remove SHALL make those contracts unavailable without modifying/rebuilding the CLI binary.

Update SHALL atomically replace the prior package version's invocation contracts only after the new package has passed verification/authorization. Failed updates SHALL leave the prior active command surface intact.

## Ownership and identity

Every registered invocation contract SHALL bind to:

- package ID;
- package version;
- package content digest;
- graph ID/version;
- entry-point ID;
- aliases;
- option schema;
- required/optional capabilities;
- required enforcement properties;
- mediation requirements;
- registry state/version.

A package cannot register a contract claiming another package identity.

## Namespace conflicts

Aliases and canonical entry-point names SHALL be globally unique within one active Praxis registry unless a future explicit namespace mechanism is adopted.

Installation/update SHALL fail before activation when a command or alias conflicts with an existing active package or reserved Praxis control-plane command.

Packages SHALL NOT shadow core control commands.

## Trust and authority

CLI registration is discoverability, not authority.

A registered command does not grant its package any capabilities. Invocation continues through canonical command normalization and deterministic policy/capability/approval/effect enforcement.

Package signatures/provenance do not grant command authority; they establish package identity/integrity only.

## Help and introspection

`praxis help` SHALL render package command/option information from the active registry rather than compiled domain imports.

The registry SHOULD support machine-readable introspection so LLM-client adapters, TUIs, GUIs, shell completion, MCP/tool surfaces, and generated skills can derive the same command surface.

## Client integration

Generated client skills/hooks/commands SHALL consume the same invocation registry. They SHALL NOT maintain an independent hard-coded list of package entry points.

Installing or disabling a package therefore changes the canonical Praxis command catalog first; client-specific materializers may then regenerate or refresh their native surfaces from that catalog.

## Failure behavior

- missing/corrupt registry: package commands fail closed; core recovery/doctor commands remain available;
- invalid registered contract: excluded from active command resolution and reported by diagnostics;
- alias collision: new registration fails atomically;
- disabled/removed package: invocation fails as unknown/unavailable;
- stale client-generated command: runtime registry remains authoritative and rejects unavailable versions.

## Consequences

### Positive

- plugins extend Praxis without rebuilding the CLI;
- domain packages remain decoupled from the main binary;
- one canonical command schema drives CLI, LLM clients, TUI/GUI, completion, and help;
- uninstall/disable immediately removes command availability;
- package update semantics include command-surface evolution.

### Costs

- invocation registry becomes durable runtime state;
- installation/update require atomic command-surface registration;
- client surfaces need refresh/materialization lifecycle;
- core must reserve/control its own command namespace.

## Acceptance direction

Qualification SHALL prove:

1. a newly installed fixture package adds a previously unknown CLI subcommand without rebuilding Praxis;
2. package-defined options are validated from the registered contract;
3. disable/remove makes the command unavailable;
4. update can add/remove options and aliases atomically;
5. alias collision fails without damaging the existing registry;
6. package cannot claim another package identity;
7. package cannot shadow a reserved core command;
8. registration alone grants no capability authority;
9. CLI/help and machine-readable discovery return the same active contract set;
10. client materializers can derive their surfaces from the registry rather than a compiled list.
