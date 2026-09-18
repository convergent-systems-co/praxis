# SPEC-015: Dynamic Invocation Registry

- Status: Draft
- Governing ADRs: 037, 046
- Depends on: SPEC-001, SPEC-004, SPEC-011

## Purpose

Define the authoritative local registry by which installed/active packages expose user-facing CLI/client entry points without requiring the Praxis CLI or client adapters to hard-code package commands, aliases, or options.

## Core invariants

1. Package/domain entry points are registry-driven, not compiled into the main CLI.
2. Praxis control-plane commands are reserved and cannot be shadowed by packages.
3. Every active entry point binds to one exact verified package ID/version/content digest and one exact graph ID/version.
4. Registry presence grants discoverability only; it grants no capability, approval, policy exception, or execution authority.
5. The same active invocation contracts drive CLI parsing, help/completion, structured client discovery, generated client surfaces, and runtime validation.
6. Install/update/disable/remove changes the command surface atomically with package lifecycle state.
7. Stale generated client commands never override the runtime registry.
8. Invalid, conflicting, stale, disabled, or unverified registrations fail closed.

## Registry record

Each registered invocation record SHALL contain:

- registry record ID/version;
- package ID;
- package version;
- package content digest;
- package install/activation generation;
- canonical `InvocationContract` bytes/digest;
- graph ID/version;
- canonical entry-point ID;
- aliases;
- option/argument schema;
- required/optional capabilities;
- required enforcement properties;
- mediation requirements;
- enabled/disabled state;
- registered/updated timestamp;
- provenance/signature verification reference where applicable.

The registry SHALL reject a contract whose embedded package identity/version does not match the verified package being registered.

## Reserved namespace

The runtime SHALL maintain a versioned reserved core-command set. Initial examples include:

- `help`
- `list`
- `status`
- `resume`
- `cancel`
- `install`
- `uninstall`
- `update`
- `doctor`
- package/registry administration commands

A package SHALL NOT register a canonical entry point or alias matching a reserved core command.

Reserved-name comparison SHALL use the same normalization rules as invocation parsing.

## Uniqueness

Within the active registry:

- canonical entry-point names are unique;
- aliases are unique;
- an alias cannot collide with another canonical entry point;
- canonical names/aliases cannot collide with reserved core commands.

Conflict is detected before activation. Failed registration SHALL leave the previously active registry unchanged.

## Installation transaction

Activation of package-defined command surfaces SHALL conceptually perform:

```text
verify immutable package
  -> validate invocation contracts
  -> verify package identity binding
  -> check reserved names/conflicts
  -> check compatibility
  -> authorize package/capability changes
  -> atomically persist package install generation + invocation records
  -> activate package generation
```

If any step fails, no partial command registration becomes visible.

## Update semantics

An update MAY add/remove entry points, aliases, arguments, or options.

The new command surface SHALL become visible atomically with the new active package generation. The previous surface remains active until successful activation.

Updates that alter capability/enforcement requirements remain subject to SPEC-011 reauthorization rules.

Option/alias changes alone do not automatically grant authority but SHALL be surfaced in update review and compatibility diagnostics.

## Disable/remove semantics

Disabled or removed package generations SHALL not resolve through normal invocation lookup.

Historical registry records MAY remain for audit/replay but SHALL be marked inactive and SHALL NOT be treated as active command definitions.

A running durable graph remains pinned to its historical package/graph identity according to runtime compatibility policy; removing its CLI alias does not rewrite its authoritative history.

## CLI behavior

The main `praxis` CLI SHALL:

1. recognize reserved core commands locally;
2. for any other command, load/query active invocation contracts from the registry;
3. resolve canonical name/alias;
4. validate arguments/options from the registered contract;
5. emit the canonical normalized invocation;
6. continue through normal deterministic authority/enforcement paths.

The normalized invocation SHALL retain the exact active package ID/version/content digest and graph ID/version, not only friendly names. Registry reads recheck canonical contract bytes against the stored contract digest before client parsing. A syntactically valid replacement contract, stale alias projection, disabled generation, or digest mismatch fails closed.

The CLI binary SHALL NOT import domain packages merely to enumerate their invocation contracts.

## Help, list, completion, and machine discovery

`praxis help`, `praxis list`, and shell completion SHALL derive package/domain commands from the registry.

A machine-readable discovery API SHALL expose the same active contracts for:

- LLM-client adapters;
- MCP/tool surfaces;
- generated skills/commands;
- TUI/GUI surfaces;
- shell completion;
- diagnostics.

These surfaces SHALL NOT maintain an independent authoritative catalog.

## Client materialization

Client-native skills/hooks/commands are projections of the active invocation registry.

When registry generation changes, adapters MAY regenerate eagerly or lazily. Until refreshed, stale projected commands may remain visible in a client UI, but runtime resolution SHALL reject commands whose registered package generation is no longer active.

## Persistence and recovery

Invocation registry state is authoritative runtime/package state and SHALL be persisted transactionally.

On restart, the active registry SHALL reconstruct deterministically from persisted package generation + invocation records.

Corrupt/conflicting registry state SHALL fail closed for package commands while preserving access to core recovery/diagnostic commands where possible.

## Security requirements

- Registration never grants capabilities.
- Package signature validity never grants capabilities.
- Package content cannot self-register outside the install/activation boundary.
- A plugin process cannot mutate registry records merely because it is running.
- Alias resolution occurs before canonical command creation and records the resolved stable IDs.
- Runtime enforcement uses the registered exact contract, not client-supplied option metadata.
- Client-projected help/schema is advisory; runtime registry is authoritative.

## Acceptance tests

1. install a fixture package and invoke a previously unknown subcommand without rebuilding/restarting the CLI binary;
2. package-defined option validation comes only from the registered contract;
3. help/list/completion and machine discovery expose the same active contracts;
4. disable makes the command unavailable;
5. remove makes the command unavailable while historical run replay remains valid;
6. update atomically replaces the old command surface;
7. failed update leaves the previous surface intact;
8. alias collision fails atomically;
9. package cannot shadow reserved core commands;
10. package cannot register a contract claiming another package identity/digest;
11. stale client-projected command is rejected when registry generation is inactive;
12. registry registration grants no execution capability;
13. corrupt registry prevents package command execution but preserves recovery diagnostics;
14. CLI source/build has no required imports of installed domain packages for command discovery.

## Deliverables

- durable invocation-registry schema;
- reserved core-command registry;
- atomic package registration/update/disable/remove integration;
- registry query/list/describe API;
- CLI dynamic resolver;
- dynamic help/completion provider;
- machine-readable client discovery provider;
- conformance fixtures for install/update/conflict/stale-client behavior.
