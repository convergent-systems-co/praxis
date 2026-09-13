# PLAN-001: Praxis 2 Master Delivery Plan

- Status: Draft
- Branch: `redesign/praxis2`
- Architecture authority: `docs/ADR/*`
- Implementation authority: `docs/SPEC/*`

## Purpose

Deliver Praxis 2 as a general platform for persistent, self-improving agents and graphs across domains.

Software development is the first major proving domain and a primary deliverable, but it does not define the core architecture.

This plan converts architecture into dependency-ordered delivery. Work items SHALL be derived from specifications, not directly from ADR prose.

## Delivery rules

1. ADRs define architectural decisions and rationale.
2. SPECs define executable contracts, invariants, failure behavior, and acceptance criteria.
3. PLANs define dependency order, integration gates, and work decomposition.
4. Issues/work bundles are generated from SPEC deliverables and acceptance tests.
5. Existing Praxis code is salvageable implementation, not architectural authority.
6. A work bundle is complete only when its governing spec acceptance criteria pass.
7. Cross-domain behavior must be expressed in core contracts; domain-specific behavior belongs in packages/graphs.
8. Client skills, hooks, MCP definitions, and instruction files are adapters. Praxis runtime state remains authoritative.

## Delivery topology

### Wave 0: Contract foundation

**Outcome:** stable semantic boundaries exist before parallel implementation expands.

Governing specs:

- SPEC-001 Canonical Contracts
- SPEC-002 Command, Query, and Event Boundary
- SPEC-003 Scheduler and Resource Governance
- SPEC-004 Client Integration and Invocation

Deliverables:

- authoritative versioned schema source;
- stable IDs/versioning rules;
- command/query/event envelopes;
- graph/agent/slice/package/preference/profile contracts;
- invocation/client capability contracts;
- scheduler/resource contracts;
- compatibility and migration fixtures;
- architecture conformance test harness.

**Exit gate:** all public/persisted boundary contracts validate deterministically and have compatibility tests.

### Wave 1: Authoritative kernel and persistence

**Outcome:** Praxis has a deterministic local source of truth independent of any LLM client.

Scope:

- SQLite authoritative state store;
- durable append-only event log;
- projections/read models;
- replay and recovery;
- schema migrations;
- command processing boundary;
- idempotency/correlation/causation handling;
- exportable state primitives.

Key ADRs: 031, 032, 035, 036.

**Exit gate:** a run can be reconstructed from durable state/events after process termination with equivalent projections.

### Wave 2: Graph execution kernel

**Outcome:** generic graphs execute correctly without assuming a software-development domain.

Scope:

- graph loading and validation;
- entry points;
- slices as externally meaningful work units;
- internal block/node execution;
- dependency resolution;
- cancellation;
- retry/recovery;
- checkpoint/resume;
- resource acquisition and fairness;
- deterministic transition/event emission.

Key ADRs: 002, 004, 022, 034.

**Exit gate:** generic fixture graphs execute, fail, cancel, recover, and resume correctly under resource contention.

### Wave 3: Plugin and capability runtime

**Outcome:** Praxis core remains small while capabilities can be added without language/domain lock-in.

Scope:

- Go core/plugin boundary;
- gRPC/protobuf transport;
- plugin discovery and lifecycle;
- capability advertisement;
- capability authorization;
- plugin isolation/failure handling;
- executor plugin model;
- observability around plugin calls.

Key ADRs: 016, 028, 029.

**Exit gate:** an out-of-process plugin in a non-core language can advertise and execute a canonical capability without redefining core contracts.

### Wave 4: Inference and executor routing

**Outcome:** graphs request capabilities/reasoning rather than hard-code model vendors.

Scope:

- executor abstraction;
- authenticated CLI/session reuse where supported;
- Claude/Codex/Copilot/local model adapters as implementations;
- reasoning tiers;
- capability evidence;
- routing and fallback;
- cost/latency/quality telemetry;
- failure escalation.

Key ADRs: 015, 016, 018.

**Exit gate:** the same graph can execute through multiple eligible inference/executor implementations without changing graph semantics.

### Wave 5: Learning and adaptation

**Outcome:** Praxis observes execution, learns candidate improvements, and promotes stable behavior under governance.

Scope:

- observations;
- evidence aggregation;
- evaluation records;
- heuristic/candidate extraction;
- confidence;
- promotion/demotion;
- stabilization;
- deterministic extraction where possible;
- separation of execution, learning, and governance.

Key ADRs: 006, 007, 012, 018, 019, 022, 023.

