# SPEC-015: Dynamic Command Registry and Package Distribution

- Status: Draft
- Governing ADRs: 013, 020, 021, 025, 037, 038, 041, 046
- Depends on: SPEC-004, SPEC-007, SPEC-011

## Purpose

Define how installed Praxis packages/plugins expose CLI/client entry points dynamically and how packages are discovered, inspected, installed, updated, and removed through a replaceable distribution adapter. GitHub Releases is the initial transport.

## Core invariants

1. Praxis core SHALL NOT hard-code domain/package entry points.
2. Active package command surfaces derive from installed `InvocationContract`s.
3. Core lifecycle/control-plane commands are reserved and stable.
4. Invocation registration does not grant capability or execution authority.
5. Disabled, removed, unverified, or unauthorized packages cannot contribute active entry points.
6. Alias collisions and core-command shadowing fail closed.
7. Help, completion, option parsing, client materialization, and runtime validation derive from the same registered contract source.
8. Distribution transport is replaceable and never becomes root of trust.
9. Installed package identity is immutable/digest-bound and does not depend on mutable branch state.
10. Install/update/uninstall change package state and command registration atomically from the user's perspective.

## Core command surface

The CLI MAY compile only Praxis control-plane commands such as:

`discover`, `info`, `install`, `update`, `uninstall`, `list`, `help`, `status`, `resume`, `cancel`, `doctor`, `version`.

The reserved command set SHALL be explicit and versioned.

## Invocation registry

The runtime SHALL maintain a durable registry keyed by stable entry-point ID and aliases. Each active entry contains:

- package ID/version/content digest;
- graph ID/version;
- contract version;
- aliases;
- typed options/arguments;
- capability/enforcement declarations;
- package install generation/state;
- activation timestamp;
- contract digest/provenance.

Registration SHALL validate the `InvocationContract`, reserve core command names, detect alias collisions across active packages, and bind the contract to the exact installed package generation.

A registry change SHALL not be accepted if the referenced package generation is not verified/authorized/installed according to SPEC-011.

## Resolution

For a non-core CLI/client entry point:

1. parse entry-point token;
2. resolve alias through the active registry;
3. load the canonical registered `InvocationContract`;
4. validate options/arguments;
5. pin package/graph generation;
6. execute SPEC-004 authority/enforcement flow.

No package-specific import, switch branch, or compiled option table is permitted in the core CLI.

## Lifecycle commands

### `praxis discover [query]`

Queries configured distribution adapters and returns discovery metadata. Results are not installed/trusted merely by appearing.

### `praxis info <package>`

Returns package/release metadata, requested capabilities, enforcement/crypto requirements, invocation contracts, dependencies, signatures/provenance, installed state, and update availability where known.

### `praxis install <package> [version/ref]`

Resolves an immutable release artifact, downloads it, verifies digest/signatures/dependency lock, performs inspection and permission review, obtains required authorization, persists package generation, registers invocation contracts, and activates according to lifecycle policy.

### `praxis update <package> [version/ref]`

Resolves a candidate release, verifies independently, compares capabilities/enforcement/crypto/invocation surfaces, requires reauthorization for expansion/material changes, runs migration/compatibility checks, then atomically activates the new package generation and contract set. Failure preserves the previous active generation.

### `praxis uninstall <package>`

Deactivates/unregisters invocation contracts and removes the package according to state-retention policy. It SHALL NOT delete unrelated durable run/evidence/history state unless an explicit separate retention action is authorized.

## GitHub Releases adapter

The initial adapter SHALL use GitHub Releases APIs rather than mutable branch contents.

The adapter interface SHALL expose at least:

- `Discover(query)`;
- `Info(packageRef)`;
- `Resolve(packageRef, versionConstraint)`;
- `FetchArtifact(resolvedRelease)`;
- `CheckUpdate(installedPackage)`.

The GitHub adapter SHALL map repository/release assets to canonical Praxis package metadata. It SHALL NOT treat release publication as verification or authorization.

A compatible release SHOULD provide canonical manifest, artifact/archive, digest, signature envelope(s), dependency lock where applicable, and invocation contract metadata. Missing required security artifacts SHALL fail installation according to package policy.

## Atomic registration semantics

Activation SHALL construct and validate the full candidate invocation set before changing the active registry. If any alias conflicts, contract is invalid, or package transition fails, no new entry points become visible.

Update SHALL not unregister the previous working command surface until the replacement generation and its complete invocation set are ready to activate.

Uninstall SHALL make invocation aliases unavailable before removed package code can be launched again.

## Security requirements

- Reserved core aliases cannot be registered by packages.
- Package A cannot replace Package B's alias through update/install ordering.
- Registry rows reference immutable package generation/content digest.
- Registry data is authoritative runtime metadata and cannot be sourced from client prompt/instruction files.
- `InvocationContract` content is validated but remains non-authoritative for capabilities beyond its declared request; actual grants come from deterministic authority state.
- Package update permission expansion follows SPEC-011 reauthorization rules.
- Distribution network/content is untrusted until cryptographic/digest verification succeeds.
- Rollback restores the prior verified package generation and its prior invocation contracts, not consumed leases/approvals.

## Acceptance tests

1. install a fixture package and its new alias appears without recompiling core CLI;
2. fixture package options are accepted/rejected solely from its registered `InvocationContract`;
3. uninstall removes aliases immediately while durable run history remains queryable;
4. disabled package contributes no active alias;
5. package cannot register `status`, `install`, or another reserved core command;
6. two packages claiming the same alias fail activation deterministically;
7. update changes command/options atomically and rollback restores prior contract;
8. capability-expanding update requires reauthorization before command activation;
9. help/completion/list derive from active registry and match runtime validation;
10. GitHub Releases discovery does not imply trust/install/authorization;
11. mutable repository branch changes do not mutate an installed immutable generation;
12. a distribution adapter can be replaced in tests without changing lifecycle/registry semantics;
13. failed update preserves prior active package and invocation surface;
14. stale/removed registry entry cannot execute missing or non-active package generation.

## Deliverables

- durable invocation registry schema/store;
- reserved core-command registry;
- package activation/deactivation transaction/service;
- distribution adapter interface;
- GitHub Releases adapter;
- CLI lifecycle commands;
- dynamic help/list/completion resolver;
- package update comparison/activation path;
- integration/adversarial tests.
