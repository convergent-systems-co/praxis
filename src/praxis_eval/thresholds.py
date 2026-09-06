"""Configurable promotion-policy/threshold parsing.

`parse_promotion_policy` validates a document against
`PROMOTION_POLICY_SCHEMA_PATH` (fail-closed, propagating
`ContractValidationError` on schema violations) and then builds a
`PromotionPolicy`. Schema `minItems`/`items` can express per-threshold shape
but not "no duplicate `metric` values" across the array, so that invariant is
enforced here via `PromotionPolicyError` after schema validation succeeds.
"""

from __future__ import annotations

from praxis_contracts.validator import validate_document
from praxis_eval.types import (
    PROMOTION_POLICY_SCHEMA_PATH,
    PromotionPolicy,
    promotion_policy_from_document,
)


class PromotionPolicyError(Exception):
    """Raised for policy-shape problems that schema validation cannot express."""


def parse_promotion_policy(document: dict) -> PromotionPolicy:
    validate_document(document, PROMOTION_POLICY_SCHEMA_PATH)

    metrics = [threshold["metric"] for threshold in document["thresholds"]]
    duplicates = {metric for metric in metrics if metrics.count(metric) > 1}
    if duplicates:
        raise PromotionPolicyError(
            f"duplicate metric(s) in thresholds: {sorted(duplicates)}"
        )

    return promotion_policy_from_document(document)
