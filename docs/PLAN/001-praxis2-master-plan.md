# PLAN-001: Praxis 2 Master Delivery Plan

- Status: Draft
- Branch: `redesign/praxis2`
- Architecture authority: `docs/ADR/*`
- Implementation authority: `docs/SPEC/*`
- Review record: `docs/PLAN/002-praxis2-adversarial-and-security-review.md`

## Purpose

Deliver Praxis 2 as a general platform for persistent, self-improving agents and graphs across domains.

Software development is the first major proving domain and a primary deliverable, but it does not define the core architecture. Workspace Intelligence is a reusable capability and likewise does not make repository semantics part of Praxis core.

## Delivery rules

1. ADRs define architectural decisions and rationale.
2. SPECs define executable contracts, invariants, failure behavior, security properties, and acceptance criteria.
3. PLANs define dependency order, integration gates, and work decomposition.
4. Issues/work bundles are generated from SPEC deliverables and acceptance tests, not directly from ADR prose.
5. Existing Praxis code is salvageable implementation, not architectural authority.
6. A work bundle is complete only when its governing SPEC acceptance and security criteria pass.
7. Cross-domain behavior belongs in core contracts; domain-specific behavior belongs in packages/graphs.
8. Client skills, hooks, MCP definitions, and instruction files are adapters, not authority mechanisms.
9. If an LLM can choose to ignore a control, the control is advisory rather than enforcement.
10. Untrusted content is data, not authority. Content may influence proposals but cannot authorize them.
11. Security guarantees are reported as specific verified capabilities/properties. Unknown means unavailable.
12. Every side effect must remain within deterministic authority through the final commit boundary.
13. Cryptography is profile-driven and algorithm-agile; Praxis-native asymmetric protection prefers standardized post-quantum mechanisms and fails closed when a required cryptographic profile cannot be satisfied.
14. Expensive discovery and durable uncertainty SHOULD be resolved once into versioned reusable planning artifacts when work justifies it; implementation slices consume that baseline and replan only invalidated/local variance.
15. Planning rigor is progressive: narrow low-risk work retains a direct fast path; architecturally material work may require ADRs, architecture views, SPECs, conformance mapping, and a master plan before implementation.

## Cross-cutting security invariants

The following apply to every wave:

- least privilege and deny by default;
- explicit principal identity and provenance;
- capability grants are scoped and revocable;
- no ambient authority is assumed merely because a process/client can technically perform an action;
- security-sensitive approvals bind to canonical action intent and are revalidated before commit;
- one-shot authority is non-replayable;
- ambiguous external side effects are reconciled rather than blindly retried;
- derived state, model output, repository content, and tool output cannot self-promote into authority;
- sensitive data release is destination-aware and policy mediated;
- quotas bound graph work, plugins, payloads, retries, indexing, context, and inference spend;
- security-critical state migrations, replay, synchronization, and recovery fail closed on ambiguity;
- durable cryptographic records identify their algorithm/profile and key version explicitly;
- post-quantum protection is preferred for Praxis-native asymmetric cryptography, with `pq-required`, `pq-preferred`, hybrid, and classical-compatible policy profiles;
- cryptographic verification never implies execution authorization or capability trust;
- downgrade from a required post-quantum or hybrid profile is prohibited.

## Delivery topology

### Wave 0: Contract foundation

**Outcome:** stable semantic and security boundaries exist before parallel implementation expands.

Governing specs initially include SPEC-001 through SPEC-004 and must be extended for ADR-040 through ADR-043.

Deliverables include stable IDs/versioning, command/query/event envelopes, graph/agent/slice/package/preference/profile contracts, provenance/trust-class contracts, invocation/client capability and enforcement contracts, scheduler/resource contracts, plugin principal/isolation contracts, capability leases, `ActionIntent`/approval binding, cryptographic profile and envelope contracts, key identifiers/version/lifecycle contracts, compatibility fixtures, and the architecture conformance harness.

**Exit gate:** all public/persisted boundary contracts validate deterministically; compatibility, authority, anti-replay, trust-class, crypto-profile, downgrade-resistance, and key-rotation tests pass.

### Wave 1: Authoritative kernel and persistence

**Outcome:** deterministic local source of truth independent of any LLM client.

