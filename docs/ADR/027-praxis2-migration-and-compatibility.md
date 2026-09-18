# ADR-027: Praxis 2 Migration and Compatibility Strategy

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 is a redesign, not an incremental rename of the current architecture. Existing runtime, contracts, executors, graphs, overlays, evaluation, and learning code may contain useful components, but treating existing behavior as authoritative would constrain the redesign to legacy assumptions.

The redesign branch must be able to reuse proven implementation without inheriting obsolete ontology or contracts by accident.

The same principle applies to installed Praxis packages: upstream graph/package versions, local adaptations, and user-specific descendants must evolve without silently destroying local state or conflating upgrade with replacement.

## Decision

`redesign/praxis2` is the integration branch for the redesign until explicitly changed.

Migration will follow these rules:

1. Praxis 2 ADRs define the target architecture. Existing ADRs do not govern the redesign unless a Praxis 2 ADR explicitly reincorporates a decision.
2. Existing code is evaluated as reusable implementation, not as architectural authority.
3. Reused components must conform to new Praxis 2 contracts rather than forcing the new contracts to preserve legacy shapes.
4. Compatibility adapters may exist at boundaries where they reduce migration cost, but compatibility layers must be explicit and removable.
5. New Praxis 2 schemas and persisted state require versioned migrations. Silent reinterpretation of legacy state is prohibited.
6. Development remains a proving domain, not the defining ontology of the core.
7. Existing tests may be retained when they validate still-required behavior; tests that encode superseded architecture must be replaced rather than blindly preserved.

Migration should proceed in vertical slices that establish executable end-to-end behavior under the new contracts, rather than rewriting every package before any useful execution exists.

### Installed package lifecycle

An installed catalog artifact creates a local installation record that references an immutable upstream version. Runtime state, learned preferences, behavioral evidence, and user modifications belong to the local installation or its descendants rather than to the upstream artifact.

Praxis distinguishes four operations:

- **upgrade**: move to a newer compatible upstream version while preserving applicable local state;
- **migrate**: transform local state or contracts using an explicit versioned migration;
- **rebase**: create a new local descendant based on a newer upstream version while replaying or reconciling local modifications where possible;
- **fork**: intentionally sever automatic upgrade expectations while retaining provenance and ancestry.

These operations must never be treated as equivalent.

### Upgrade behavior

A package may declare compatibility ranges and migration functions. Praxis may recommend or automatically prepare an upgrade, but activation of the new version must preserve the following invariants:

- explicit user configuration is not silently discarded;
- learned state is retained when its schema and semantics remain valid;
- incompatible state requires migration, quarantine, or explicit reset;
- local graph modifications are not silently overwritten by upstream graph changes;
- capability expansion triggers fresh authorization under ADR-025;
- the previous active version remains addressable for rollback until retention policy removes it.

If Praxis cannot prove a safe deterministic merge between local graph modifications and the new upstream graph, it creates a candidate rebase and preserves both lineages rather than guessing.

### Portability

Portable state identifies contracts and artifacts by stable IDs plus versions, not by local file paths or installation-specific identifiers. Export/import must preserve lineage, provenance, preference authority, graph identity, package identity, and migration history.

A moved installation may resolve different executor implementations or machine-local capabilities, but that environmental remapping must not alter the identity of the graph, agent, goal, or durable state being moved.

## Consequences

Praxis 2 can reuse mature pieces without becoming trapped by the previous design. Migration remains measurable and reversible. Compatibility debt is visible rather than implicit.

Installed graphs and agents can evolve without treating user personalization as disposable package data. Upstream improvement and local adaptation can coexist as explicit lineage operations.

## Non-goals

This ADR does not require preserving source or API compatibility with the current main branch. Compatibility is justified only where it materially reduces transition cost without violating the redesign.

It also does not require automatic semantic merge of arbitrary locally modified graphs. When correctness is uncertain, preservation of both lineages is preferred over destructive automation.
