# ADR-014: Preference Contracts and Install-Time Seeding

- Status: Draft
- Date: 2026-09-13

## Context

User-focused graphs and agents often contain meaningful adaptation points. Requiring Praxis to infer every preference from scratch creates unnecessary cold-start cost and frustration.

## Decision

Packages may declare a versioned Preference Contract describing preference slots that materially affect initial execution.

Each slot declares:

- key and description
- type and allowed values where bounded
- whether setup is required, optional, or learnable
- safe default if applicable
- allowed scopes such as global, work, home, organization, project, or goal class
- whether the value may be adapted through later learning

Installation asks only for preferences that materially change the initial path. Users may skip learnable preferences.

Install-time answers seed local preference records with explicit provenance. They are strong initial priors, not immutable configuration unless the contract marks them as policy/invariants.

## Consequences

Catalog packages can start closer to a user's preferred process while remaining adaptive. A package author declares where personalization is expected instead of inventing an ad hoc questionnaire.