**Exit gate:** a controlled fixture demonstrates observation -> candidate -> evaluation -> governed promotion -> rollback/demotion without direct uncontrolled self-modification.

### Wave 6: Persistent agent identity and memory

**Outcome:** an agent persists across sessions, clients, graph generations, and runtime restarts without being reduced to prompt text.

Scope:

- persistent agent identity;
- generations and lineage;
- graph binding/version ancestry;
- memory records and retrieval;
- introspection;
- cross-agent knowledge transfer;
- provenance/confidence;
- governed self-modification.

Key ADRs: 003, 009, 010, 012, 017.

**Exit gate:** one agent continues coherently across two clients and a graph-version transition while preserving lineage and provenance.

### Wave 7: Packaging, catalog, and trust

**Outcome:** graphs/agents/process packages can be distributed and installed safely.

Scope:

- package format;
- immutable versions/digests;
- dependency resolution;
- publisher provenance;
- signatures/integrity;
- declared permissions/capabilities;
- installation/update/rollback;
- GitHub-backed catalog as initial transport;
- locally modified descendants/forks;
- package compatibility rules.

Key ADRs: 013, 020, 021, 025, 027.

**Exit gate:** install, verify, run, personalize, upgrade, rollback, and fork a package while preserving lineage and user state.

### Wave 8: Personalization and behavioral matching

**Outcome:** packages start close to user preferences and continue adapting without confusing defaults, learned behavior, and policy.

Scope:

- preference contracts;
- install-time seeding;
- scope inheritance;
- explicit/learned/default precedence;
- behavioral profiles;
- declared versus observed/measured evidence;
- catalog matching by goal, context, environment, and behavioral fit.

Key ADRs: 008, 014, 023, 026, 030.

**Exit gate:** two users can install the same package, seed different preferences, and evolve divergent local descendants without corrupting the upstream package.

### Wave 9: Client integration surfaces

**Outcome:** Praxis is naturally usable from LLM clients while remaining client-neutral.

Governing spec: SPEC-004.

Scope:

- canonical `InvocationContract`;
- `/praxis <entry-point> [options]` UX;
- native skill/slash-command materialization;
- structured tool/MCP surface;
- optional lifecycle hooks;
- minimal context/instruction projection;
- CLI compatibility surface;
- client capability discovery;
- deterministic install/update/uninstall of generated adapters;
- shared status/resume/cancel operations across clients;
- presentation capability model.

Primary proving invocation:

```text
/praxis develop <options> --dashboard
```

The equivalent structured tool invocation and CLI invocation MUST produce the same normalized Praxis invocation and run semantics.

**Exit gate:** start a run from one supported client, inspect/resume it from another, and attach the dashboard without client-specific graph behavior.

### Wave 10: Portable and multi-machine state

**Outcome:** Praxis identity/state can move or reconcile across machines without making a cloud service mandatory.

Scope:

- local-first portable state;
- export/import;
- synchronization envelopes;
- reconciliation/conflict semantics;
- machine/device identity where needed;
- encrypted/sensitive state handling;
- offline operation.

Key ADRs: 011, 024.

**Exit gate:** two machines reconcile supported shared state deterministically while preserving conflicts/provenance rather than silently overwriting them.

### Wave 11: Proving domain A - software development

**Outcome:** software development is delivered as a first-class Praxis package/graph suite and validates the architecture under demanding real work.

This wave SHALL have its own domain specification under `docs/SPEC/` and SHALL NOT redefine core contracts.

Expected user surface:

```text
/praxis develop <options>
/praxis develop <options> --dashboard
```

Likely package responsibilities include:

- goal/repository discovery;
- specification/planning workflows;
- implementation slices;
- test/review/repair loops;
- Git/worktree/branch handling;
- CI/PR integration;
- evidence-driven completion;
- development-specific resource/capability extensions;
- learned user/team workflow preferences.

**Exit gate:** substantial real repository work can be executed end-to-end through the development package with persistent learning and client-neutral invocation.

### Wave 12: Proving domain B - non-development

**Outcome:** prove Praxis is genuinely domain-neutral.

Select a domain whose ontology and workflow differ materially from software development. Candidate examples include structured research, operational planning, document production, or another repeatable multi-step knowledge workflow.

The purpose is architectural falsification: identify any core contract that accidentally encodes development assumptions.

**Exit gate:** a non-development package runs without adding domain terminology or semantics to Praxis core.

### Wave 13: Praxis 1 migration and salvage

**Outcome:** retain useful existing implementation without inheriting obsolete architecture.

