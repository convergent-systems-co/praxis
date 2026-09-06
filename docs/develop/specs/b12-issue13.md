# Bundle b12-issue13

## Scope

Issue: https://github.com/convergent-systems-co/praxis/issues/13 — "Prove develop-on-Praxis parity against the v4 baseline"

This is the FINAL issue in Epic #1's child-issue queue. This bundle implements ONLY issue #13. Do not implement Epic #1 itself (it is the controlling/tracking issue — it must not be closed or implemented as part of this bundle; closing it, if desired, is a decision for the human after this lands).

## Dependency status

Depends on #3, #9, #10, #12 — all merged to `main`. Read what they actually produced before designing anything:

- `#3` (baseline benchmark): `benchmark/` — `corpus/` (8 scenario definitions), `metrics/metrics-spec.md`, `report-format/real-run-report-format.md`, `runs/run-20260905T153704Z-praxis-bootstrap/report.md` (one captured real run), `baseline/{baseline-report.md,acceptance-thresholds.md}`, `fixtures/README.md`. This is THE baseline you must prove parity against.
- `#9` (dashboard): `src/praxis_dashboard/` — read-only observability projection.
- `#10` (candidate eval/promotion): `src/praxis_eval/{types,candidates,measurements,comparison,thresholds,gates,promotion,rollback,ledger}.py` — the eval/promotion machinery this issue's parity comparisons should likely run through or alongside.
- `#12` (overlay contract): `src/praxis_overlay/` (generic contract: `manifest.py`, `registry.py`, `evidence.py`, `resources.py`), `src/overlays/development/` (the actual `develop`-overlay implementation: `overlay.py`, `graph.py`, `manifest.py`, `graders.py`, `resources.py`, `compat.py`), `src/overlays/trivial/` (the second, non-development fixture overlay). This is the "Praxis-backed `develop`" you must run and compare against #3's baseline.

Also present (built by other already-merged issues, available but not directly required): `src/praxis_runtime/` (#4), `src/praxis_executors/` (#5), `src/praxis_evidence/` (#6), `src/praxis_runtime/resources/` (#7), `src/praxis_policy/` (#8), `src/praxis_learning/` (#11).

## Full issue body

Parent: #1

## Goal

Prove that the Praxis-backed `develop` overlay preserves the expected behavioral and delivery guarantees of the accepted `develop` v4 baseline.

## Required comparisons

Compare current `develop` v4 baseline runs from #3 against Praxis-backed `develop` using the same workload definitions.

Measure at minimum:

- legal state/event sequence
- completion status
- evidence gates
- retry/repair behavior
- human interrupt behavior
- worktree/resource isolation
- TDD/review/verification semantics
- PR/delivery behavior where applicable
- wall-clock time
- executor latency
- cost/token usage where available
- review/adversarial defect rate

## Acceptance criteria

- Deterministic fake-executor parity tests pass for all accepted baseline fixtures.
- Any intentional state/event differences are documented as an accepted contract migration, not silently normalized.
- Real benchmark runs show no material regression in safety/completion reliability.
- Performance regressions beyond the agreed threshold block migration unless explicitly accepted.
- The legacy `develop` implementation can remain available during migration/rollback until parity is accepted.
- Successful completion provides the evidence needed to make Praxis the runtime dependency beneath `develop`.

## Depends on

- #3 (merged, satisfied)
- #12 (merged, satisfied)
- #9 (merged, satisfied)
- #10 (merged, satisfied)

## Constraints (from the Epic, apply to this issue)

- Do not copy implementation code from ECC or other external projects; external repositories may inform patterns only.
- Keep generic runtime semantics free of software-development concepts such as PRs, TDD, GitHub issues, branches, or code review. Like #12, this issue is explicitly ABOUT proving `develop`-on-Praxis parity, so this bundle's own deliverable code/docs legitimately discuss `develop`/PR/TDD concepts in the context of the parity comparison and the `src/overlays/development/` overlay — but do not introduce any new dev/PR/TDD vocabulary into Praxis core packages (`praxis_runtime`, `praxis_contracts`, `praxis_evidence`, `praxis_executors`, `praxis_policy`, `praxis_eval`, `praxis_overlay`, `praxis_dashboard`, `praxis_learning`).
- Keep model/vendor names out of graph semantics.
- Fail closed on malformed state, invalid transitions, missing evidence, authority violations, and resource conflicts.
- Prefer machine-readable contracts and deterministic validation over prose policy.

## Notes for the tech lead

- This is the last issue in the epic's dependency chain. Once this merges, every child issue of Epic #1 (#2-#13) is complete — Epic #1 itself is a separate decision for a human, do not touch it.
- The actual `~/.claude/skills/develop` skill (the real, external delivery tool building this very repository) is available as reference material for understanding what "the `develop` v4 baseline" behaviorally means (GRAPH.yaml, agents/, runtime/, AUTONOMY.md) — use it as read-only reference for accuracy, do not copy its implementation code, and do not modify it (it is outside this repository and outside your worktree's footprint).
- Run the full existing test suite (not just new tests) before considering any task done — this run has repeatedly surfaced real cross-bundle contract mismatches only when the full suite ran against all merged work together.
- Take the scope at face value; this is a substantial deliverable — the capstone proof for the entire epic, not a stub. It's fine for it to reuse and directly build on #3's, #10's, and #12's existing artifacts rather than reinventing anything.
