# ADR-013: Catalog and Bootstrap Packages

- Status: Draft
- Date: 2026-09-13

## Context

If every Praxis installation must relearn useful graphs, procedures, and agent structures from scratch, users repeatedly pay the same exploration cost.

## Decision

Praxis 2 supports a transport-neutral catalog of reusable packages. GitHub may be the first distribution backend, but the core must not depend on GitHub.

A package may contain:

- graph definitions or graph fragments
- agent seeds
- procedural skills
- hooks and validators
- policy defaults
- evaluators
- capability vocabulary
- preference contracts
- behavioral/profile metadata

Packages are starting priors, not universal best practices. Catalog matching should prefer evidenced fit to the current goal, context, and human working style rather than popularity alone.

Local evolution is separate from catalog evolution. A personalized descendant does not automatically become a new public catalog version. Generalized improvements may be proposed back to the catalog only after removing user-specific state and passing evaluation.

## Consequences

Praxis learns locally but can bootstrap collectively. New users inherit reusable deterministic structure without inheriting another user's private or idiosyncratic state.