Scope:

- code salvage matrix;
- adapt reusable runtime/executor/evidence/dashboard components;
- state migration where justified;
- explicit compatibility adapters;
- removal of temporary compatibility layers;
- replace tests that encode superseded architecture.

Key ADR: 027.

This work can occur opportunistically in earlier waves, but migration compatibility SHALL NOT block correct Praxis 2 contracts.

**Exit gate:** all retained legacy components conform to Praxis 2 contracts or are explicitly isolated behind removable compatibility boundaries.

### Wave 14: Productization and release qualification

**Outcome:** Praxis 2 is installable, diagnosable, documented, and supportable.

Scope:

- installer/update/uninstall;
- `praxis doctor`/diagnostics;
- package author documentation;
- graph author documentation;
- client adapter documentation;
- administrator/security documentation;
- user guide;
- migration guide;
- release/version policy;
- end-to-end qualification suite;
- performance/resource baselines;
- security review;
- failure/recovery drills.

**Exit gate:** clean install -> client integration -> package install -> real run -> restart/recovery -> upgrade/rollback passes on supported platforms.

## Cross-wave architecture conformance matrix

Every implementation bundle SHALL record the following mapping:

| Architecture decision | Implementation specification | Implementation surface | Required conformance evidence |
|---|---|---|---|
| ADR-034 scheduling/concurrency | SPEC-003 | scheduler/resource runtime | deadlock, fairness, cancellation, starvation, contention tests |
| ADR-035 command/query boundary | SPEC-002 | command handlers, event store, projections | mutation-only-through-command tests; replay/query purity |
| ADR-036 schema evolution | SPEC-001 | contracts/schema/migrations | compatibility and migration fixture suite |
| ADR-037 client adapters | SPEC-004 | adapter SDK, skills, MCP/tools, CLI | equivalent normalized invocation across client mechanisms |

As additional specs are created, this matrix SHALL expand so every Accepted or implementation-governing ADR maps to at least one executable spec and conformance test surface.

## Work-bundle derivation

A delivery issue/work bundle SHALL identify:

- governing SPEC section(s);
- governing ADR(s);
- concrete deliverable(s);
- dependencies;
- files/packages expected to change where known;
- acceptance tests to implement/pass;
- migration impact;
- security/authority implications;
- observable completion evidence.

An issue SHALL NOT be considered adequately specified when it contains only an ADR reference or architectural intent.

## Parallelism policy

Parallel work is encouraged only after shared contracts for that wave are stable enough to prevent incompatible local interpretations.

Safe parallelism generally follows interface boundaries:

- schema/bindings versus independent adapter implementations;
- state-store implementation versus read-only projections after event contracts stabilize;
- multiple executor/client adapters against a frozen adapter contract;
- independent domain packages against stable core contracts.

Do not parallelize competing definitions of the same authoritative contract.

## Master acceptance criteria

Praxis 2 is ready for release when all of the following hold:

1. Core runtime contains no required software-development ontology.
2. Authoritative state survives restarts and replays deterministically.
3. Generic graphs execute with governed resource concurrency and failure recovery.
4. Multiple executors/models can satisfy capability requests without graph rewrites.
5. Learning produces governed, reversible adaptations with provenance.
6. Persistent agent identity and memory survive client/session changes.
7. Packages install/update/rollback with integrity, provenance, and permissions enforced.
8. User preferences and behavioral matching work without conflating policy, defaults, and learned behavior.
9. The same package can be invoked through multiple LLM-client mechanisms using equivalent canonical semantics.
10. `/praxis develop <options> --dashboard` works as a complete client-facing proving example without making `develop` or `dashboard` core graph semantics.
11. State is portable/reconcilable according to the supported multi-machine model.
12. Both a development and a materially non-development proving package pass end-to-end qualification.
13. Every implementation-governing ADR is traceable to a SPEC and executable conformance evidence.
14. Installation, diagnostics, documentation, recovery, upgrade, and rollback are release-qualified.

## Immediate next specifications

After SPEC-001 through SPEC-004, create detailed implementation specs in dependency order for:

1. authoritative state/event store and projections;
2. graph execution/runtime;
3. plugin/capability runtime;
4. executor/inference routing;
5. learning/evaluation/promotion;
6. persistent identity/memory/lineage;
7. package/catalog/trust;
8. preferences/behavioral profiles;
9. portable state/reconciliation;
10. software-development proving package;
11. non-development proving package;
12. release/distribution/diagnostics.

These detailed specs become the source from which delivery issues are generated.
