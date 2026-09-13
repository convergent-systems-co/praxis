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

Contracts and records SHALL be content-addressed and use contract-owned schema-version metadata under ADR-052. A durable preference record binds its exact package contract ID/version. Package contracts own slot descriptions, native allowed values, applicable scope kinds, defaults, required/setup status, and learnability; core does not infer those semantics from slot names or values.

Preference history is append-only authoritative state. Explicit user/organization records require deterministic authorization bound to the subject, scope, slot, authority, and evidence reference; persisted event actor/trust metadata is replay-verified. A caller cannot mint an explicit or learned preference by selecting a source enum. Learned promotion requires a distinct deterministic authority bound to the exact subject, scope, slot, record, and promotion evidence. Corrections append a new record that supersedes an exact active predecessor without deleting it.

An agent generation that declares a preference-contract reference SHALL resolve that exact contract from authoritative history before executing its operational graph. Required unresolved slots fail closed. Every executed node receives the resolved slot/value, preference-record identity, and contract identity; provider replacement or process restart cannot substitute a different contract or erase preference lineage. Scope applicability is explicit package/contract state. Core SHALL NOT infer domain hierarchy from path-like scope strings.

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

Learnable slots MAY accumulate observations/candidates. Drift SHALL be measured against current explicit/default baseline. A package-owned, content-addressed drift policy binds the exact profile-divergence policy, preference contract, slot/value, scope, and confidence. Learned promotion requires the exact divergence report to exist in authoritative profile history and preserves the reference fact, current fact, report, policy, and superseded preference identities. Promotion follows SPEC-012 governance and preserves provenance; favorable profile evidence cannot self-authorize the learned record.

Explicit correction immediately supersedes learned behavior in the corrected scope and may generate negative learning evidence.

The reference one-to-one migration is content-addressed and binds source/target contract identities, transform identity, and explicit slot mappings. A caller cannot mint migrated authority by selecting the migrated source enum: the append event embeds both verified contracts and the exact migration, and replay re-verifies the active origin, slot mapping, value, scope, evidence, confidence, and original authority lineage. It accepts a value only when the target contract independently permits that value and scope. The migrated record preserves its origin record, original source authority/provenance, and supersession lineage; migration does not downgrade an explicit user preference into learned evidence. Split/merge/type transforms require an explicitly versioned package migrator rather than a core guess.

Install-time input discovery returns only required, unresolved, applicable slots without safe defaults. Default seeding produces package-default records only from values declared by the package contract.

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
10. two users install same package and resolve different seeded/local preference states;
11. software-delivery and research packages use incompatible slots/values/scopes through the same ledger;
12. explicit correction, migration lineage, source precedence, and full history survive SQLite restart;
13. caller-selected explicit source without authority fails append and actor mismatch fails replay.
14. software-delivery and research operational graphs consume governed learned drift, then consume an explicit correction after restart and provider replacement while preserving exact record/contract identity;
15. caller-selected learned and migrated source classes fail their generic append paths, and migration rejects a value or scope incompatible with its target contract.

## Deliverables

- preference contract/slot/record schemas;
- preset and install-time resolution mechanism;
- scope/source precedence resolver;
- preference migration interface;
- learned drift/promotion integration;
- behavioral profile/evidence schema;
- matching eligibility/ranking interface;
- conformance fixtures for conflicts and migration.
