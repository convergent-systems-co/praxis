# SPEC-013: Develop Planning Baseline and Progressive Rigor

- Status: Draft
- Governing ADRs: 033, 039, 040, 044
- Depends on: SPEC-008, SPEC-010

## Purpose

Make expensive discovery, architecture, and planning reusable across software-development slices while preserving a fast path for small work.

## Core model

The `develop` package distinguishes:

- **Project phase**: reduce durable uncertainty and compile it into reusable artifacts.
- **Slice phase**: execute a bounded work item against the current project baseline, resolving only local/delta uncertainty.

## Rigor profiles

### Direct

Use when work is narrow, low-risk, reversible, acceptance is objective, and no material architectural uncertainty exists.

Expected path:

`discover -> classify -> prepare -> implement -> validate -> review/integrate`

### Planned

Use when local design/decomposition is needed but durable architecture is not materially changing.

Expected path:

`discover -> classify -> delta/local plan -> prepare -> implement -> validate -> review/integrate`

### Architected

Use for material architecture, security boundaries, persistence/schema changes, public contracts, cross-cutting changes, novel domains, or large multi-bundle outcomes.

Expected project path:

`discover -> frame -> variance -> ADRs -> architecture views -> sequence/security views as applicable -> SPECs -> master plan -> conformance map -> implementation bundles`

Implementation bundles then use slice execution against that baseline.

User preference can raise/lower rigor; deterministic policy may set a minimum.

## PlanningBaseline

A baseline SHALL contain:

- baseline ID/version/digest;
- goal/project identity;
- workspace snapshot/fingerprint;
- discovery evidence references;
- requirements/constraints;
- variance register;
- ADR references and statuses;
- architecture-view references/digests;
- SPEC references/digests;
- master-plan reference/digest;
- conformance mappings;
- assumptions and validity predicates;
- created/updated timestamps and provenance.

Baseline content is derived knowledge, not implicit permission or policy authority.

## Artifact dependency graph

Praxis SHALL record dependency relationships between planning artifacts. At minimum:

`goal/requirements -> decisions/ADRs -> architecture views -> SPECs -> master-plan bundles -> implementation slices -> executable evidence`

Dependencies MAY be many-to-many. Invalidation propagates only through dependent edges.

## Variance register

Each item SHALL have ID, statement, category, status, alternatives, recommendation, rationale/evidence, reversibility/materiality, affected artifacts, and optional human-decision requirement.

Statuses SHALL distinguish unresolved, provisionally-assumed, resolved, superseded, and invalidated.

The planner resolves clear recommendations autonomously. Human interruption is reserved for material difficult-to-reverse forks without a dominant recommendation.

## Baseline applicability

Before project-level planning for a slice, the develop graph SHALL deterministically evaluate:

- baseline exists;
- goal/project identity matches;
- relevant ADR/SPEC/plan references still exist and are compatible;
- workspace evidence required by validity predicates is fresh;
- no changed evidence invalidates an applicable assumption;
- requested slice is covered by an existing or derivable plan bundle.

Result is one of:

- `reuse`: no project replanning;
- `delta`: refresh only affected artifacts/bundles;
- `replan`: material baseline invalidation;
- `architect`: no adequate baseline and architected rigor required.

## Delta planning

Delta planning SHALL receive changed evidence plus the smallest dependency closure of affected planning artifacts. It SHALL NOT regenerate unrelated ADRs/SPECs/plan bundles.

When a baseline is updated, its version/digest changes and affected slice references are revalidated.

## Slice contract

Every implementation slice derived from a baseline SHALL reference:

- baseline ID/version/digest;
- governing ADRs/SPEC sections;
- relevant architecture views;
- dependencies/predecessor evidence;
- exact scope/non-goals;
- required capabilities/authority;
- acceptance/security tests;
- Workspace Intelligence context policy/budget;
- unresolved local variance;
- completion evidence requirements.

A slice planner MAY refine local implementation details but SHALL NOT silently contradict governing baseline contracts. Contradiction produces a variance/invalidation signal.

## Planning amortization telemetry

Record:

- baseline build time/tokens/cost;
- baseline reuse count;
- delta-planning count/cost;
- full-replan count/reason;
- per-slice planning time/tokens;
- time to first useful implementation action;
- conformance violations/drift;
- invalidation causes;
- accepted implementation outcomes.

Derived metrics SHOULD include planning cost per completed slice and repeated planning avoided.

## Acceptance tests

1. direct low-risk fixture bypasses project architecture planning;
2. architected fixture produces baseline before implementation bundles;
3. second slice reuses baseline without repeating project planning;
4. implementation-only source change refreshes evidence without regenerating unrelated ADRs;
5. SPEC change invalidates dependent bundles but not unrelated architecture decisions;
6. architectural assumption change invalidates dependent SPEC/plan closure;
7. slice cannot silently contradict governing SPEC;
8. unresolved reversible local choice does not unnecessarily interrupt user;
9. material non-dominant architectural fork suspends for human decision;
10. baseline references preserve provenance and workspace freshness;
11. telemetry demonstrates amortized planning across multi-slice fixture;
12. stale baseline is detected rather than trusted because it exists.

## Exit criteria

A multi-bundle repository change can perform expensive project discovery/architecture once, derive implementation slices, execute at least two slices using the same baseline, and selectively replan only invalidated portions when the workspace changes.
