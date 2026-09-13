# PLAN-001: Praxis 2 Master Delivery Plan

- Status: COMPLETE
- Branch: `redesign/praxis2`
- Architecture authority: `docs/ADR/*`
- Implementation authority: `docs/SPEC/*`
- Review record: `docs/PLAN/002-praxis2-adversarial-and-security-review.md`
- Final conformance: `docs/PLAN/003-praxis2-final-conformance-report.md`
- Reconciled: 2026-09-13
- Validated implementation/conformance head: `05c048d2dc07889e38642b2954850d96d719337d`

## Purpose

Deliver Praxis 2 as a domain-neutral platform for persistent, self-improving agents and graphs with deterministic authority below the LLM.

Praxis is not a software-development loop. Software development is a proving domain. Research is a second proving domain. Goals is the reusable human-to-outcome front end that compiles expensive uncertainty into reusable baselines.

## Delivery laws

1. ADRs define durable architectural decisions and rationale.
2. SPECs define executable contracts, invariants, failure behavior, security properties, and acceptance criteria.
3. PLANs define dependency order, integration gates, qualification, and completion evidence.
4. Existing/legacy implementation is salvageable evidence, not architectural authority.
5. Cross-domain behavior belongs in core contracts; domain behavior belongs in packages/graphs.
6. Client skills, hooks, prompts, MCP descriptions, and generated instructions are adapters, not authority.
7. If an LLM can choose to ignore a control, the control is advisory rather than enforcement.
8. Untrusted content is evidence/data, not authority.
9. Every external side effect remains within deterministic authority through the final commit/dispatch boundary.
10. Cryptography is profile-driven and algorithm-agile; Praxis-native asymmetric protection prefers standardized post-quantum mechanisms and fails closed on required-profile mismatch.
11. Expensive discovery and durable uncertainty are compiled once into reusable Goal/Planning Baselines when justified; downstream slices perform delta planning rather than rediscovery.
12. Planning rigor is progressive. Direct work retains a fast path; material work escalates to Goals/architecture/specification/planning.
13. Praxis core owns a stable control plane. Domain/package CLI commands are dynamically materialized from installed `InvocationContract`s and are never hard-coded into core.
14. A Praxis Package is the universal distribution unit. Graphs, agent definitions, preferences, templates, and executable plugins are typed package contents; plugin is not synonymous with package.
15. Authoritative persistence is defined by semantic provider contracts. SQLite is the default/reference implementation, not the architecture.
16. Distribution transport is replaceable. GitHub Releases is the initial adapter and never becomes root of trust.

## Cross-cutting security invariants

- least privilege and deny by default;
- explicit principal identity and provenance;
- capability grants are scoped, revocable, and never implied by package/plugin presence;
- approval and capability use are bound to canonical action intent where required;
- one-shot authority is non-replayable;
- ambiguous external effects reconcile rather than blindly retry;
- plugin authority is bound to exact instance/runtime session and consumed at the authoritative dispatch boundary;
- provider limitations cannot weaken security semantics silently;
- derived indexes/projections/models cannot self-promote into authority;
- sensitive context release is destination-aware and policy-mediated;
- quotas bound graph work, nesting, retries, plugins, context, indexing, inference and payloads;
- migrations/replay/sync/recovery fail closed on security ambiguity;
- package signatures prove integrity/provenance, not safety/authorization;
- update capability/enforcement/crypto expansion requires renewed authorization;
- active package invocation aliases are atomically tied to immutable package generations;
- runtime/client alternative bypass paths are declared and packages requiring exclusive mediation fail closed;
- post-quantum/hybrid required profiles cannot silently downgrade;
- recommendation delegation changes interaction behavior only and never grants execution authority.

## Delivery topology and status

### Wave 0: Contract foundation — COMPLETE

Canonical/security contracts, principals/provenance, capability leases, `ActionIntent`, approval binding, crypto profiles, invocation contracts, plugin identities, scheduler/resource contracts, and compatibility tests are implemented.

### Wave 1: Authoritative kernel and persistence — COMPLETE

SQLite is the reference implementation behind semantic provider interfaces. Migrations, append-only events, optimistic aggregate versions, projections/checkpoints, replay/recovery, authority/effect state, secure encrypted blobs, package/invocation registry, read-only query path and provider conformance are implemented.

### Wave 2: Graph execution and resource kernel — COMPLETE

Graph validation, explicit terminal semantics, bounded loops/retries, typed failures, cancellation, durable waits, replay/resume, exact nested graph identity and scheduler/resource primitives are implemented and tested.

### Wave 3: Plugin capability/isolation runtime — COMPLETE

Provider discovery, authenticated instance identity, protocol negotiation, handshake validation, lifecycle/quarantine, supervisor/restart ceilings, isolation declarations, exact instance/session lease binding, authoritative persisted lease reload and atomic bounded-use consumption immediately before transport are implemented.

### Wave 4: Workspace Intelligence — COMPLETE

Root confinement, traversal/symlink rejection, bounded context packs, evidence freshness/provenance, destination-aware sensitive release and crypto-profile constraints are implemented.

### Wave 5: Inference and executor routing — COMPLETE

D0 deterministic / D1 bounded / D2 deep reasoning tiers, capability routing, latency/token/deadline budgets, escalation and fast-path classification are implemented.

### Wave 6: Learning and adaptation — COMPLETE

Candidate lifecycle, independent-evidence requirement, deterministic promotion, invariant non-learnability, rollback-oriented evaluation and evidence provenance are implemented.