Scope: SQLite authoritative state, append-only events, projections, replay/recovery, migrations, command processing, idempotency/correlation/causation, policy state, approval/lease state, crypto metadata/key references, and export primitives.

Key ADRs: 031, 032, 035, 036, 040, 042, 043.

**Exit gate:** restart/replay produces equivalent authoritative state; malformed/stale/replayed authority cannot create a valid mutation; encrypted/signed durable records retain sufficient profile/key metadata for verification and migration.

### Wave 2: Graph execution and resource kernel

**Outcome:** generic graphs execute without domain assumptions and cannot exhaust the runtime without deterministic limits.

Scope: graph validation, slices, node/block execution, dependencies, cancellation, retry/recovery, checkpoint/resume, resource acquisition/fairness, work/depth/retry quotas, and deterministic transitions.

Key ADRs: 002, 004, 022, 034.

**Exit gate:** fixture graphs execute/fail/cancel/recover/resume under contention and hostile resource-exhaustion cases remain bounded.

### Wave 3: Plugin, capability, and isolation runtime

**Outcome:** Praxis core stays small while plugins are useful without becoming ambient privileged processes.

Scope: Go core/plugin boundary, gRPC/protobuf transport, plugin discovery/lifecycle, authenticated instance identity, capability advertisement and leases, least-privilege isolation profiles, credential brokering, filesystem/network/process restrictions, quotas, supervision, revocation, failure containment, observability, and cryptographic-provider capability discovery.

Key ADRs: 016, 028, 029, 038, 041, 043.

**Exit gate:** a non-core-language plugin can execute a granted capability; attempts to use denied filesystem/network/process authority fail under an enforced profile; unsupported guarantees are reported rather than assumed; cryptographic provider capabilities are resolved deterministically rather than by the LLM.

### Wave 4: Workspace Intelligence

**Outcome:** graphs obtain precise workspace evidence with lower latency/token use without turning derived indexes into authority.

Governing spec: SPEC-008.

Scope: incremental deterministic indexing, lexical/path search, symbol/AST/reference/dependency lookup, VCS/change awareness, disposable derived indexes, context packs, optional semantic retrieval, task-scoped working graphs, token/byte budgets, provenance/trust classes, sensitivity labels, path/secret exclusions, symlink/traversal defenses, destination-aware context release, cache invalidation, and retrieval telemetry.

Key ADRs: 039, 040, 041, 042.

**Exit gate:** hostile-workspace corpus proves secret exclusions, prompt-injection provenance, stale-index detection, bounded context, deterministic lookup, and materially reduced model exploration/token use.

### Wave 5: Inference and executor routing

**Outcome:** graphs request capabilities/reasoning rather than model vendors.

Scope: executor abstraction, authenticated client/session reuse, Claude/Codex/Copilot/local adapters, reasoning tiers, capability evidence, routing/fallback, cost/latency/quality telemetry, trust-class-preserving context delivery, and failure escalation.

Key ADRs: 015, 016, 018, 038, 040.

**Exit gate:** the same graph executes through multiple eligible executors without semantic changes, and untrusted evidence cannot become authority through an executor adapter.

### Wave 6: Learning and adaptation

**Outcome:** Praxis learns useful behavior without learning around governance or security.

Scope: observations, evidence aggregation, candidate extraction, confidence, evaluation, promotion/demotion, stabilization, deterministic extraction, provenance ancestry, duplicate-source detection, and non-learnable security/policy boundaries.

Key ADRs: 006, 007, 012, 018, 019, 022, 023, 040.

**Exit gate:** controlled fixture demonstrates observation -> candidate -> evaluation -> governed promotion -> rollback while hostile/repeated evidence cannot weaken policy or fabricate independent corroboration.

### Wave 7: Persistent agent identity and memory

**Outcome:** agents persist across sessions, clients, generations, and restarts without reducing identity to prompt text.

Scope: identity, generations/lineage, graph binding, memory/retrieval, introspection, cross-agent transfer, provenance/trust classes, governed self-modification, poisoning-resistant promotion, and long-lived encrypted state policy.

Key ADRs: 003, 009, 010, 012, 017, 040, 043.

**Exit gate:** an agent continues across clients and graph versions while provenance survives, untrusted content cannot silently become authoritative memory, and sensitive long-lived state can be protected under a PQ-capable profile.

### Wave 8: Packaging, catalog, dependency trust, and signatures

