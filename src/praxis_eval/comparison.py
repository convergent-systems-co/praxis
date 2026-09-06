"""Paired candidate-vs-baseline metric comparison.

`compare_measurements` is a pure function: for each `MetricThreshold` in
`policy.thresholds`, in order, it produces exactly one `MetricComparison`,
never fabricating a passing comparison it cannot actually make (missing
baseline data always surfaces as `"inconclusive"`, per
`benchmark/baseline/acceptance-thresholds.md`'s "do not invent a placeholder"
rule). T7's gate is what turns `"inconclusive"` into a block for
`required`/`prohibited` constraints.
"""

from __future__ import annotations

from praxis_eval.types import Measurement, MetricComparison, MetricThreshold, PromotionPolicy


def _find_measurement(
    measurements: tuple[Measurement, ...], metric: str
) -> Measurement | None:
    for measurement in measurements:
        if measurement.metric == metric:
            return measurement
    return None


def _within_tolerance(
    candidate_value: float, baseline_value: float, direction: str, max_regression_pct: float
) -> bool:
    if direction == "lower_is_better":
        return candidate_value <= baseline_value * (1 + max_regression_pct / 100)
    return candidate_value >= baseline_value * (1 - max_regression_pct / 100)


def _at_least_as_good(candidate_value: float, baseline_value: float, direction: str) -> bool:
    if direction == "lower_is_better":
        return candidate_value <= baseline_value
    return candidate_value >= baseline_value


def _compare_one(
    threshold: MetricThreshold,
    candidate_measurements: tuple[Measurement, ...],
    baseline_measurements: tuple[Measurement, ...] | None,
) -> MetricComparison:
    baseline = (
        _find_measurement(baseline_measurements, threshold.metric)
        if baseline_measurements is not None
        else None
    )
    baseline_value = baseline.value if baseline is not None else None

    candidate = _find_measurement(candidate_measurements, threshold.metric)
    if candidate is None:
        return MetricComparison(
            metric=threshold.metric,
            constraint=threshold.constraint,
            candidate_value=None,
            baseline_value=baseline_value,
            status="missing",
            reason="no candidate measurement for metric",
        )

    if baseline is None:
        return MetricComparison(
            metric=threshold.metric,
            constraint=threshold.constraint,
            candidate_value=candidate.value,
            baseline_value=None,
            status="inconclusive",
            reason="no baseline measurement for metric",
        )

    max_regression_pct = (
        threshold.max_regression_pct if threshold.max_regression_pct is not None else 0
    )
    if _within_tolerance(candidate.value, baseline.value, threshold.direction, max_regression_pct):
        status = (
            "improved"
            if _at_least_as_good(candidate.value, baseline.value, threshold.direction)
            else "within_threshold"
        )
        return MetricComparison(
            metric=threshold.metric,
            constraint=threshold.constraint,
            candidate_value=candidate.value,
            baseline_value=baseline.value,
            status=status,
        )

    reason = (
        f"{threshold.metric}: candidate value {candidate.value} exceeded tolerance of "
        f"{max_regression_pct}% regression from baseline value {baseline.value}"
    )
    return MetricComparison(
        metric=threshold.metric,
        constraint=threshold.constraint,
        candidate_value=candidate.value,
        baseline_value=baseline.value,
        status="regressed",
        reason=reason,
    )


def compare_measurements(
    candidate_measurements: tuple[Measurement, ...],
    baseline_measurements: tuple[Measurement, ...] | None,
    policy: PromotionPolicy,
) -> list[MetricComparison]:
    return [
        _compare_one(threshold, candidate_measurements, baseline_measurements)
        for threshold in policy.thresholds
    ]
