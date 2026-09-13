# ADR-025: Catalog Trust, Provenance, and Signing

- Status: Draft
- Date: 2026-09-13

## Context

Praxis may use a shared catalog of graphs, agents, procedures, evaluators, and packages to reduce cold-start learning. Catalog artifacts can influence execution and therefore become part of the system's trust boundary.

Distribution convenience must not imply execution trust. GitHub may be a practical catalog transport, but repository visibility, stars, ownership, or source location are not sufficient trust signals.

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

### Trust model

Trust is evaluated along separate dimensions rather than collapsed into one boolean:

- integrity: whether the bytes match the identified artifact;
- provenance: who published the artifact and through which lineage;
- capability risk: what the artifact requests permission to do;
- compatibility: whether the artifact targets the installed Praxis contracts;
- evidence quality: what measured behavior supports its profile or claims;
- local trust decision: what the current user or organization has chosen to allow.

A valid signature proves integrity and publisher association. It does not prove that the artifact is safe, appropriate, or high quality.

### Installation authority

Installation and execution are separate decisions.

Praxis may download or inspect an untrusted artifact without granting it execution authority. Before first execution, requested capabilities must be resolved against local policy and the user's authorization. Capabilities not granted are denied by default.

An update that requests broader capabilities is treated as a new authorization event. Existing grants do not automatically expand.

### Local modification

Any local modification creates a distinct descendant in lineage. The modified artifact must no longer be represented as byte-identical to its upstream signed version. Praxis retains the upstream identity, modification provenance, and resulting local digest.

### Updates

Updates are explicit version transitions. Local personalized descendants retain their own lineage even when their upstream catalog package advances.

A package update may be automatically downloaded if policy allows, but must not silently replace an active local descendant when the update changes behavior, contracts, dependencies, or requested capabilities. Praxis may calculate and present compatibility and migration plans.

## Consequences

Community bootstrap can accelerate learning without turning the catalog into an implicit root of trust. Provenance, tamper detection, capability review, and local authority remain available even if distribution moves away from GitHub.

The model also permits private catalogs, organization-approved registries, and purely local packages without changing core trust semantics.

## Non-goals

This ADR does not define a centralized approval authority or require every private local graph to be signed. It does not treat popularity or publisher reputation as proof of safety.