**Outcome:** packages can be distributed and installed without treating provenance or signatures as safety.

Scope: package format, immutable digest/version, dependency lock/resolution, publisher provenance, algorithm-agile signatures, ML-DSA-preferred Praxis-native signing, optional SLH-DSA diversity, transitive capability aggregation, permissions, install/update/rollback, GitHub-backed initial catalog transport, local descendants/forks, compatibility, trust policy, signature/key rotation, revocation, and cryptographic-profile migration.

Key ADRs: 013, 020, 021, 025, 027, 041, 043.

**Exit gate:** install/verify/run/upgrade/rollback/fork succeeds; a validly signed malicious fixture receives no implicit authority; transitive capability expansion requires authorization; package signatures survive key rotation and algorithm migration according to policy.

### Wave 9: Personalization and behavioral matching

**Outcome:** packages start near user preferences and adapt without confusing preference with policy.

Scope: preference contracts, install seeding, scope inheritance, explicit/learned/default precedence, behavioral profiles, evidence classes, catalog matching, and policy/preference separation.

Key ADRs: 008, 014, 023, 026, 030, 040.

**Exit gate:** users can diverge safely while neither package content nor learned preferences can override deterministic policy.

### Wave 10: LLM client integration and enforcement surfaces

**Outcome:** Praxis feels native in LLM clients while client UX never becomes the authority layer.

Governing spec: SPEC-004.

Scope: `InvocationContract`, `/praxis <entry-point> [options]`, native skills/commands, MCP/tool surface, lifecycle hooks, minimal context projection, CLI bridge, client capability/enforcement discovery, generated adapter lifecycle, status/resume/cancel, dashboard presentation, constrained/unconstrained mode reporting, deterministic action mediation, and cryptographic capability/profile reporting where client-native secure channels or signing are relied upon.

Primary proving invocation:

```text
/praxis develop <options> --dashboard
```

**Exit gate:** equivalent invocations across clients create equivalent canonical commands; an unconstrained alternate client tool is detected/reported and packages requiring exclusive mediation fail closed; cryptographic requirements are not silently downgraded by client limitations.

### Wave 11: Portable and multi-machine state

**Outcome:** state can move/reconcile without making cloud mandatory, replaying stale authority, or weakening cryptographic protection.

Scope: export/import, synchronization envelopes, conflict semantics, device identity, encrypted/sensitive state, offline operation, portability classes for policy/trust/approvals/leases/learning/memory, PQ-preferred key establishment for Praxis-native synchronization paths, hybrid/PQ-required profiles, key rotation, and encrypted-backup migration.

Key ADRs: 011, 024, 040, 042, 043.

**Exit gate:** machines reconcile supported state deterministically; one-shot approvals/runtime leases are not imported as portable authority; policy/trust conflicts fail closed; `pq-required` synchronization/export cannot silently fall back to classical-only protection.

### Wave 12: Proving domain A - software development

**Outcome:** software development is a first-class Praxis package/graph suite and validates the platform under demanding real work.

Governing spec: SPEC-013 in addition to the generic runtime/Workspace Intelligence specs.

Expected surface:

```text
/praxis develop <options>
/praxis develop <options> --dashboard
```

Responsibilities include goal/repository discovery, progressive-rigor classification, reusable Planning Baselines, explicit variance registers, ADR and architecture-view workflows, sequence/security views where applicable, SPEC derivation, dependency-ordered master-plan/work-bundle derivation, baseline invalidation/delta planning, implementation slices, test/review/repair loops, Git/worktree/branch handling, CI/PR integration, evidence-driven completion, development-specific capabilities, and learned workflow preferences.

The package SHALL distinguish project-level uncertainty reduction from slice-level planning. Expensive discovery/architecture is performed once when justified, versioned into a baseline, and reused by subsequent slices. A slice first attempts deterministic baseline reuse and delta planning before invoking project-level planning again. Direct low-risk work remains able to bypass architected mode.

**Exit gate:** a multi-bundle repository change performs expensive project discovery/architecture once, produces a versioned baseline and conformance-linked master plan, executes at least two slices against it without repeated project planning, selectively replans invalidated portions after a controlled workspace change, and still supports a direct low-risk fast path. End-to-end execution must retain bounded context, persistent learning, client-neutral invocation, and deterministic side-effect enforcement.

