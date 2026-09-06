# Bundle b9-issue10

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/10 — "Implement candidate configuration evaluation, promotion, and rollback"

This bundle implements ONLY issue #10. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — #9 and #12 are separate bundles running concurrently right now in different worktrees/branches.

## Dependency status

Depends on #3, #4, #5, #6 — all merged to `main`. Read what they actually produced before designing anything:

- `#3` (baseline benchmark): `benchmark/` — `corpus/` (8 scenario definitions), `metrics/metrics-spec.md`, `report-format/real-run-report-format.md`, `runs/` (runbook + one captured run), `baseline/{baseline-report.md,acceptance-thresholds.md}`, `fixtures/README.md`. This is the "baseline" this issue's eval framework compares candidates against — read `benchmark/baseline/` and `benchmark/metrics/metrics-spec.md` closely.
- `#4` (runtime engine): `src/praxis_runtime/{graph,events,state,transitions,replay,migrations}.py`, `docs/runtime.md`.
- `#5` (executors): `src/praxis_executors/{interface,matching,policy,registry}.py`, `docs/executors.md`.
- `#6` (evidence/proof/gates): `src/praxis_evidence/{types,proof,graders,gates,aggregate}.py`, `docs/evidence.md`.

## Full issue body

Parent: #1

## Goal

Make Praxis evolution eval-driven. Runtime, routing, prompt, policy, scheduler, or configuration changes should be evaluated against an accepted baseline before promotion.

## Deliverables

- candidate configuration registry
- immutable candidate identity/versioning
- baseline pointer
- benchmark/eval runner integration
- paired comparison of candidate vs baseline
- configurable promotion thresholds
- health/regression gates
- human approval gate for promotion where required
- append-only promotion/rollback evidence
- rollback to previous accepted configuration on failed health checks

## Acceptance criteria

- A candidate cannot become active without recorded evaluation evidence.
- Candidate and baseline are evaluated against the same workload/seed definitions where applicable.
- Promotion is reproducible from stored measurements and policy.
- Failed health/regression checks leave or restore the previous active configuration.
- Runtime changes can be compared on reliability, latency, retries, human interrupts, and cost where available.
- No self-learned heuristic can silently modify active behavior through this mechanism.

## Depends on

- #3 (merged, satisfied)
- #4 (merged, satisfied)
- #5 (merged, satisfied)
- #6 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary (multiple prior bundles in this run were blocked and repaired for exactly this).
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issue #11 (bounded learning) depends on this and explicitly integrates with "#10 evaluation/promotion gates" — keep the promotion-gate public interface clean and documented, since #11 will call into it.
- Issue #13 (parity proof) also depends on this.
- Run the full existing test suite (not just new tests) before considering any task done — several prior bundles in this run only surfaced real cross-bundle issues when the full suite ran against all merged work together.
- Take the scope at face value; this is a substantial deliverable, not a stub.
