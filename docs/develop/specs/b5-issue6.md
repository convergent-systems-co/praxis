# Bundle b5-issue6

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/6 — "Implement evidence, proof, and evaluation gate contracts"

This bundle implements ONLY issue #6. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — those are separate bundles, some running concurrently right now (#5, #7 — same dependency tier as this one, different worktrees/branches).

## Dependency status

Depends on #2 and #4, both merged to `main`. Read what they actually produced before designing anything — do not guess at names:

- `#2` (contracts/ontology): `src/praxis_contracts/` (`validator.py`), `schemas/v1/{promise,requirement,capability,capability-advertisement,evidence-requirement,resource-claim}.schema.json`, `docs/ontology.md`. `schemas/v1/evidence-requirement.schema.json` already exists — this issue builds the actual evidence/proof/grading runtime around that contract-level schema; read it first and be consistent with it (extend, don't duplicate/conflict).
- `#4` (runtime engine): `src/praxis_runtime/` (`graph.py`, `events.py`, `state.py`, `transitions.py`, `replay.py`, `migrations.py`, `testing/fake_executor.py`), `schemas/v1/{graph,event,run-state}.schema.json`, `docs/runtime.md`. This is where transitions gate on evidence — your evidence/proof/grader contracts need to plug into `transitions.py`'s legality checks.

## Full issue body

Parent: #1

## Goal

Make completion evidence a first-class runtime contract. Transitions should occur because required proof exists and validates, not because an executor reports that work feels done.

## Deliverables

- generic evidence schema
- proof/gate definition schema
- deterministic grader interface
- model-based grader interface as an optional capability, never as the sole authority where deterministic checks exist
- human-review gate type
- artifact references and provenance
- stale-proof semantics tied to graph/config/runtime revisions
- aggregate gate results for fan-in/join nodes

## Acceptance criteria

- A transition can require one or more named evidence classes.
- Missing, malformed, stale, or contradictory evidence blocks the transition.
- Deterministic graders are preferred where available.
- A proof record identifies the run, graph version, executor, inputs/artifacts, and result without storing secrets by default.
- Domain overlays can define specialized evidence types without changing core runtime code.
- Tests cover false success, missing evidence, stale evidence, and mixed deterministic/model/human gates.

## Depends on

- #2 (merged, satisfied)
- #4 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary (a prior bundle in this run was blocked and repaired for exactly this).
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issue #9 (dashboard), #10 (candidate eval/promotion), and #12 (overlay contract) all depend on this. Keep the evidence/proof/gate public interface clean and documented.
- Note issue #5 (executor abstraction) is being built concurrently in a sibling bundle right now — it "normalizes executor output to the Praxis result/evidence contract," which is this issue's contract. Some coordination risk exists if both bundles want to touch the same shared vocabulary; if you hit a real conflict, name it clearly as a blocker rather than guessing at #5's unmerged design.
- If you discover a real correctness/design gap in #2's or #4's already-merged code while building this, do NOT silently work around it or leave it broken — fix it if it's in a file not already in another concurrent bundle's in-flight footprint, or report it clearly as a blocker with human_required if it is.
