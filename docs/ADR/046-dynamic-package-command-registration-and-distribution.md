# ADR-046: Dynamic Package Command Registration and Replaceable Distribution

- Status: Draft
- Date: 2026-09-13
- Related: ADR-013, ADR-020, ADR-021, ADR-025, ADR-037, ADR-038, ADR-041

## Context

Praxis packages/plugins can add user-facing graphs and entry points. If the main CLI hard-codes domain commands, every package addition requires a Praxis core rebuild and domain semantics leak into the control plane. This violates the package/plugin architecture and prevents third-party extensions from becoming first-class CLI/client experiences.

Packages also require a concrete discovery and lifecycle mechanism. The first transport will use GitHub Releases, but distribution location must not become package identity or root of trust.

## Decision

Praxis SHALL separate its CLI into:

1. a stable **core control plane** owned by Praxis; and
2. a **dynamic package command plane** materialized from active installed `InvocationContract`s.

Core control-plane commands MAY include:

- `discover`
- `info`
- `install`
- `update`
- `uninstall`
- `list`
- `help`
- `status`
- `resume`
- `cancel`
- `doctor`
- `version`

Domain/package entry points such as `goals`, `develop`, `research`, and future third-party commands SHALL NOT be compiled into the main CLI as package-specific imports or switch cases.

Installing/activating a package registers its declared invocation contracts. Disabling/removing it removes those contracts from discovery. Updating replaces the registered contract set atomically with the activated package generation.

## Invocation ownership

The canonical package artifact SHALL include one or more versioned `InvocationContract`s when the package exposes user-facing entry points.

Invocation metadata controls:

- aliases/entry-point names;
- arguments/options and validation;
- help/synopsis/completion;
- package/graph identity and version;
- required/optional capabilities;
- required enforcement properties;
- presentation/degradation metadata.

Registration is metadata publication only. It does not grant execution capabilities or authority.

## Collision policy

Core control-plane command names are reserved and cannot be claimed by packages.

Package alias collisions SHALL fail deterministically before activation unless an explicit future namespace policy resolves them. First-installed-wins behavior is prohibited because install order must not silently change command meaning.

## Distribution abstraction

Praxis SHALL define a replaceable distribution/catalog interface supporting at least:

- discover/search;
- info/metadata;
- release resolution;
- artifact retrieval;
- update availability.

GitHub Releases is the initial adapter. GitHub repository identity, release tags, download counts, stars, or ownership are discovery/provenance inputs, not execution trust or package identity.

Package identity remains the verified immutable package manifest/content digest/signature/dependency-lock semantics defined by SPEC-011.

## GitHub Releases initial convention

A Praxis-compatible GitHub release SHOULD publish:

- canonical package manifest;
- immutable package artifact/archive;
- digest metadata;
- signature envelope(s);
- dependency lock where applicable;
- invocation contracts;
- optional human-readable release notes.

The adapter SHALL resolve a release to immutable artifact metadata before installation. Mutable branch contents SHALL NOT define an installed package.

## Lifecycle

The user-facing lifecycle is:

```text
discover -> info -> install -> active -> update/disable -> uninstall
```

Installation SHALL perform verification, inspection, transitive capability review, authorization, durable installation, and invocation registration before activation.

Update SHALL resolve a new immutable release, verify it independently, compute permission/enforcement/crypto/invocation changes, require reauthorization where needed, then atomically switch package generation and invocation registration.

Uninstall SHALL deactivate/unregister entry points before or atomically with package removal so stale commands cannot launch removed code.

## Client materialization

LLM-client skills, slash commands, command palettes, completion, and help SHOULD derive from the same active invocation registry as the CLI. Client adapters may cache/materialize artifacts, but runtime resolution remains authoritative.

## Security invariants

- Package command registration never grants a capability.
- Signed package metadata does not imply authorization.
- Disabled/removed/unverified packages cannot contribute active invocation contracts.
- Alias collisions and attempts to shadow reserved core commands fail closed.
- Update cannot silently broaden requested capabilities/enforcement/crypto requirements.
- Runtime invocation pins package/graph identity from the registered contract before execution.
- Distribution transport cannot override package trust policy.

## Consequences

Praxis core no longer needs rebuilding for package-defined commands. CLI/client UX becomes extensible while retaining a stable deterministic control plane. Distribution can later move beyond GitHub Releases without changing package identity or lifecycle semantics.
