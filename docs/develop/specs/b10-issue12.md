# Bundle b10-issue12

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/12 — "Define overlay contract and integrate develop as the first Praxis overlay"

This bundle implements ONLY issue #12. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — #9 and #10 are separate bundles running concurrently right now in different worktrees/branches.

## Dependency status

Depends on #4, #5, #6, #7, #8 — all merged to `main`. Read what they actually produced before designing anything:

- `#4` (runtime engine): `src/praxis_runtime/{graph,events,state,transitions,replay,migrations}.py`, `docs/runtime.md`.
- `#5` (executors): `src/praxis_executors/{interface,matching,policy,registry}.py`, `docs/executors.md`.
- `#6` (evidence/proof/gates): `src/praxis_evidence/{types,proof,graders,gates,aggregate}.py`, `docs/evidence.md`.
- `#7` (resource claims/leases): `src/praxis_runtime/resources/{claims,leases,observed,policy,scheduler}.py`, `docs/resources.md`.
- `#8` (policy/authority/budgets): `src/praxis_policy/{authority,budgets,failure_classification,gate,profiles,receipts}.py`, `schemas/v1/{authority-requirement,budget-requirement,policy-profile,policy-requirement}.schema.json`, `docs/policy.md`.

## Full issue body

Parent: #1

## Goal

Define the generic overlay mechanism and move current `develop` semantics onto Praxis without embedding software-development behavior in the Praxis core.

## Overlay boundary

Praxis owns: graph execution; state/events/checkpoints; executor matching; resource scheduling; evidence gates; policy/recovery; observability; evaluation/promotion.

`develop` owns: Git/GitHub semantics; issues and bundling; TDD; implementation/test/review personas and task types; filesystem/worktree conventions; PR creation/monitoring; branch cleanup; documentation review; merge auditing; software-specific dashboard labels and proof types.

## Deliverables

- versioned overlay manifest/schema
- overlay lifecycle and registration rules
- domain capability namespace rules
- domain resource-provider extension points
- domain evidence/grader extension points
- development overlay implementing current `develop` graph semantics
- compatibility adapter/migration path for the current AI Atoms `bundle/develop`
- documentation showing that `ai-atoms` remains the catalog/distribution surface while Praxis is developed in this repository

## Acceptance criteria

- Praxis core contains no GitHub issue, PR, TDD, branch, merge, or code-review assumptions.
- `develop` can express its existing graph and policies through the overlay contract.
- The overlay may request capabilities but may not name a required LLM/model vendor in graph semantics.
- Existing `develop` invocation can be preserved or transitioned with an explicit compatibility plan.
- A second trivial non-development overlay fixture demonstrates that the overlay interface is genuinely generic.

## Depends on

- #4 (merged, satisfied)
- #5 (merged, satisfied)
- #6 (merged, satisfied)
- #7 (merged, satisfied)
- #8 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary. This issue is special: it is explicitly ABOUT defining a `develop` overlay, so the *overlay implementation itself* legitimately references `develop`/PR/TDD concepts (that's its whole purpose) — but the Praxis *core* contracts (`praxis_runtime`, `praxis_contracts`, `praxis_evidence`, `praxis_executors`, `praxis_policy`) must remain completely free of them, per the acceptance criteria's first bullet. Keep the overlay's own code in a clearly separate namespace/package from core.
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- This is the largest and most architecturally significant remaining issue: it must reconcile the independent contributions of #4 (runtime), #5 (executors), #6 (evidence), #7 (resources), and #8 (policy) into one coherent overlay-extension mechanism. Read all five of their `docs/*.md` files before designing the overlay manifest/schema.
- Issue #13 (parity proof) depends on this and needs the development overlay you build here to actually run `develop`-shaped work through Praxis for comparison against the #3 baseline.
- Run the full existing test suite (not just new tests) before considering any task done — this run has repeatedly surfaced real cross-bundle contract mismatches only when the full suite ran against all merged work together (see #5+#6's evidence-shape mismatch, discovered only at integration time).
- Take the scope at face value; this is a substantial deliverable, not a stub. It is fine (and expected) for this to be the biggest bundle in the run so far.
