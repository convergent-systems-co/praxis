"""Tests for configurable promotion-policy/threshold parsing.

`parse_promotion_policy` validates a document against
`promotion-policy.schema.json` (fail-closed, propagating
`ContractValidationError` on schema violations) and then builds a
`PromotionPolicy`. Schema `minItems`/`items` can express per-threshold shape
but not "no duplicate `metric` values" across the array, so that invariant is
enforced here via `PromotionPolicyError` after schema validation succeeds.
"""

from __future__ import annotations

import copy

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_eval.thresholds import PromotionPolicyError, parse_promotion_policy
from praxis_eval.types import MetricThreshold, PromotionPolicy

VALID_POLICY_DOCUMENT = {
    "spec_version": "1.0.0",
    "name": "standard-promotion",
    "thresholds": [
        {
            "metric": "latency_ms",
            "constraint": "required",
            "direction": "lower_is_better",
            "max_regression_pct": 5,
        },
        {
            "metric": "accuracy",
            "constraint": "preferred",
            "direction": "higher_is_better",
        },
        {
            "metric": "cost_usd",
            "constraint": "prohibited",
            "direction": "lower_is_better",
        },
    ],
}


def test_parse_promotion_policy_valid_document_builds_expected_policy():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)

    policy = parse_promotion_policy(document)

    assert isinstance(policy, PromotionPolicy)
    assert policy.spec_version == "1.0.0"
    assert policy.name == "standard-promotion"
    assert policy.authority_requirement is None
    assert policy.thresholds == (
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
        ),
        MetricThreshold(
            metric="cost_usd",
            constraint="prohibited",
            direction="lower_is_better",
        ),
    )


def test_parse_promotion_policy_preserves_document_order():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)
    document["thresholds"] = list(reversed(document["thresholds"]))

    policy = parse_promotion_policy(document)

    assert [t.metric for t in policy.thresholds] == ["cost_usd", "accuracy", "latency_ms"]


def test_parse_promotion_policy_with_authority_requirement_passed_through_unchanged():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)
    document["authority_requirement"] = {
        "spec_version": "1.0.0",
        "scopes": [{"scope": "production-deploy", "constraint": "required"}],
    }

    policy = parse_promotion_policy(document)

    assert policy.authority_requirement == document["authority_requirement"]


def test_parse_promotion_policy_missing_thresholds_raises_contract_validation_error():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)
    del document["thresholds"]

    with pytest.raises(ContractValidationError):
        parse_promotion_policy(document)


def test_parse_promotion_policy_invalid_constraint_raises_contract_validation_error():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)
    document["thresholds"][0]["constraint"] = "mandatory"

    with pytest.raises(ContractValidationError):
        parse_promotion_policy(document)


def test_parse_promotion_policy_invalid_direction_raises_contract_validation_error():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)
    document["thresholds"][0]["direction"] = "sideways"

    with pytest.raises(ContractValidationError):
        parse_promotion_policy(document)


def test_parse_promotion_policy_duplicate_metric_raises_promotion_policy_error():
    document = copy.deepcopy(VALID_POLICY_DOCUMENT)
    document["thresholds"].append(
        {
            "metric": "latency_ms",
            "constraint": "preferred",
            "direction": "lower_is_better",
        }
    )

    with pytest.raises(PromotionPolicyError):
        parse_promotion_policy(document)