### Wave 7: Persistent agent identity and memory — COMPLETE

Persistent agent generations/lineage, scoped memory, provenance/trust-preserving ranking and poisoning-resistant behavior are implemented. Downloadable definitions remain distinct from local agent identities.

### Wave 8: Packaging, catalog, dependency trust and signatures — COMPLETE

Implemented:

- universal immutable package manifests;
- typed graph/agent/plugin contents;
- dependency lock validation and transitive capability aggregation;
- install/update/uninstall lifecycle and rollback semantics;
- dynamic invocation registry tied to immutable package generations;
- graph-only, agent-only and mixed package fixtures;
- independent local agents from one installed definition;
- GitHub Releases adapter with `discover/info/install/update/uninstall/list`;
- universal `praxis-package` discovery topic;
- bounded release payloads;
- canonical manifest/artifact/signature release assets;
- exact manifest+artifact digest signature binding;
- local publisher trust;
- algorithm-agile verifier interface;
- Ed25519 classical verifier;
- PQ/hybrid fail-closed semantics when required verifier capability is unavailable;
- explicit-only PQ-preferred classical fallback;
- renewed review for capability/enforcement/crypto expansion.

### Wave 9: Personalization and behavioral matching — COMPLETE

Scoped preferences, explicit > learned > default precedence and behavioral matching without authority escalation are implemented.

### Wave 10: LLM client integration and enforcement surfaces — COMPLETE

Implemented:

- canonical parser/resolver and invocation contracts;
- client capability/enforcement profiles;
- enforcement-below-LLM semantics;
- dynamic package commands with no domain-package imports in core CLI;
- registry-derived help;
- package lifecycle commands;
- read-only status;
- authorized `cancel` and `resume` through deterministic run-control authority;
- `doctor` and `version`;
- CLI resume as authorized liveness transition only, never direct node execution.

Core control plane:

```text
praxis discover
praxis info
praxis install
praxis update
praxis uninstall
praxis list
praxis help
praxis status
praxis resume
praxis cancel
praxis doctor
praxis version
```

All domain/package entry points derive from installed active `InvocationContract`s.

### Wave 11: Portable and multi-machine state — COMPLETE AT CONTRACT/CLASSIFICATION LEVEL

Portable-state classification and nonportable runtime authority classes are implemented. Release closure does not require a cloud sync service.

### Wave 12: Proving domain A — software development — COMPLETE

`develop` implements direct fast path, bounded planning, Goals architected path, Goal/Planning Baseline binding, software materialization, validation/repair, amortization telemetry and end-to-end subgraph execution.

### Wave 13: Proving domain B — structured research — COMPLETE

Research uses the same Goals/subgraph/runtime semantics without adding development ontology to core.

### Wave 14: Praxis 1 migration/salvage — COMPLETE FOR REDESIGN BRANCH BOUNDARY

Praxis 2 is architecturally independent of legacy paths and reuses legacy material only when it conforms to current contracts.

### Wave 15: Productization, adversarial qualification and release — COMPLETE

Closed:

1. provider-neutral persistence interfaces and conformance tests;
2. universal package graph/agent/mixed fixtures;
3. dynamic CLI lifecycle/help/status/resume/cancel control surface;
4. README/license/contribution-rights alignment;
5. final adversarial/security review in PLAN-002;
6. final conformance mapping in PLAN-003;
7. package distribution payload limits and cryptographic signature verification added during final hostile-path review;
8. full `Praxis 2 Go` workflow green at validated head `05c048d2dc07889e38642b2954850d96d719337d`.

## Architecture conformance matrix

| Architecture decision | Specification | Executable evidence |
|---|---|---|
| ADR-034 scheduling/concurrency | SPEC-003/010 | quota/fairness/cancel/nesting tests |
| ADR-035 command/query boundary | SPEC-002/006 | event/replay/query-purity tests |
| ADR-038 enforcement below LLM | SPEC-004 | preflight/client enforcement tests |
| ADR-039 Workspace Intelligence | SPEC-008 | confinement/context/freshness tests |
| ADR-040 trust/instruction separation | SPEC-001/008/009/012 | provenance/prompt-injection boundaries |
| ADR-041 plugin isolation | SPEC-007 | isolation/handshake/supervisor tests |
| ADR-042 approval binding | SPEC-002/006 | stale/replay/intent mutation tests |
| ADR-043 cryptographic agility/PQC | SPEC-005 | profile resolution/envelope/signature/downgrade tests |
| ADR-044 planning amortization | SPEC-013 | baseline reuse/delta/direct-path tests |
| ADR-045 Goals | SPEC-014 | Goals baseline/recommendation/invalidation tests |
| ADR-046 dynamic command surface | SPEC-015 | registry/alias/core-reservation/help tests |
| ADR-047 state provider abstraction | SPEC-016 | provider conformance suite |
| ADR-048 universal packages | SPEC-011/017 | graph-only/agent-only/mixed package fixtures |

## Completion calculation

All architectural, implementation, qualification, security-review and final CI acceptance gates are closed.

**Praxis 2 master-plan completion: 100%.**

The completion-marker commit that changes this document is documentation-only; validated executable behavior is the green CI head recorded above.

## Definition of 100%

Satisfied on 2026-09-13 by green full-branch CI at validated head `05c048d2dc07889e38642b2954850d96d719337d`, with PLAN-002 and PLAN-003 providing final adversarial/security and conformance records.
