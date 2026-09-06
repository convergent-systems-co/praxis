"""End-to-end candidate lifecycle test.

Exercises the public interfaces of T2/T3/T4/T5/T6/T7/T8/T9 together against
real temp-directory-backed `CandidateRegistry`/`PromotionLedger` instances --
no mocking of this package's own modules: register a baseline, promote it
(first promotion), derive and promote a winning candidate, reject a
regressed candidate, then roll back -- and confirms `PromotionLedger.read_all()`
alone (no test-only side channel) tells that whole story via monotonically
increasing `seq` and the `candidate_id`/`previous_candidate_id` chain.

The second test proves the "no self-learned heuristic can silently modify
active behavior" acceptance criterion directly: there is no public setter on
`PromotionLedger` for the active pointer, and appending a `"rejected"`-decision
record via `ledger.append()` does not move `active_candidate_id()`.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import pytest

from praxis_eval.candidates import CandidateRegistry, build_candidate_config
from praxis_eval.ledger import PromotionLedger
from praxis_eval.measurements import build_evaluation_record
from praxis_eval.promotion import PromotionError, PromotionOutcome, evaluate_candidate, promote
from praxis_eval.rollback import rollback
from praxis_eval.types import MetricThreshold, PromotionPolicy, PromotionRecord

_SPEC_VERSION = "1.0.0"
_WORKLOAD_ID = "benchmark/corpus/example.md"


@dataclass(frozen=True)
class _FakeProfile:
    auto_approved_authority_scopes: frozenset[str]


def _policy() -> PromotionPolicy:
    return PromotionPolicy(
        spec_version=_SPEC_VERSION,
        thresholds=(
            MetricThreshold(
                metric="latency_ms",
                constraint="required",
                direction="lower_is_better",
                max_regression_pct=5,
            ),
            MetricThreshold(
                metric="accuracy",
                constraint="preferred",
                direction="higher_is_better",
                max_regression_pct=2,
            ),
            MetricThreshold(
                metric="cost_usd",
                constraint="prohibited",
                direction="lower_is_better",
                max_regression_pct=0,
            ),
        ),
    )


def test_full_candidate_lifecycle_register_promote_reject_promote_rollback(
    tmp_path: Path,
) -> None:
    registry = CandidateRegistry(tmp_path / "registry")
    ledger = PromotionLedger(tmp_path / "ledger")
    policy = _policy()
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    # 1. Register and promote a baseline candidate: first promotion ever, so
    # there is no prior active candidate and previous_candidate_id is None.
    # The baseline is evaluated against its own measurements -- there is no
    # earlier candidate to compare it to.
    baseline = build_candidate_config({"model": "baseline-v1"})
    registry.register(baseline)
    baseline_eval = build_evaluation_record(
        candidate_id=baseline.candidate_id,
        workload_id=_WORKLOAD_ID,
        measurements={"latency_ms": 100.0, "accuracy": 0.9, "cost_usd": 1.0},
    )

    baseline_decision = evaluate_candidate(
        candidate_id=baseline.candidate_id,
        candidate_measurements=baseline_eval.measurements,
        baseline_measurements=baseline_eval.measurements,
        policy=policy,
        profile=profile,
    )
    assert baseline_decision.outcome is PromotionOutcome.ACCEPTED
    assert ledger.active_candidate_id() is None

    baseline_record = promote(
        ledger, registry, baseline_decision, evaluation_ids=[baseline_eval.evaluation_id]
    )
    assert baseline_record.previous_candidate_id is None
    assert ledger.active_candidate_id() == baseline.candidate_id

    # 2. Build a winning candidate derived from the baseline (lineage set via
    # parent_candidate_id), evaluate it against the baseline's measurements,
    # and promote it.
    winner = build_candidate_config(
        {"model": "candidate-v2"}, parent_candidate_id=baseline.candidate_id
    )
    registry.register(winner)
    winner_eval = build_evaluation_record(
        candidate_id=winner.candidate_id,
        workload_id=_WORKLOAD_ID,
        measurements={"latency_ms": 95.0, "accuracy": 0.92, "cost_usd": 1.0},
        baseline_candidate_id=baseline.candidate_id,
    )

    winner_decision = evaluate_candidate(
        candidate_id=winner.candidate_id,
        candidate_measurements=winner_eval.measurements,
        baseline_measurements=baseline_eval.measurements,
        policy=policy,
        profile=profile,
    )
    assert winner_decision.outcome is PromotionOutcome.ACCEPTED

    winner_record = promote(
        ledger,
        registry,
        winner_decision,
        evaluation_ids=[baseline_eval.evaluation_id, winner_eval.evaluation_id],
    )
    assert winner_record.previous_candidate_id == baseline.candidate_id
    assert ledger.active_candidate_id() == winner.candidate_id

    # 3. Build a regressed candidate (fails the required latency_ms
    # threshold) and confirm promote() refuses it, leaving the winner active.
    regressed = build_candidate_config(
        {"model": "candidate-v3-bad"}, parent_candidate_id=winner.candidate_id
    )
    registry.register(regressed)
    regressed_eval = build_evaluation_record(
        candidate_id=regressed.candidate_id,
        workload_id=_WORKLOAD_ID,
        measurements={"latency_ms": 200.0, "accuracy": 0.5, "cost_usd": 5.0},
        baseline_candidate_id=winner.candidate_id,
    )

    regressed_decision = evaluate_candidate(
        candidate_id=regressed.candidate_id,
        candidate_measurements=regressed_eval.measurements,
        baseline_measurements=winner_eval.measurements,
        policy=policy,
        profile=profile,
    )
    assert regressed_decision.outcome is PromotionOutcome.REJECTED

    with pytest.raises(PromotionError):
        promote(ledger, registry, regressed_decision, evaluation_ids=[regressed_eval.evaluation_id])
    assert ledger.active_candidate_id() == winner.candidate_id

    # 4. Roll back: the ledger should restore the baseline (the accepted
    # record immediately before the winner's).
    rollback_record = rollback(ledger, registry, reason="candidate-v2 regressed in production")
    assert rollback_record.candidate_id == baseline.candidate_id
    assert rollback_record.previous_candidate_id == winner.candidate_id
    assert ledger.active_candidate_id() == baseline.candidate_id

    # 5. The full ledger, read purely from stored records (no test-only side
    # channel), must have monotonically increasing seq and must tell the
    # whole story: promote baseline (no previous) -> promote winner (previous
    # baseline) -> rollback to baseline (previous winner). The rejected
    # attempt never appears because promote() raises before ever appending.
    records = ledger.read_all()
    assert [r.seq for r in records] == list(range(len(records)))
    assert len(records) == 3

    assert records[0].action == "promote"
    assert records[0].candidate_id == baseline.candidate_id
    assert records[0].previous_candidate_id is None
    assert records[0].decision == "accepted"

    assert records[1].action == "promote"
    assert records[1].candidate_id == winner.candidate_id
    assert records[1].previous_candidate_id == baseline.candidate_id
    assert records[1].decision == "accepted"

    assert records[2].action == "rollback"
    assert records[2].candidate_id == baseline.candidate_id
    assert records[2].previous_candidate_id == winner.candidate_id
    assert records[2].decision == "accepted"

    assert not any(r.candidate_id == regressed.candidate_id for r in records)


def test_active_candidate_id_can_only_move_via_promote_or_rollback(tmp_path: Path) -> None:
    registry = CandidateRegistry(tmp_path / "registry")
    ledger = PromotionLedger(tmp_path / "ledger")
    policy = _policy()
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    # No public setter exists for the active pointer: PromotionLedger's only
    # public surface is append/read_all/active_candidate_id/close.
    public_api = {name for name in dir(PromotionLedger) if not name.startswith("_")}
    assert public_api == {"append", "read_all", "active_candidate_id", "close"}

    candidate = build_candidate_config({"model": "baseline-v1"})
    registry.register(candidate)
    evaluation = build_evaluation_record(
        candidate_id=candidate.candidate_id,
        workload_id=_WORKLOAD_ID,
        measurements={"latency_ms": 100.0, "accuracy": 0.9, "cost_usd": 1.0},
    )
    decision = evaluate_candidate(
        candidate_id=candidate.candidate_id,
        candidate_measurements=evaluation.measurements,
        baseline_measurements=evaluation.measurements,
        policy=policy,
        profile=profile,
    )
    promote(ledger, registry, decision, evaluation_ids=[evaluation.evaluation_id])
    assert ledger.active_candidate_id() == candidate.candidate_id

    # Directly appending a "rejected"-decision record for a different
    # candidate must not move the active pointer, even though append() is
    # public and does not itself forbid a "rejected" decision value.
    ledger.append(
        PromotionRecord(
            spec_version=_SPEC_VERSION,
            record_id="side-channel-attempt",
            seq=0,
            action="promote",
            candidate_id="attacker-controlled-candidate",
            decision="rejected",
            produced_at="2026-09-06T00:00:00Z",
        )
    )

    assert ledger.active_candidate_id() == candidate.candidate_id
