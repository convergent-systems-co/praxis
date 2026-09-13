# PLAN-001: Praxis 2 Master Delivery Plan

- Status: Active / Reconciled
- Branch: `redesign/praxis2`
- Architecture authority: `docs/ADR/*`
- Implementation authority: `docs/SPEC/*`
- Review record: `docs/PLAN/002-praxis2-adversarial-and-security-review.md`
- Reconciled: 2026-09-13

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

Outcome: stable canonical/security boundaries.

Implemented evidence includes canonical command/event/security contracts, principals/provenance/trust classes, capability leases, `ActionIntent`, approval binding, crypto profiles, invocation contracts, plugin identities, scheduler/resource contracts, and compatibility tests.

Exit gate: deterministic validation and negative security tests are green.

### Wave 1: Authoritative kernel and persistence — COMPLETE WITH PROVIDER ABSTRACTION CLOSURE

Outcome: deterministic durable source of truth independent of LLM/client session.

Implemented: SQLite reference provider, migration ledger, append-only events, optimistic aggregate versions, projections/checkpoints, replay/recovery, authority/effect state, secure encrypted blobs, package/invocation registry schema, read-only query path.

Governing: SPEC-006, SPEC-016; ADR-031/032/035/036/040/042/043/047.

Closure requirement: provider-neutral semantic interfaces/conformance suite must front SQLite so runtime/domain code need not know schema details.

### Wave 2: Graph execution and resource kernel — COMPLETE

Implemented: graph validation, explicit terminal semantics, generic runner, bounded loops/retries, typed failures, cancellation, durable waits, replay/resume, nested graph scope/authority/quota narrowing, exact subgraph identity/version references, scheduler/fairness/quota primitives.

Exit gate: fast path, architected subgraph path, retry/replay/cancel/wait fixtures pass.

### Wave 3: Plugin capability/isolation runtime — COMPLETE

Implemented: provider discovery, authenticated instance identity, protocol negotiation, handshake validation, lifecycle/quarantine, supervisor/restart ceilings, isolation enforcement declarations, instance/session-bound capability leases, authoritative atomic lease consumption immediately before transport dispatch.

Exit gate: stale-session, denied-operation, one-use replay, missing-isolation, quarantine and protocol fixtures pass.

### Wave 4: Workspace Intelligence — COMPLETE

Implemented: workspace root confinement, symlink/traversal rejection, bounded context packs, lexical/path evidence, freshness/provenance, destination-aware sensitive release and PQ profile constraints.

Exit gate: confinement, secret/sensitive release, budget and stale-evidence fixtures pass.

### Wave 5: Inference and executor routing — COMPLETE

Implemented: D0 deterministic / D1 bounded / D2 deep reasoning tiers, capability-based routing, latency/token/deadline budgets, escalation semantics, fast-path development classification.

Design principle: use the lowest reasoning level that reliably solves the problem.

### Wave 6: Learning and adaptation — COMPLETE

Implemented: candidate lifecycle, independent-evidence requirement, deterministic promotion gate, policy/invariant non-learnability, rollback-oriented evaluation, evidence provenance.

### Wave 7: Persistent agent identity and memory — COMPLETE

Implemented: persistent agent generations/lineage, scoped memory, provenance/trust-preserving ranking, poisoning-resistant trust behavior, local identity separate from downloadable definition.

Universal package closure is governed by SPEC-017.

### Wave 8: Packaging, catalog, dependency trust and signatures — FUNCTIONALLY COMPLETE; FINAL PACKAGE-CONTENT FIXTURES REMAIN

Implemented: immutable manifest identity, dependency lock validation, transitive capability aggregation, install lifecycle/update review, rollback semantics, lineage, crypto profiles, dynamic invocation registry, GitHub Releases distribution adapter.

New governing ADRs/SPECs: ADR-046/048, SPEC-011/015/017.

Required final fixtures:

- graph-only package install/activate/update/rollback/uninstall;
- agent-definition-only package with two independent local identities;
- mixed graph+plugin package proving plugin authority remains lease-gated;
- catalog discovery filtering by content class.

### Wave 9: Personalization and behavioral matching — COMPLETE

Implemented: scoped preferences, explicit > learned > default precedence, behavioral profile matching that ranks eligible packages without granting authority.

### Wave 10: LLM client integration and enforcement surfaces — FUNCTIONALLY COMPLETE; CLI LIFECYCLE/CONTROL CLOSURE REMAINS

