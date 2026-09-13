# ADR-021: Privacy, Scope, and Catalog Contribution

- Status: Draft
- Date: 2026-09-13

## Context

Praxis 2 learns from human behavior, contextual preferences, and agent history. Reusing that learning across machines, organizations, agents, or public catalogs creates privacy and scope risks.

## Decision

All learned artifacts carry explicit scope and provenance. Private local learning is the default.

Promotion to broader scopes is deliberate:

- agent-private
- human-private
- context/project
- organization/team
- shared local catalog
- public catalog

Artifacts contributed beyond their original scope must be generalized and stripped of unnecessary user-specific, project-specific, credential, secret, or sensitive contextual data. Public/catalog contribution is never an automatic consequence of local learning.

Packages fetched from a catalog do not gain access to private memory by default; access is mediated by capability, policy, and preference contracts.

## Consequences

Praxis can benefit from collective reusable process without turning personal adaptation into involuntary data sharing.
