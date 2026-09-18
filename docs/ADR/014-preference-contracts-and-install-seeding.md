# ADR-014: Preference Contracts and Install-Time Seeding

- Status: Draft
- Date: 2026-09-13

## Context

User-focused graphs and agents often contain meaningful adaptation points. Requiring Praxis to infer every preference from scratch creates unnecessary cold-start cost and frustration.

Preferences also exist at different scopes and from different authorities. Without deterministic resolution, catalog defaults, user choices, learned preferences, organization policy, and graph-local adaptation can silently conflict.

## Decision

Packages may declare a versioned Preference Contract describing preference slots that materially affect initial execution.

Each slot declares:

- key and description;
- type and allowed values where bounded;
- whether setup is required, optional, or learnable;
- safe default if applicable;
- allowed scopes such as global, organization, work, home, project, goal class, graph instance, or agent instance;
- whether the value may be adapted through later learning;
- whether the slot is advisory, preference, constraint, or policy;
- migration behavior when the contract changes.

Installation asks only for preferences that materially change the initial path. Users may skip learnable preferences.

Install-time answers seed local preference records with explicit provenance. They are strong initial priors, not immutable configuration unless the contract marks them as policy/invariants.

### Preference authority

Praxis resolves preference values by authority first and specificity second.

The authority order is:

1. explicit runtime human instruction for the current execution;
2. explicit persisted human configuration;
3. organization or system policy that is authorized to govern the current scope;
4. graph or package configuration explicitly chosen by the user during installation or later configuration;
5. learned local preference with sufficient evidence;
6. catalog/package recommended default;
7. package safe default.

A lower-authority source may not overwrite a higher-authority source. Learning may propose a change to an explicit value, but may not silently mutate it.

Within the same authority class, the most specific applicable scope wins. For example, an explicit project preference overrides an explicit global preference for that project. Scope specificity must be deterministic and represented in the canonical domain model rather than inferred from natural language.

### Provenance

Every resolved preference must retain:

- source authority;
- source scope;
- origin package/graph when applicable;
- version of the preference contract;
- whether the value was explicit, seeded, inferred, or learned;
- confidence/evidence when learned;
- creation and last-confirmed timestamps.

Praxis must be able to explain why a particular preference value was selected.

### Contract evolution

Preference contracts are versioned independently from graph execution state. Package updates may add optional slots freely, but may not reinterpret an existing key with incompatible semantics without a migration.

When a slot is renamed, split, merged, or changes type, the package must provide a migration or Praxis must treat the new slot as unset. Silent reinterpretation is prohibited.

## Consequences

Catalog packages can start closer to a user's preferred process while remaining adaptive. A package author declares where personalization is expected instead of inventing an ad hoc questionnaire.

Preference resolution is deterministic and auditable. User intent remains authoritative while still allowing Praxis to learn lower-level working preferences over time.

## Non-goals

This ADR does not make every operational parameter a preference. Deterministic implementation configuration, credentials, capabilities, and security policy remain separate concerns even when surfaced through the same installation experience.
