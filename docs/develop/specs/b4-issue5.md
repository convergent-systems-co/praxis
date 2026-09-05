# Bundle b4-issue5

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/5 — "Implement executor abstraction and capability/promise-based matching"

This bundle implements ONLY issue #5. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — those are separate bundles, some running concurrently right now (#6, #7 — same dependency tier as this one, different worktrees/branches).

## Dependency status

Depends on #2 and #4, both merged to `main`. Read what they actually produced before designing anything — do not guess at names:

- `#2` (contracts/ontology): `src/praxis_contracts/` (`validator.py`), `schemas/v1/{promise,requirement,capability,capability-advertisement,evidence-requirement,resource-claim}.schema.json`, `docs/ontology.md`. This defines the promise/capability vocabulary you match against.
- `#4` (runtime engine): `src/praxis_runtime/` (`graph.py`, `events.py`, `state.py`, `transitions.py`, `replay.py`, `migrations.py`, `testing/fake_executor.py`), `schemas/v1/{graph,event,run-state}.schema.json`, `docs/runtime.md`. This is the deterministic core you plug executors into. `testing/fake_executor.py` already exists — read it before building "a deterministic fake executor" (a listed deliverable below) to avoid duplicating it; extend/adapt it if it's a reasonable starting point, or explain in the PR why a new one was needed instead.

## Full issue body

Parent: #1

## Goal

Replace direct model/harness assumptions with a generic executor contract. Graph nodes request capabilities/promises; executors advertise capabilities/promises; Praxis deterministically matches them subject to policy.

## Deliverables

- executor interface: launch, status, cancel, capabilities, result
- capability advertisement format
- capability matching algorithm
- required/preferred/prohibited capability semantics
- executor health/availability signal
- cost/risk/latency metadata hooks without hardcoding provider-specific names into graph semantics
- deterministic fake executor
- first real adapter sufficient to run Praxis end to end
- explicit extension path for Claude, Codex, Copilot, OpenCode, MLX/local, and future executors

## Acceptance criteria

- No graph node names a model or vendor.
- Swapping two executors that fulfill the same promise set does not require graph edits.
- Matching failures return an explicit unsatisfied-promise explanation.
- Policy may restrict which advertised executors are eligible without changing the graph.
- Executor output is normalized to the Praxis result/evidence contract.
- Tests prove deterministic selection when candidates are equivalent and deterministic rejection when promises are unmet.

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

- Issue #8 (policy profiles/authority/budgets) depends on #2+#4+#5 and will build on your executor contract — keep its public interface clean and documented (`docs/` alongside `docs/ontology.md` and `docs/runtime.md`).
- Issue #9 (dashboard) and #12 (overlay contract) also depend on this. Take the scope at face value; this is a substantial foundational deliverable, not a stub.
- If you discover a real correctness/design gap in #2's or #4's already-merged code while building this, do NOT silently work around it or leave it broken — fix it if it's in a file already outside any other bundle's current in-flight footprint (check with the orchestrator's run-state if unsure), or report it clearly as a blocker with human_required if it requires editing a file another concurrent bundle (#6 or #7) might also be touching.
