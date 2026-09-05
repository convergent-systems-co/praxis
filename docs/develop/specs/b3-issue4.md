# Bundle b3-issue4

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/4 — "Implement generic graph, run-state, event, checkpoint, and transition engine"

This bundle implements ONLY issue #4. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — those are separate bundles.

## Dependency status

Issue #4 depends on #2 only. #2 (Praxis contracts and Promise/Capability execution ontology) is merged to `main` as of this bundle's base commit — its schemas/vocabulary live under whatever path #2 established (check `docs/`, `schemas/`, and the project root for what #2's PR actually added; do not guess field names from the issue text alone, read what #2 actually produced). Build the runtime engine so it is consistent with #2's contracts/vocabulary rather than inventing a competing one.

## Full issue body

Parent: #1

## Goal

Extract and implement the deterministic execution core that owns graph legality and durable progress independently of any model, harness, or domain overlay.

## Deliverables

- graph loader and validator
- persistent run-state store
- append-only event log
- cursor/checkpoint model
- deterministic transition evaluation
- fan-out/join primitives
- terminal, blocked, handoff, and recovery states
- resume/replay support
- schema-version migration strategy
- deterministic fake-executor test harness

## Required properties

- Conversation history is never the source of progress.
- A restarted process resumes from persisted run state and events.
- Invalid transitions fail closed.
- Missing required evidence blocks transitions.
- Event vocabulary is versioned and validated.
- State writes are atomic enough to survive concurrent schedulers/executors without corruption.
- Domain overlays cannot bypass core transition legality.

## Acceptance criteria

- A non-development sample graph can run end to end with deterministic fake executors.
- A crash/restart at every transition boundary resumes correctly.
- Event replay reconstructs the same observable run state.
- Tests cover malformed graphs, stale state, duplicate events, illegal transitions, and interrupted writes.

## Depends on

- #2 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review.
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issues #5, #6, #7 all depend on #4 (plus #2) and will be built against whatever runtime/module structure you establish here — keep the graph/run-state/event/checkpoint/transition engine's public interfaces clean and documented, since three sibling bundles will import from it soon.
- This is a substantial, foundational deliverable. Take the scope at face value; do not shrink it to a stub.
