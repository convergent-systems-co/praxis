# Bundle b6-issue7

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/7 — "Generalize scheduling from file footprints to resource claims and leases"

This bundle implements ONLY issue #7. Do not implement Epic #1 (it is the controlling/tracking issue and must not be closed or implemented). Do not pull in work from any other child issue of Epic #1 — those are separate bundles, some running concurrently right now (#5, #6 — same dependency tier as this one, different worktrees/branches).

## Dependency status

Depends on #2 and #4, both merged to `main`. Read what they actually produced before designing anything — do not guess at names:

- `#2` (contracts/ontology): `src/praxis_contracts/` (`validator.py`), `schemas/v1/{promise,requirement,capability,capability-advertisement,evidence-requirement,resource-claim}.schema.json`, `docs/ontology.md`. `schemas/v1/resource-claim.schema.json` already exists as the contract-level resource-claim vocabulary — this issue builds the actual scheduling/lease runtime around it; read it first and be consistent with it (extend, don't duplicate/conflict).
- `#4` (runtime engine): `src/praxis_runtime/` (`graph.py`, `events.py`, `state.py`, `transitions.py`, `replay.py`, `migrations.py`, `testing/fake_executor.py`), `schemas/v1/{graph,event,run-state}.schema.json`, `docs/runtime.md`. This is the deterministic core your scheduler/lease manager integrates with (claim conflicts should block/serialize transitions the same way evidence gates do).

## Full issue body

Parent: #1

## Goal

Generalize `develop`'s file-footprint scheduling concept into a domain-neutral resource-claim model that can safely schedule filesystem, infrastructure, repository, database, network, and future resource types.

## Deliverables

- resource-claim schema with resource type, identifier, access mode, scope, and optional lease semantics
- deterministic conflict detection
- static claim planning
- observed/touched-resource recording
- lease acquire/renew/release contract with owner, heartbeat, and epoch/generation
- policy for undeclared resource access
- support for parking/retrying work when a newly requested resource conflicts
- domain adapter for filesystem claims sufficient to express current `develop` footprints

## Acceptance criteria

- Two compatible read claims can run concurrently.
- Conflicting write/mutate claims serialize deterministically.
- A task that requests an undeclared resource cannot silently mutate it.
- Strict policy may treat undeclared access as a planning defect; another policy may allow deterministic dynamic acquisition when safe.
- Lease expiry/recovery is bounded and does not allow stale owners to mutate after losing ownership.
- Final mutation/commit revalidates current resource ownership.
- Tests cover overlap, stale leases, heartbeat loss, epoch mismatch, dynamic acquisition, and workspace-wide fallback claims.

## Depends on

- #2 (merged, satisfied)
- #4 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review — this applies to docstrings/comments/test names too, not just schema vocabulary (a prior bundle in this run was blocked and repaired for exactly this). Note: the issue text itself says the domain adapter should be "sufficient to express current `develop` footprints" — that's about filesystem-claim *capability*, not about naming `develop`/GitHub/PR concepts inside the generic core; keep the adapter's vocabulary generic (files/paths/access-modes), and if you need a concrete example, describe it generically (e.g. "a filesystem write claim on a set of paths"), not in terms of PRs or branches.
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- Issue #9 (dashboard) and #12 (overlay contract) both depend on this. Keep the resource-claim/lease public interface clean and documented.
- If you discover a real correctness/design gap in #2's or #4's already-merged code while building this, do NOT silently work around it or leave it broken — fix it if it's in a file not already in another concurrent bundle's in-flight footprint, or report it clearly as a blocker with human_required if it is.