Implemented: canonical parser/resolver, invocation contracts, client capability/enforcement profiles, enforcement-below-LLM semantics, dynamic package command registry, status read path, durable run control service.

Core CLI reserved surface:

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

All domain/package entry points derive from active installed `InvocationContract`s. Core CLI may not import domain packages to expose commands.

Remaining closure: finalize CLI help/list/completion and mutation command wiring through deterministic authority.

### Wave 11: Portable and multi-machine state — COMPLETE AT CONTRACT/CLASSIFICATION LEVEL

Implemented: portable state classification, nonportable runtime authority (approvals/leases/client sessions/resource locks), sensitive encrypted state path, canonical-state principle.

Release closure does not require a cloud sync service; provider-neutral export/import remains the canonical future extension point.

### Wave 12: Proving domain A — software development — COMPLETE

Implemented `develop` package with:

- direct fast path;
- bounded local planning;
- architected path through Goals;
- Goal/Planning Baseline binding;
- software materialization contract;
- validation/repair loop;
- planning-amortization telemetry;
- end-to-end Goals subgraph execution.

Exit evidence: architected work enters Goals/materialization/local planning; direct work skips it; baseline reuse reduces project-level replanning.

### Wave 13: Proving domain B — structured research — COMPLETE

Implemented research package using the same Goals/subgraph/runtime semantics without development ontology.

Exit evidence: research can build or reuse a Goal Baseline, gather/synthesize/challenge evidence, and complete without adding software-development concepts to core.

### Wave 14: Praxis 1 migration/salvage — COMPLETE FOR REDESIGN BRANCH BOUNDARY

Praxis 2 architecture is independent of legacy architecture. Existing implementation is reused only where it conforms to the new contracts. Legacy paths that bypass deterministic command/capability/provenance/enforcement boundaries are not architectural dependencies.

A future production migration may preserve additional legacy state, but it is not a prerequisite for the Praxis 2 redesign branch to satisfy its master plan.

### Wave 15: Productization, adversarial qualification and release — IN CLOSURE

Implemented/available: CI, Go module lock, restart/replay tests, SQLite migrations, secure blobs, plugin authority tests, Goals/develop/research proving packages, qualification matrix, GitHub Releases adapter, dynamic CLI registry, PolyForm Noncommercial licensing.

Remaining release gates:

1. provider-neutral persistence conformance implementation/tests (SPEC-016);
2. universal package graph/agent/mixed fixture implementation/tests (SPEC-017);
3. finish CLI `list/help/status/resume/cancel` against durable registry/run control without authority bypass;
4. align README/user docs with Praxis 2 architecture and PolyForm Noncommercial license;
5. run final adversarial/security review against current implementation and update PLAN-002;
6. run full branch CI green after all closure changes;
7. produce final master-plan conformance report with no unexplained open acceptance gates.

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
| ADR-043 cryptographic agility/PQC | SPEC-005 | profile resolution/envelope/downgrade tests |
| ADR-044 planning amortization | SPEC-013 | baseline reuse/delta/direct-path tests |
| ADR-045 Goals | SPEC-014 | Goals baseline/recommendation/invalidation tests |
| ADR-046 dynamic command surface | SPEC-015 | registry/alias/core-reservation tests |
| ADR-047 state provider abstraction | SPEC-016 | provider conformance suite |
| ADR-048 universal packages | SPEC-011/017 | graph-only/agent-only/mixed package fixtures |

## Completion calculation

Completion is weighted by accepted architecture/runtime capability, not lines of code. Documentation without executable evidence does not close an implementation gate. A subsystem is complete when its governing invariants have implementation plus positive/negative tests.

Current reconciliation identifies only Wave 15 closure gates and the explicit closure items in Waves 1, 8 and 10. All other waves have implementation evidence sufficient for the redesign-branch master plan.

## Work-bundle rules

Every remaining bundle must identify:

- governing ADR/SPEC;
- exact implementation surface;
- authority/security implications;
- positive and negative tests;
- migration/provider/package compatibility impact;
- observable completion evidence.

No new domain feature is added during closure unless it fixes a violated invariant or an explicit acceptance gate.

## Definition of 100%

Praxis 2 master-plan completion is 100% when:

- all seven Wave 15 release gates above are closed;
- full branch CI is green at the reconciled head;
- PLAN-002 reflects the final adversarial/security review;
- PLAN-001 has no unqualified implementation acceptance gate marked incomplete;
- the final conformance report maps every current ADR/SPEC addition through implementation and test evidence.
