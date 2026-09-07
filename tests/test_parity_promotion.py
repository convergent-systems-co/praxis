"""Tests for the promotion-policy gate and rollback demonstration (AC3/AC4/AC5).

`benchmark/parity/promotion-policy.json` is the concrete gate: a `required`
threshold on `completion_success` (`direction: "higher_is_better"`,
`max_regression_pct: 0`) makes "no material safety/completion regression"
(AC3) an enforced check, not prose. `wall_seconds` is `preferred` only --
never `required`/`prohibited` -- because `benchmark/baseline/
acceptance-thresholds.md` assigns no regression threshold (`N`) for any
timing metric beyond a single baseline sample, and this bundle's own
candidate/baseline pair additionally has an incomparable timing basis
(deterministic fake-executor replay vs a real captured agent run): a
`required`/`prohibited` constraint here would fabricate a threshold the
frozen baseline docs forbid inventing.

This file loads T5's real baseline/candidate `EvaluationRecord` pair
(`benchmark/parity/evaluations/*-02-feature-implementation.json`) and runs
them through the real `praxis_eval.comparison`/`praxis_eval.gates`/
`praxis_eval.promotion`/`praxis_eval.rollback` machinery -- no mocking of
this package's own modules, mirroring `tests/test_promotion_end_to_end.py`.

The legacy candidate is promoted first (representing the incumbent
implementation already active) and the Praxis candidate second (the
migration under test), so that `rollback.rollback`'s "restore the
second-to-last accepted candidate" mechanics land back on the legacy
candidate -- the concrete demonstration of AC5 ("legacy remains available").
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

from praxis_eval.candidates import CandidateRegistry, build_candidate_config
from praxis_eval.comparison import compare_measurements
from praxis_eval.gates import evaluate_promotion_gate
from praxis_eval.ledger import PromotionLedger
from praxis_eval.measurements import build_evaluation_record
from praxis_eval.promotion import PromotionOutcome, evaluate_candidate, promote
from praxis_eval.rollback import rollback
from praxis_eval.thresholds import parse_promotion_policy
from praxis_eval.types import EvaluationRecord, PromotionPolicy, evaluation_record_from_document

POLICY_PATH = (
    Path(__file__).resolve().parent.parent / "benchmark" / "parity" / "promotion-policy.json"
)
EVALUATIONS_DIR = Path(__file__).resolve().parent.parent / "benchmark" / "parity" / "evaluations"
BASELINE_PATH = EVALUATIONS_DIR / "baseline-02-feature-implementation.json"
CANDIDATE_PATH = EVALUATIONS_DIR / "candidate-02-feature-implementation.json"


@dataclass(frozen=True)
class _FakeProfile:
    auto_approved_authority_scopes: frozenset[str]


def _load_policy() -> PromotionPolicy:
    document = json.loads(POLICY_PATH.read_text())
    return parse_promotion_policy(document)


def _load_eval_record(path: Path) -> EvaluationRecord:
    return evaluation_record_from_document(json.loads(path.read_text()))


def test_completion_success_gate_satisfied_and_wall_seconds_never_blocks():
    policy = _load_policy()
    baseline = _load_eval_record(BASELINE_PATH)
    candidate = _load_eval_record(CANDIDATE_PATH)

    comparisons = compare_measurements(candidate.measurements, baseline.measurements, policy)
    by_metric = {comparison.metric: comparison for comparison in comparisons}

    assert by_metric["completion_success"].constraint == "required"
    assert by_metric["completion_success"].status in {"within_threshold", "improved"}

    # wall_seconds is `preferred`: real baseline (1932.0s, a captured live run) vs real
    # candidate (0.000115s, deterministic fake-executor replay) is not a comparable timing
    # basis, so this bundle asserts only the mechanism's real classification, not a
    # fabricated pass/fail -- see praxis_eval.comparison.compare_measurements: with both
    # baseline and candidate values present, a `preferred` constraint is never classified
    # "inconclusive" (that status is reserved for a missing baseline measurement).
    assert by_metric["wall_seconds"].constraint == "preferred"
    assert by_metric["wall_seconds"].status == "improved"

    gate_result = evaluate_promotion_gate(candidate.candidate_id, comparisons)

    assert gate_result.satisfied is True


def test_praxis_candidate_promotes_then_rollback_restores_legacy(tmp_path: Path) -> None:
    policy = _load_policy()
    baseline_record = _load_eval_record(BASELINE_PATH)
    candidate_record = _load_eval_record(CANDIDATE_PATH)

    registry = CandidateRegistry(tmp_path / "registry")
    ledger = PromotionLedger(tmp_path / "ledger")
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    # 1. The legacy implementation is already active: promote it first, evaluated against
    # its own measurements (there is no earlier candidate to compare it to), exactly as
    # test_promotion_end_to_end.py's baseline step does.
    legacy = build_candidate_config({"implementation": "develop-v4-legacy"})
    registry.register(legacy)
    legacy_eval = build_evaluation_record(
        candidate_id=legacy.candidate_id,
        workload_id=baseline_record.workload_id,
        measurements=baseline_record.measurements,
    )
    legacy_decision = evaluate_candidate(
        candidate_id=legacy.candidate_id,
        candidate_measurements=legacy_eval.measurements,
        baseline_measurements=legacy_eval.measurements,
        policy=policy,
        profile=profile,
    )
    assert legacy_decision.outcome is PromotionOutcome.ACCEPTED
    promote(ledger, registry, legacy_decision, evaluation_ids=[legacy_eval.evaluation_id])
    assert ledger.active_candidate_id() == legacy.candidate_id

    # 2. The Praxis candidate is the migration under test: evaluate it against the legacy
    # candidate's measurements and promote it. This is AC3/AC4 made concrete -- the gate
    # actually runs and actually accepts.
    praxis = build_candidate_config(
        {"implementation": "development-overlay"}, parent_candidate_id=legacy.candidate_id
    )
    registry.register(praxis)
    praxis_eval = build_evaluation_record(
        candidate_id=praxis.candidate_id,
        workload_id=candidate_record.workload_id,
        measurements=candidate_record.measurements,
        baseline_candidate_id=legacy.candidate_id,
    )
    praxis_decision = evaluate_candidate(
        candidate_id=praxis.candidate_id,
        candidate_measurements=praxis_eval.measurements,
        baseline_measurements=legacy_eval.measurements,
        policy=policy,
        profile=profile,
    )
    assert praxis_decision.outcome is PromotionOutcome.ACCEPTED

    praxis_record = promote(
        ledger,
        registry,
        praxis_decision,
        evaluation_ids=[legacy_eval.evaluation_id, praxis_eval.evaluation_id],
    )
    assert praxis_record.decision == "accepted"
    assert ledger.active_candidate_id() == praxis.candidate_id

    # 3. AC5, concretely: legacy remains available, and rollback restores it without
    # re-running any gate (rollback.py's own docstring) -- it simply replays the ledger and
    # finds the second-to-last accepted candidate_id, which is the legacy promotion above.
    rollback_record = rollback(
        ledger, registry, reason="parity-proof rollback demonstration (AC5)"
    )

    assert rollback_record.candidate_id == legacy.candidate_id
    assert ledger.active_candidate_id() == legacy.candidate_id
