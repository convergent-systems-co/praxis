# ADR-025: Catalog Trust, Provenance, and Signing

- Status: Draft
- Date: 2026-09-13

## Context

Praxis may use a shared catalog of graphs, agents, procedures, evaluators, and packages to reduce cold-start learning. Catalog artifacts can influence execution and therefore become part of the system's trust boundary.

## Decision

Praxis catalog artifacts must carry explicit provenance and integrity metadata. The catalog transport may initially be GitHub, but trust semantics must not depend on GitHub itself.

Each installable artifact or package must support:

- immutable version identifiers;
- publisher identity/provenance;
- content digest;
- declared capabilities and permissions;
- dependency manifest;
- preference/adaptation contract;
- evidence/profile metadata;
- compatibility constraints;
- signature or equivalent verifiable integrity mechanism when distributed beyond a trusted local source.

Praxis must distinguish trusted, untrusted, and locally modified artifacts. A catalog package may seed local state but cannot silently acquire broader authority than declared.

Updates must be explicit version transitions. Local personalized descendants retain their own lineage even when their upstream catalog package advances.

## Consequences

Community bootstrap can accelerate learning without turning the catalog into an implicit root of trust. Provenance, tamper detection, and permission review remain available even if distribution moves away from GitHub.

## Non-goals

This ADR does not define a centralized approval authority or require every private local graph to be signed.
