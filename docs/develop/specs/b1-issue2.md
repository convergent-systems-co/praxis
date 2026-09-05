# Bundle b1-issue2

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/2 — "Define Praxis contracts and Promise/Capability execution ontology"

This bundle implements ONLY issue #2. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 (#3-#13) — those are separate bundles, some blocked on this one.

## Repository context

Praxis is a brand-new, empty repository (only LICENSE/NOTICE/README exist). There is no existing build system, language, or tooling yet. This bundle is establishing foundational contracts, so it is reasonable for it to also choose/set up the base project scaffolding (language, package manifest, test runner) needed to express and validate these contracts, if no scaffolding exists yet. Keep the choice minimal and justified in the PR description.

## Full issue body

Parent: #1

## Goal

Define the machine-readable contracts and vocabulary that let a graph request "what it needs" (a promise/capability) without naming a model or vendor, and let an executor advertise "what it can do." This is the ontology every later Praxis component (runtime, scheduler, executor registry, evidence gates, policy) is built against.

## Core architectural rule (from the Epic)

> Graphs request promises/capabilities. They do not name models or vendors.

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review.
- Keep model/vendor names out of graph semantics.
- Prefer machine-readable contracts and deterministic validation over prose policy.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts (as applicable to contract validation).

## Deliverables (infer concretely from the Epic's target architecture and the issues that depend on #2 — #3, #4, #5, #6, #7, #8 — since this ontology must be sufficient for all of them)

- A versioned schema/vocabulary for: promises, capabilities, capability advertisement, and required/preferred/prohibited semantics (needed by #5).
- A vocabulary/shape for graph nodes to declare requirements without naming a model or vendor.
- A vocabulary/shape sufficient to describe evidence/proof requirements at a node (needed by #6) and resource claims (needed by #7) — at the contract/schema level only; do not implement the runtime engine, executor matching, evidence grading, or scheduler itself (those are #4, #5, #6, #7).
- Deterministic validation for the above schemas (malformed contract documents are rejected, not silently accepted).
- Documentation of the ontology: what a promise is, what a capability is, how matching is intended to work at the contract level (not the algorithm itself, which is #5).

## Acceptance criteria

- Contracts/schemas are machine-readable and versioned.
- A sample "graph requests a capability" document and a sample "executor advertises a capability" document both validate against the schemas.
- No schema or vocabulary name references a specific model or vendor.
- No schema conflates domain-specific (software-development) concepts into the core ontology.
- Validation rejects malformed/incompatible documents with a clear reason (fail closed).
- Tests cover valid contracts, malformed contracts, and version mismatches.

## Depends on

- None. This is the root issue for the epic's critical path.

## Notes for the tech lead

- This issue has no upstream dependency, so scope tightly to contracts/ontology/schema/validation. Do not build the graph engine, scheduler, or executor registry — those belong to #4, #5, #6, #7 respectively, which depend on this issue's output.
- Because #3 (baseline benchmark) is running in parallel in a sibling bundle and depends on #2 only "for final schema alignment," avoid gratuitous churn to schema field names once you land something reasonable — #3 needs to align its fixture format to whatever you produce here, but its initial workload capture does not block on this.
