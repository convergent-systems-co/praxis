# Bundle b11-issue11

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/11 — "Add bounded project-scoped learning with eval-gated promotion"

This bundle implements ONLY issue #11. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — #9 and #12 are separate bundles running concurrently right now in different worktrees/branches.

## Dependency status

Depends on #10, merged to `main`: `src/praxis_eval/{types,candidates,measurements,comparison,thresholds,gates,promotion,rollback,ledger}.py`. Read this package closely — issue #11 explicitly integrates with "#10 evaluation/promotion gates," so your candidate-heuristic promotion path should call into `src/praxis_eval/`'s existing promotion/gate machinery rather than building a parallel one.

## Full issue body

Parent: #1

## Goal

Allow Praxis to learn from execution telemetry without allowing observations to mutate active runtime policy directly.

## Principle

Learning produces hypotheses, not authority.

Observed corrections, recurrent failures, successful recovery patterns, and workflow efficiencies may produce candidate heuristics. Candidates remain scoped and inert until they pass evaluation and an applicable promotion policy.

## Deliverables

- observation/event extraction pipeline
- project-scoped candidate heuristic format
- confidence/evidence model
- contradiction/decay handling
- candidate clustering/deduplication
- project-to-global promotion proposal path
- integration with #10 evaluation/promotion gates
- explicit prohibition on directly injecting unvalidated learned rules into active graph/runtime behavior

## Acceptance criteria

- One observation cannot become an active global rule.
- Project-specific patterns remain project-scoped by default.
- Candidates carry provenance and evidence references.
- Contradictory evidence lowers confidence or blocks promotion.
- Promotion requires evaluation evidence through #10.
- Learned candidates cannot modify authority, policy floors, security invariants, or graph legality without explicit reviewed promotion.

## Depends on

- #10 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary (multiple prior bundles in this run were blocked and repaired for exactly this).
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issue #10's own `src/praxis_eval/` is likely still receiving unrelated concurrent changes from other in-flight bundles' cross-cutting integration work at the time you start (issues #9 and #12 both depend on #4-#8 broadly, though neither lists #10 as a dependency, so direct conflicts are unlikely — but verify before assuming).
- Run the full existing test suite (not just new tests) before considering any task done — this run has repeatedly surfaced real cross-bundle contract mismatches only when the full suite ran against all merged work together.
- Take the scope at face value; this is a substantial deliverable, not a stub.
