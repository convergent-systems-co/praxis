# Bundle b7-issue8

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/8 — "Implement policy profiles, authority boundaries, budgets, and bounded recovery"

This bundle implements ONLY issue #8. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — #6 and #7 are separate bundles running concurrently right now in different worktrees/branches; do not touch their in-flight work.

## Dependency status

Depends on #2, #4, #5 — all merged to `main`. Read what they actually produced before designing anything — do not guess at names:

- `#2` (contracts/ontology): `src/praxis_contracts/validator.py`, `schemas/v1/{promise,requirement,capability,capability-advertisement,evidence-requirement,resource-claim}.schema.json`, `docs/ontology.md`.
- `#4` (runtime engine): `src/praxis_runtime/{graph,events,state,transitions,replay,migrations}.py`, `src/praxis_runtime/testing/fake_executor.py`, `schemas/v1/{graph,event,run-state}.schema.json`, `docs/runtime.md`. `transitions.py` is where transition legality is evaluated — your policy/authority/retry-budget contracts need to gate here, alongside evidence gates (issue #6, running concurrently, not yet merged — do not depend on its unmerged output; if you need an evidence-gate hook point that doesn't exist yet, design your own minimal integration seam and note it for #12's overlay-contract work to reconcile later).
- `#5` (executor abstraction): `src/praxis_executors/{interface,matching,policy,registry}.py`, `src/praxis_executors/adapters/{fake,subprocess_executor}.py`, `docs/executors.md`. Note `src/praxis_executors/policy.py` already exists — read it first; this issue's "policy profiles" may need to extend or integrate with it rather than create a second, conflicting policy module. If its scope is narrower than what #8 needs (e.g. only executor-eligibility policy, not node-level minimum policy / authority boundaries / budgets), that's fine — build #8's broader policy-profile system alongside it, and be explicit in the PR about how the two relate.

## Full issue body

Parent: #1

## Goal

Move safety and autonomy policy into explicit machine-readable runtime contracts instead of ambient prompt behavior.

## Deliverables

- policy-profile schema, initially supporting profiles such as `fast`, `standard`, `strict`, and `regulated`
- node-level minimum policy requirements
- human authority boundary definitions
- retry and repair budgets
- transient vs substantive failure classification
- budget controls for cost/time/retries
- deterministic escalation rules
- alternate-executor retry semantics when policy allows
- explicit fail-closed behavior when policy/authority requirements cannot be satisfied

## Acceptance criteria

- A user may select a stricter profile but cannot lower a node below its declared minimum.
- Destructive, credential, billing, production, legal/compliance, and other authority-gated actions can be represented without embedding domain-specific logic in the core.
- Retry loops are bounded.
- The runtime distinguishes `blocked`, `retryable`, `human_required`, and `failed` states deterministically.
- Policy decisions are recorded as auditable events/receipts.
- Tests cover exhausted retry budgets, authority denial, policy escalation, and alternate-executor fallback.

## Depends on

- #2 (merged, satisfied)
- #4 (merged, satisfied)
- #5 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary (multiple prior bundles in this run were blocked and repaired for exactly this).
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issue #12 (overlay contract) depends on #4+#5+#6+#7+#8 and will need to reconcile whatever policy/authority seams #6, #7, and #8 each introduce independently and concurrently — keep your public interface clean, documented, and narrowly scoped to this issue's own deliverables so that reconciliation is tractable later.
- If a doc-only task hits the "no footprint-legal RED test" issue (a documentation deliverable with no natural paired test file), the established precedent in this run is: skip the automated RED-test-first phase for that task, write the doc directly, and rely on tester/adversarial-tester manual accuracy review instead.
- If you discover a real correctness/design gap in #2's, #4's, or #5's already-merged code while building this, do NOT silently work around it or leave it broken — fix it if it's in a file not already in another concurrent bundle's (#6, #7) in-flight footprint, or report it clearly as a blocker with human_required if it is.
