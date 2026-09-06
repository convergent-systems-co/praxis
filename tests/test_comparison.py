"""Tests for paired candidate-vs-baseline metric comparison.

`compare_measurements` produces exactly one `MetricComparison` per
`MetricThreshold` in `policy.thresholds`, in order, per
`benchmark/baseline/acceptance-thresholds.md`'s "do not invent a placeholder"
rule: missing baseline data is always surfaced as `"inconclusive"`, never
silently upgraded to a passing comparison, even for `required`/`prohibited`
constraints (T7's gate is what turns that into a block).
"""

from __future__ import annotations

from praxis_eval.comparison import compare_measurements
from praxis_eval.types import Measurement, MetricComparison, MetricThreshold, PromotionPolicy

_SPEC_VERSION = "1.0.0"


def _policy(*thresholds: MetricThreshold) -> PromotionPolicy:
    return PromotionPolicy(spec_version=_SPEC_VERSION, thresholds=tuple(thresholds))


def test_lower_is_better_within_threshold_when_slightly_worse_but_in_tolerance():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
        max_regression_pct=5,
    )
    candidate = (Measurement(metric="latency_ms", value=104.0),)
    baseline = (Measurement(metric="latency_ms", value=100.0),)

    result = compare_measurements(candidate, baseline, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="latency_ms",
            constraint="required",
            candidate_value=104.0,
            baseline_value=100.0,
            status="within_threshold",
        )
    ]


def test_lower_is_better_improved_when_candidate_at_least_as_good():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
        max_regression_pct=5,
    )
    candidate = (Measurement(metric="latency_ms", value=90.0),)
    baseline = (Measurement(metric="latency_ms", value=100.0),)

    result = compare_measurements(candidate, baseline, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="latency_ms",
            constraint="required",
            candidate_value=90.0,
            baseline_value=100.0,
            status="improved",
        )
    ]


def test_lower_is_better_regressed_when_outside_tolerance():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
        max_regression_pct=5,
    )
    candidate = (Measurement(metric="latency_ms", value=200.0),)
    baseline = (Measurement(metric="latency_ms", value=100.0),)

    [comparison] = compare_measurements(candidate, baseline, _policy(threshold))

    assert comparison.status == "regressed"
    assert comparison.candidate_value == 200.0
    assert comparison.baseline_value == 100.0
    assert comparison.reason is not None
    assert "latency_ms" in comparison.reason
    assert "200" in comparison.reason
    assert "100" in comparison.reason
    assert "5" in comparison.reason


def test_higher_is_better_within_threshold_when_slightly_worse_but_in_tolerance():
    threshold = MetricThreshold(
        metric="accuracy",
        constraint="preferred",
        direction="higher_is_better",
        max_regression_pct=5,
    )
    candidate = (Measurement(metric="accuracy", value=0.96),)
    baseline = (Measurement(metric="accuracy", value=1.0),)

    result = compare_measurements(candidate, baseline, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="accuracy",
            constraint="preferred",
            candidate_value=0.96,
            baseline_value=1.0,
            status="within_threshold",
        )
    ]


def test_higher_is_better_improved_when_candidate_at_least_as_good():
    threshold = MetricThreshold(
        metric="accuracy",
        constraint="preferred",
        direction="higher_is_better",
        max_regression_pct=5,
    )
    candidate = (Measurement(metric="accuracy", value=1.0),)
    baseline = (Measurement(metric="accuracy", value=1.0),)

    result = compare_measurements(candidate, baseline, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="accuracy",
            constraint="preferred",
            candidate_value=1.0,
            baseline_value=1.0,
            status="improved",
        )
    ]


def test_higher_is_better_regressed_when_outside_tolerance():
    threshold = MetricThreshold(
        metric="accuracy",
        constraint="preferred",
        direction="higher_is_better",
        max_regression_pct=5,
    )
    candidate = (Measurement(metric="accuracy", value=0.5),)
    baseline = (Measurement(metric="accuracy", value=1.0),)

    [comparison] = compare_measurements(candidate, baseline, _policy(threshold))

    assert comparison.status == "regressed"
    assert comparison.candidate_value == 0.5
    assert comparison.baseline_value == 1.0
    assert comparison.reason is not None
    assert "accuracy" in comparison.reason


def test_zero_tolerance_default_when_max_regression_pct_omitted_marks_any_worse_value_regressed():
    threshold = MetricThreshold(
        metric="cost_usd",
        constraint="prohibited",
        direction="lower_is_better",
    )
    candidate = (Measurement(metric="cost_usd", value=10.01),)
    baseline = (Measurement(metric="cost_usd", value=10.0),)

    [comparison] = compare_measurements(candidate, baseline, _policy(threshold))

    assert comparison.status == "regressed"


