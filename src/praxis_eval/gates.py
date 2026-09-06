"""Health/regression promotion gate over paired metric comparisons.

Mirrors `praxis_evidence.gates.evaluate_gate`'s `required`/`preferred`/
`prohibited` constraint handling, applied here to `MetricComparison` entries
instead of proof types: `required` must be satisfied (status
`"within_threshold"` or `"improved"`) or the gate blocks; `preferred` is
surfaced for informational reasons but never blocks; `prohibited` blocks only
on an actual regression (`status == "regressed"`) -- as in
`evaluate_gate`'s docstring, absence of a determination (`missing` or
`inconclusive`) can never itself violate a prohibition.
"""

from __future__ import annotations

from praxis_eval.types import MetricComparison, PromotionGateResult

_SATISFYING_STATUSES = {"within_threshold", "improved"}


def evaluate_promotion_gate(
    candidate_id: str, comparisons: list[MetricComparison]
) -> PromotionGateResult:
    """Apply constraint semantics from each comparison and return the result.

    `PromotionGateResult.satisfied` is `True` only if every `required`
    comparison is satisfied and no `prohibited` comparison regressed.
    `evaluated` lists every `comparison.metric`, in the order given.
    """
    satisfied = True
    reasons: list[str] = []
    evaluated: list[str] = []

    for comparison in comparisons:
        evaluated.append(comparison.metric)
        item_satisfied = comparison.status in _SATISFYING_STATUSES

        if comparison.constraint == "required":
            if not item_satisfied:
                satisfied = False
                reasons.append(f"{comparison.metric}: {comparison.reason or comparison.status}")
        elif comparison.constraint == "preferred":
            if not item_satisfied:
                reasons.append(f"{comparison.metric}: {comparison.reason or comparison.status}")
        elif comparison.constraint == "prohibited":
            if comparison.status == "regressed":
                satisfied = False
                reasons.append(f"{comparison.metric}: {comparison.reason or comparison.status}")

    return PromotionGateResult(
        candidate_id=candidate_id,
        satisfied=satisfied,
        reasons=tuple(reasons),
        evaluated=tuple(evaluated),
    )
