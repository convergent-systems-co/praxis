# Bundle b8-issue9

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/9 — "Build live Praxis dashboard and run observability surface"

This bundle implements ONLY issue #9. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — #10 and #12 are separate bundles running concurrently right now in different worktrees/branches.

## Dependency status

Depends on #4, #5, #6, #7 — all merged to `main`. Read what they actually produced before designing anything:

- `#4` (runtime engine): `src/praxis_runtime/{graph,events,state,transitions,replay,migrations}.py`, `src/praxis_runtime/testing/fake_executor.py`, `schemas/v1/{graph,event,run-state}.schema.json`, `docs/runtime.md`. This is the source of run/graph/cursor/event state the dashboard projects.
- `#5` (executors): `src/praxis_executors/{interface,matching,policy,registry}.py`, `src/praxis_executors/adapters/{fake,subprocess_executor}.py`, `docs/executors.md`.
- `#6` (evidence/proof/gates): `src/praxis_evidence/{types,proof,graders,gates,aggregate}.py`, `schemas/v1/proof-record.schema.json`, `docs/evidence.md`.
- `#7` (resource claims/leases): `src/praxis_runtime/resources/{claims,leases,observed,policy,scheduler}.py`, `src/praxis_runtime/resources/adapters/filesystem.py`, `schemas/v1/lease.schema.json`, `docs/resources.md`.

Note: issue #8 (policy/authority/budgets) also merged (`src/praxis_policy/`) but is not a listed dependency of #9 — you may reference it for cost/time/retry metrics if genuinely useful and already present, but it's not required.

## Full issue body

Parent: #1

## Goal

Provide a live, read-only operator surface over Praxis state/events/evidence so humans can monitor progress without the graph depending on the dashboard.

## Deliverables

- live browser dashboard
- current run summary
- graph/DAG view with active cursors/tokens
- node/workgroup state
- blockers and next critical action
- executor assignment/capability match visibility
- resource claims/lease visibility
- proof/evidence status
- stale proof/config warnings
- cost/time/retry metrics where available
- replay/snapshot mode for completed runs

## Architectural constraint

The dashboard is a pure projection over runtime state and events. It must not own routing state and the graph must never wait on it.

## Acceptance criteria

- Dashboard can attach to a live run and update without mutating execution state.
- Completed runs can be replayed from durable records after the process exits.
- The operator can answer: what is running, what is blocked, why, what runs next, what evidence is missing, and which executor/resources are involved.
- Dashboard remains functional with a deterministic fake-executor run.
- Tests prove the dashboard cannot create legal state transitions by itself.

## Depends on

- #4 (merged, satisfied)
- #5 (merged, satisfied)
- #6 (merged, satisfied)
- #7 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary (multiple prior bundles in this run were blocked and repaired for exactly this). Note: this project's OWN delivery process (the `/develop` skill building this repo) has its own dashboard (`~/.claude/skills/develop/runtime/dashboard.py`) — that is a reference for inspiration only, at the pattern level; do not name or reference `/develop`, bundles, tech-leads, or PRs anywhere in this issue's actual deliverable code/docs, since this dashboard is for the generic Praxis runtime, not for `/develop` itself.
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issue #13 (parity proof) depends on this (#9) among others.
- Multiple bundles landing concurrently in this run have hit real cross-bundle contract mismatches when their independently-built pieces first got combined (e.g. #5 and #6 had an evidence-shape mismatch that only surfaced at merge time, not during either bundle's own isolated development). Since this bundle reads from #4/#5/#6/#7's actual current interfaces (not stubs), that risk is lower here, but still: run the full test suite including all pre-existing tests, not just your own new ones, before considering a task done.
- A "live browser dashboard" for a Python-based runtime most likely means a small local web server (e.g. built on the standard library, or a minimal dependency already reasonable for this project) serving a page that polls or streams run-state/events files. Use your judgment on the concrete implementation; the acceptance criteria are behavioral, not prescriptive about framework choice.
- Take the scope at face value; this is a substantial deliverable, not a stub.