def test_zero_tolerance_default_equal_values_is_improved():
    threshold = MetricThreshold(
        metric="cost_usd",
        constraint="prohibited",
        direction="lower_is_better",
    )
    candidate = (Measurement(metric="cost_usd", value=10.0),)
    baseline = (Measurement(metric="cost_usd", value=10.0),)

    [comparison] = compare_measurements(candidate, baseline, _policy(threshold))

    assert comparison.status == "improved"


def test_missing_candidate_measurement_status_missing():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
    )
    baseline = (Measurement(metric="latency_ms", value=100.0),)

    result = compare_measurements((), baseline, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="latency_ms",
            constraint="required",
            candidate_value=None,
            baseline_value=100.0,
            status="missing",
            reason="no candidate measurement for metric",
        )
    ]


def test_no_baseline_measurement_for_metric_status_inconclusive():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="preferred",
        direction="lower_is_better",
    )
    candidate = (Measurement(metric="latency_ms", value=100.0),)
    baseline = (Measurement(metric="other_metric", value=5.0),)

    result = compare_measurements(candidate, baseline, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="latency_ms",
            constraint="preferred",
            candidate_value=100.0,
            baseline_value=None,
            status="inconclusive",
            reason="no baseline measurement for metric",
        )
    ]


def test_baseline_none_produces_inconclusive_for_required_constraint():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
    )
    candidate = (Measurement(metric="latency_ms", value=100.0),)

    result = compare_measurements(candidate, None, _policy(threshold))

    assert result == [
        MetricComparison(
            metric="latency_ms",
            constraint="required",
            candidate_value=100.0,
            baseline_value=None,
            status="inconclusive",
            reason="no baseline measurement for metric",
        )
    ]


def test_baseline_none_produces_inconclusive_for_prohibited_constraint():
    threshold = MetricThreshold(
        metric="cost_usd",
        constraint="prohibited",
        direction="lower_is_better",
    )
    candidate = (Measurement(metric="cost_usd", value=10.0),)

    result = compare_measurements(candidate, None, _policy(threshold))

    assert result[0].status == "inconclusive"
    assert result[0].constraint == "prohibited"


def test_baseline_none_produces_inconclusive_for_every_threshold_regardless_of_constraint():
    thresholds = (
        MetricThreshold(metric="latency_ms", constraint="required", direction="lower_is_better"),
        MetricThreshold(metric="accuracy", constraint="preferred", direction="higher_is_better"),
        MetricThreshold(metric="cost_usd", constraint="prohibited", direction="lower_is_better"),
    )
    candidate = (
        Measurement(metric="latency_ms", value=100.0),
        Measurement(metric="accuracy", value=1.0),
        Measurement(metric="cost_usd", value=10.0),
    )

    result = compare_measurements(candidate, None, _policy(*thresholds))

    assert [c.status for c in result] == ["inconclusive", "inconclusive", "inconclusive"]


def test_multiple_thresholds_produce_one_comparison_each_in_policy_order():
    thresholds = (
        MetricThreshold(metric="cost_usd", constraint="prohibited", direction="lower_is_better"),
        MetricThreshold(metric="latency_ms", constraint="required", direction="lower_is_better"),
        MetricThreshold(metric="accuracy", constraint="preferred", direction="higher_is_better"),
    )
    candidate = (
        Measurement(metric="latency_ms", value=100.0),
        Measurement(metric="accuracy", value=1.0),
        Measurement(metric="cost_usd", value=10.0),
    )
    baseline = (
        Measurement(metric="latency_ms", value=100.0),
        Measurement(metric="accuracy", value=1.0),
        Measurement(metric="cost_usd", value=10.0),
    )

    result = compare_measurements(candidate, baseline, _policy(*thresholds))

    assert [c.metric for c in result] == ["cost_usd", "latency_ms", "accuracy"]
    assert len(result) == 3


def test_constraint_copied_from_threshold():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="prohibited",
        direction="lower_is_better",
    )
    candidate = (Measurement(metric="latency_ms", value=100.0),)
    baseline = (Measurement(metric="latency_ms", value=100.0),)

    [comparison] = compare_measurements(candidate, baseline, _policy(threshold))

    assert comparison.constraint == "prohibited"


def test_first_matching_measurement_used_when_duplicates_present():
    threshold = MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
    )
    candidate = (
        Measurement(metric="latency_ms", value=100.0),
        Measurement(metric="latency_ms", value=999.0),
    )
    baseline = (
        Measurement(metric="latency_ms", value=100.0),
        Measurement(metric="latency_ms", value=1.0),
    )

    [comparison] = compare_measurements(candidate, baseline, _policy(threshold))

    assert comparison.candidate_value == 100.0
    assert comparison.baseline_value == 100.0