### Wave 13: Proving domain B - non-development

**Outcome:** falsify accidental development coupling.

Choose a materially different domain such as structured research, operational planning, or document production.

**Exit gate:** the package runs without adding development terminology or semantics to Praxis core and can reuse generic evidence/provenance/security contracts.

### Wave 14: Praxis 1 migration and salvage

**Outcome:** retain useful implementation without inheriting obsolete architecture or bypasses.

Scope: salvage matrix, behavioral tests, adapters, state migration, removal of compatibility layers, and explicit rejection of legacy paths that bypass command, capability, provenance, enforcement, or cryptographic-profile boundaries.

Key ADR: 027.

**Exit gate:** retained components conform to Praxis 2 contracts or remain isolated behind removable compatibility boundaries.

### Wave 15: Productization, adversarial qualification, cryptographic migration, and release

**Outcome:** Praxis 2 is installable, diagnosable, documented, supportable, and security claims are demonstrated.

Scope: installer/update/uninstall, `praxis doctor`, package/graph/client/security docs, user/migration guides, release policy, performance baselines, recovery drills, threat-model review, hostile fixture corpus, fuzz/property tests for parsers/protocol/contracts, end-to-end security conformance, cryptographic-provider diagnostics, key rotation/revocation drills, algorithm/profile migration tests, downgrade-resistance testing, and planning-amortization benchmarks for the development proving package.

Required hostile fixtures include prompt injection in evidence, secret/symlink/path traversal, stale context versus changed workspace, malicious signed packages, signature/key substitution, cryptographic downgrade attempts, transitive dependency expansion, compromised plugins, forged plugin identity, approval replay/argument mutation, ambiguous side-effect retry, unconstrained client bypass, event/projection corruption, stale authority sync, resource exhaustion, learning attempts to weaken policy, and stale/contradictory planning baselines.

**Exit gate:** clean install -> client integration -> package install -> hostile and normal real runs -> restart/recovery -> key rotation/crypto-profile migration -> upgrade/rollback passes on supported platforms with claimed enforcement and cryptographic properties verified. Development qualification additionally demonstrates that reusable project planning materially reduces repeated planning/token/latency across multi-slice work without increasing conformance failures.

## Architecture conformance matrix

Every implementation bundle SHALL map ADR -> SPEC -> implementation surface -> executable evidence. Initial mandatory mappings include:

| Architecture decision | Specification surface | Required evidence |
|---|---|---|
| ADR-034 scheduling/concurrency | scheduler/resource SPEC | deadlock, fairness, cancellation, starvation, quota tests |
| ADR-035 command/query boundary | command/event SPEC | mutation-only-through-command, replay, query purity |
| ADR-036 schema evolution | canonical contracts SPEC | compatibility and migration fixtures |
| ADR-037 client adapters | SPEC-004 | equivalent normalized invocation across mechanisms |
| ADR-038 enforcement below LLM | SPEC-004 + security contracts | bypass/fail-closed client tests |
| ADR-039 Workspace Intelligence | SPEC-008 | retrieval quality, token/latency, staleness, exclusion tests |
| ADR-040 trust/instruction separation | canonical/inference/learning specs | prompt-injection and provenance propagation tests |
| ADR-041 plugin isolation | SPEC-007 | denied ambient authority and lease/revocation tests |
| ADR-042 approval binding | command/security SPEC | replay, TOCTOU, stale intent, idempotency tests |
| ADR-043 cryptographic agility/PQC | SPEC-005 + crypto/security/package/state specs | ML-KEM, ML-DSA, rotation, migration, hybrid semantics, downgrade-resistance tests |
| ADR-044 front-loaded uncertainty | SPEC-013 | baseline reuse, selective invalidation, planning-amortization, direct-fast-path tests |

## Work-bundle derivation

Every work item must identify governing SPEC sections and ADRs, concrete deliverables, dependencies, expected implementation surfaces, acceptance/security tests, migration impact, authority implications, and observable completion evidence.

For work covered by a Planning Baseline, bundles SHALL reference the baseline/version and applicable ADR/SPEC/architecture-view dependencies. Slice planners resolve local implementation variance and SHALL NOT regenerate project architecture unless baseline validity fails.

Parallel work begins only after shared contracts are stable. Do not parallel