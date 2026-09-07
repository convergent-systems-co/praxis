# Parity decision memo: `develop`-on-Praxis vs. the v4 baseline (issue #13, capstone)

This document is the single place a reader goes to see issue #13's whole parity proof tied
together. It synthesizes T3 (structural parity), T4 (the documented node/event scope gap), T6 (the
promotion gate's real decision), and T7 (the one captured Praxis-side run), and states this
bundle's recommendation for acceptance criterion 6: **"Successful completion provides the evidence
needed to make Praxis the runtime dependency beneath `develop`."**

It is evidence for a future human decision, not a decision itself — see
[Scope boundary](#scope-boundary-this-is-evidence-not-a-decision) below.

## T3 — structural parity and the evidence gate

[`tests/test_parity_fixtures.py`](../../tests/test_parity_fixtures.py) drives all eight fixtures
under [`benchmark/fixtures/`](../../benchmark/fixtures) — one per baseline corpus scenario — through
the real development overlay (`register_development_overlay` + `build_development_graph()`) via a
`FakeExecutor`, and every fixture reaches its `expected_terminal_status` while satisfying its own
internal honesty invariant (no fixture claims something is `expressible_in_overlay: true` unless it
actually resolves to a real graph node, manifest proof type, or `legacy_event_to_proof_type`
mapping). A second, non-parametrized test in the same file re-uses `04-security-remediation`'s
scripted evidence with its `development.test-pass` proof flipped to `fail`, and proves the terminal
node's evidence gate fails closed (raises `TransitionError`) — the same fail-closed behavior the
legacy baseline's evidence gate is expected to exhibit. Structural parity and evidence-gate parity
are demonstrated, with recorded, reproducible, all-fixtures-passing evidence.

## T4 — the documented, accepted node/event scope gap

[`docs/parity/state-event-migration.md`](state-event-migration.md) names, per scenario, exactly
which legacy `develop` v4 node/event names are expressible against the overlay and which are not.
The overlay covers only the legacy **task lane**'s 4-node linear chain
(`write_tdd -> implement -> verify -> commit_task`) plus the `VERIFY_DONE`/`REVIEW_APPROVED`
events; everything in the bundle lane, the shared recovery lane, and the human-interrupt node is
`expressible_in_overlay: false`. This is an **accepted** contract migration, not silent
normalization: every fixture lists the unreachable nodes/events explicitly rather than omitting
them, and widening the overlay beyond the task lane is named as concrete, out-of-scope follow-up
work — not a gap this bundle pretends does not exist.

## T6 — the promotion gate's real decision

[`benchmark/parity/promotion-policy.json`](../../benchmark/parity/promotion-policy.json) makes "no
material safety/completion regression" an enforced, machine-checked gate rather than prose: it sets
`completion_success` as `required` (`higher_is_better`, zero regression tolerated) and
`wall_seconds` as `preferred` only. [`tests/test_parity_promotion.py`](../../tests/test_parity_promotion.py)
runs T5's real baseline/candidate `EvaluationRecord` pair through the real
`praxis_eval.comparison`/`gates`/`promotion`/`rollback` machinery (no mocking): the
`completion_success` comparison is `within_threshold`/`improved`, and `evaluate_promotion_gate`
returns `satisfied: True` — a real, recorded decision, not an assertion of intent. The same test
file also promotes the legacy candidate first and the Praxis candidate second, then demonstrates
`rollback.rollback` restoring the legacy candidate as active without re-running the gate.

**Honesty caveat on the `completion_success` inputs themselves:** the baseline record
(`benchmark/parity/evaluations/baseline-02-feature-implementation.json`) carries
`completion_success: 0.0` against the candidate's `1.0` — but that `0.0` is **not** a recorded v4
completion failure. Per
[`benchmark/baseline/baseline-report.md`](../../benchmark/baseline/baseline-report.md), the sole
captured baseline sample is a **point-in-time snapshot** taken while the run's `status` was still
`running` (20 of 22 dispatched tasks complete), not a terminal capture; `completion_success` encodes
that snapshot's non-terminal state honestly as `0.0` rather than fabricating a terminal outcome the
capture never reached. So the promotion gate's `satisfied: True` result is a real, mechanically
correct comparison of the two recorded values, but it is **not** evidence that the Praxis candidate
out-performed an actual v4 completion failure — it compares a genuine candidate success against a
baseline value that means "no terminal outcome was captured," a distinct claim from "the baseline
failed to complete." This is the same disclosure discipline this document already applies to
`wall_seconds`'s incomparable measurement basis, applied here to `completion_success`'s baseline
input.

Performance is a different, honest story. Per
[`benchmark/baseline/acceptance-thresholds.md`](../../benchmark/baseline/acceptance-thresholds.md),
every timing/latency candidate-gate metric is only ever "within N% of the baseline, or
**inconclusive** pending more baseline samples" — and `N` is not assignable yet for any scenario,
because the v4 baseline has only one captured run. That is why `wall_seconds` is `preferred`, never
`required`/`prohibited`, in `promotion-policy.json`: a `required`/`prohibited` timing constraint
would fabricate a threshold the frozen baseline docs forbid inventing. The gate's completion/safety
decision is real and `satisfied`; its performance dimension is honestly `inconclusive`, not silently
passed.

## T7 — the one captured Praxis-side run

[`benchmark/parity/runs/run-20260906T225426Z-development-overlay/`](../../benchmark/parity/runs/run-20260906T225426Z-development-overlay)
is the one concrete captured Praxis-side run: the development overlay's four-node chain driven
through a real `TransitionEngine`/`RunStateStore`/`EventLog` to `terminal_success` on every node,
with the `commit_task` evidence gate satisfied by real proof records rather than bypassed. Its own
report is explicit that this is **"a deterministic structural/evidence-gate proxy run, not a live timing-comparable capture"**:
it carries no legacy persona dispatch, no PR, and no wall-clock time
comparable to a real `develop` session (the reported 0.0056s is fake-executor replay time, not
persona-dispatch duration). It proves the graph and evidence gate work end to end; it is not, and
does not claim to be, a timing comparison against the legacy baseline.

## Recommendation for acceptance criterion 6

Structural parity (T3), evidence-gate parity (T3), and completion/safety parity (T6) are
demonstrated with recorded, reproducible evidence: `tests/test_parity_fixtures.py` (all 8
fixtures), `tests/test_parity_promotion.py`, `benchmark/parity/promotion-policy.json`, and
`benchmark/fixtures/*.json`. As noted in the T6 section above, "completion/safety parity" here
means the gate mechanically compares the candidate's real success against the baseline's recorded
non-terminal snapshot value, not against a recorded v4 completion failure — real, reproducible
evidence for the comparison that exists, not a claim that the baseline is known to have failed.

Performance parity remains **open**. It is not resolved, and this document does not claim
otherwise: closing it requires either (a) a second real legacy baseline sample, so
`acceptance-thresholds.md`'s `N` becomes assignable for at least one scenario, or (b) a genuinely
comparable live Praxis run captured once a real orchestrator exists (out of this bundle's scope —
T7's run is a structural/evidence-gate proxy, not a timing-comparable one). Until one of those
exists, "performance parity" has no evidence to be inconclusive about resolving, and should be
read as open, not passed by default.

## Acceptance criterion 5, restated as a plain fact

The legacy `develop` skill (`~/.ai/skills/develop`) is untouched by this entire bundle: nothing in
this repository's worktree modifies it — it is outside this repository's footprint, and
[`docs/overlays/development-compat.md`](../overlays/development-compat.md) documents that the
overlay "is not wired into the legacy skill's dispatch path, and the legacy skill has no dependency
on it." T6's `tests/test_parity_promotion.py` rollback test demonstrates the ledger-level mechanism
for reverting to a previously-accepted (i.e., legacy) candidate exists and works today, independent
of whether or when a real dispatch path ever routes `/develop` through Praxis.

## Scope boundary: this is evidence, not a decision

This document is evidence for a future human decision about Epic #1; it is not itself a decision.
It does not close Epic #1 and does not reference closing it — that remains a decision for a human,
per this bundle's own spec. It also does not claim `develop`'s dispatch path has been switched to
Praxis: T6/T9 of `b10-issue12` already established that the development overlay is a parallel,
Praxis-native expression of the task lane's semantics, not a replacement wired into the legacy
skill's dispatch.
