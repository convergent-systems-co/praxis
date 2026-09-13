# SPEC-014: Preferences and Behavioral Profiles

- Status: Draft
- Governing ADRs: 008, 014, 023, 026, 030, 040
- Depends on: SPEC-001, SPEC-006, SPEC-012, SPEC-013

## Purpose

Define install-time preference contracts, scoped preference precedence, learned drift, behavioral profiles, and matching without conflating user preference with security policy or authority.

## Core invariants

1. Explicit scoped user preference outranks learned preference for the same scope.
2. Narrower applicable scope outranks broader scope when authority class is equal.
3. Package defaults seed behavior but do not override explicit user choices.
4. Learned preference cannot override deterministic policy/invariant.
5. Behavioral-profile evidence classes remain distinguishable: declared, inherited, observed, measured, confirmed.
6. Matching is advisory selection/ranking, not authorization.

## Preference contract

A package MAY declare typed preference slots with stable slot ID/version, description, value schema, required/optional flag, package default, named presets, applicable scopes, privacy/sensitivity metadata, migration rules, and whether learning may adapt the slot.

Installation SHALL ask only for required unresolved preferences and SHOULD offer meaningful presets/defaults where safe.

## Preference record

A record SHALL contain slot ID/version, value, scope, source class, provenance, confidence for learned records, creation/update time, supersession, and optional validity window.

Source classes SHALL include at least explicit_user, explicit_org, package_default, preset_seed, learned, and migrated.

## Resolution precedence

For a requested context, Praxis SHALL:

1. exclude inapplicable/expired/superseded records;
2. enforce policy constraints separately;
3. choose the narrowest applicable scope among records of the highest-authority source class;
4. prefer newer explicit correction over older explicit value at identical scope;
5. use learned confidence only to rank competing learned records, never to outrank explicit records;
6. fall back to preset/package default only when no stronger applicable value exists.

## Scope

Scope inheritance SHALL use canonical Praxis scope semantics. User/global, organization, package/domain, project/workspace, graph, agent, and run/task scopes MAY be represented. A child scope cannot silently rewrite the parent record; it creates an override record.

## Learning and drift

Learnable slots MAY accumulate observations/candidates. Drift SHALL be measured against current explicit/default baseline. Promotion of a learned value follows SPEC-012 governance and preserves provenance.

Explicit correction immediately supersedes learned behavior in the corrected scope and may generate negative learning evidence.

## Behavioral profile

A behavioral profile describes how a graph/agent/package tends to operate, not what it is authorized to do. Dimensions MAY include interaction style, autonomy, planning depth, verification rigor, latency/cost posture, verbosity, risk posture, collaboration/handoff style, and domain-specific dimensions supplied by packages.

Every dimension value SHALL include evidence class, provenance, confidence where applicable, observation window, and last-updated time.

## Matching

Catalog/install/runtime matching MAY rank candidates by goal fit, environment compatibility, required capabilities, preferences, and behavioral profile fit. Security/policy compatibility is a hard eligibility filter before preference/style ranking.

A better profile match cannot compensate for missing enforcement/capability/security requirements.

## Acceptance tests

1. explicit project preference outranks learned project preference;
2. explicit narrower scope outranks explicit broader scope;
3. package default is used only when stronger applicable records are absent;
4. learned high-confidence value cannot override explicit user value;
5. policy constraint rejects an otherwise preferred value;
6. correction supersedes learned value without deleting lineage;
7. profile declared/observed/measured values remain distinguishable;
8. matching rejects security-incompatible package before style score;
9. preference migration preserves explicit user intent/provenance;
10. two users install same package and resolve different seeded/local preference states.

## Deliverables

- preference contract/slot/record schemas;
- preset and install-time resolution mechanism;
- scope/source precedence resolver;
- preference migration interface;
- learned drift/promotion integration;
- behavioral profile/evidence schema;
- matching eligibility/ranking interface;
- conformance fixtures for conflicts and migration.
