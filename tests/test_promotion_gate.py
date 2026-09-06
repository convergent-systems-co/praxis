"""Tests for the health/regression promotion gate over paired comparisons.

`evaluate_promotion_gate` applies `required`/`preferred`/`prohibited`
constraint semantics to a list of `MetricComparison` (see T5's
`praxis_eval.comparison`), mirroring `praxis_evidence.gates.evaluate_gate`'s
handling of the same three constraint kinds for proof types: `required` must
be satisfied or the gate blocks, `preferred` never blocks, `prohibited`
blocks only on an actual regression -- absence of a determination (missing/
inconclusive) can never itself violate a prohibition.
"""

from __future__ import annotations

from praxis_eval.gates import evaluate_promotion_gate
from praxis_eval.types import MetricComparison, PromotionGateResult

_CANDIDATE_ID = "candidate-1"


def _comparison(
    metric: str, constraint: str, status: str, reason: str | None = None
) -> MetricComparison:
    return MetricComparison(
        metric=metric,
        constraint=constraint,
        candidate_value=1.0,
        baseline_value=1.0,
        status=status,
        reason=reason,
    )


def test_required_regressed_blocks_with_distinct_reason():
    comparison = _comparison(
        "latency_ms", "required", "regressed", reason="candidate exceeded tolerance"
    )

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result == PromotionGateResult(
        candidate_id=_CANDIDATE_ID,
        satisfied=False,
        reasons=("latency_ms: candidate exceeded tolerance",),
        evaluated=("latency_ms",),
    )


def test_required_missing_blocks_with_distinct_reason():
    comparison = _comparison(
        "latency_ms", "required", "missing", reason="no candidate measurement for metric"
    )

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is False
    assert result.reasons == ("latency_ms: no candidate measurement for metric",)


def test_required_inconclusive_blocks_with_distinct_reason():
    comparison = _comparison(
        "latency_ms", "required", "inconclusive", reason="no baseline measurement for metric"
    )

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is False
    assert result.reasons == ("latency_ms: no baseline measurement for metric",)


def test_required_inconclusive_without_reason_falls_back_to_status():
    comparison = _comparison("latency_ms", "required", "inconclusive")

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is False
    assert result.reasons == ("latency_ms: inconclusive",)


def test_required_within_threshold_satisfies():
    comparison = _comparison("latency_ms", "required", "within_threshold")

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is True
    assert result.reasons == ()


def test_required_improved_satisfies():
    comparison = _comparison("latency_ms", "required", "improved")

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is True
    assert result.reasons == ()


def test_preferred_regressed_never_blocks_but_adds_informational_reason():
    comparison = _comparison(
        "accuracy", "preferred", "regressed", reason="candidate below baseline"
    )

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is True
    assert result.reasons == ("accuracy: candidate below baseline",)


def test_prohibited_regressed_blocks():
    comparison = _comparison("cost_usd", "prohibited", "regressed", reason="cost increased")

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is False
    assert result.reasons == ("cost_usd: cost increased",)


def test_prohibited_missing_does_not_block_and_adds_no_reason():
    comparison = _comparison(
        "cost_usd", "prohibited", "missing", reason="no candidate measurement for metric"
    )

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is True
    assert result.reasons == ()


def test_prohibited_inconclusive_does_not_block_and_adds_no_reason():
    comparison = _comparison(
        "cost_usd", "prohibited", "inconclusive", reason="no baseline measurement for metric"
    )

    result = evaluate_promotion_gate(_CANDIDATE_ID, [comparison])

    assert result.satisfied is True
    assert result.reasons == ()


def test_multiple_comparisons_one_blocking_required_among_satisfied_yields_only_its_reason():
    comparisons = [
        _comparison("cost_usd", "prohibited", "within_threshold"),
        _comparison("latency_ms", "required", "regressed", reason="latency exceeded tolerance"),
        _comparison("accuracy", "required", "improved"),
        _comparison("throughput", "preferred", "within_threshold"),
    ]

    result = evaluate_promotion_gate(_CANDIDATE_ID, comparisons)

    assert result.satisfied is False
    assert result.reasons == ("latency_ms: latency exceeded tolerance",)


def test_evaluated_lists_every_input_metric_in_input_order_regardless_of_outcome():
    comparisons = [
        _comparison("cost_usd", "prohibited", "missing"),
        _comparison("latency_ms", "required", "regressed", reason="exceeded tolerance"),
        _comparison("accuracy", "preferred", "improved"),
    ]

    result = evaluate_promotion_gate(_CANDIDATE_ID, comparisons)

    assert result.evaluated == ("cost_usd", "latency_ms", "accuracy")


def test_candidate_id_carried_onto_result():
    comparison = _comparison("latency_ms", "required", "improved")

    result = evaluate_promotion_gate("candidate-xyz", [comparison])

    assert result.candidate_id == "candidate-xyz"
