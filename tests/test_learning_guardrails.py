"""Tests for the bounded-learning guardrails module.

`check_configuration`/`check_target`/`require_authority_review` are the
fail-closed checks that keep learned heuristics from injecting
authority/policy/security/graph-legality configuration, or targeting those
subsystems directly, and that force every learned-heuristic promotion policy
to demand human review (docs/policy.md's zero-auto-approval default).
"""

from __future__ import annotations

import pytest

from praxis_eval.types import MetricThreshold, PromotionPolicy
from praxis_learning.guardrails import (
    GuardrailViolation,
    check_configuration,
    check_target,
    require_authority_review,
)

_SPEC_VERSION = "1.0.0"


def _policy(authority_requirement: dict | None) -> PromotionPolicy:
    return PromotionPolicy(
        spec_version=_SPEC_VERSION,
        thresholds=(
            MetricThreshold(
                metric="accuracy",
                constraint="required",
                direction="higher_is_better",
            ),
        ),
        authority_requirement=authority_requirement,
    )


def test_check_configuration_forbidden_key_at_top_level_raises():
    with pytest.raises(GuardrailViolation):
        check_configuration({"authority_requirement": {"scopes": []}})


def test_check_configuration_forbidden_key_nested_in_list_of_dicts_raises():
    configuration = {
        "steps": [
            {"name": "harmless"},
            {"nested": {"policy_floor": "high"}},
        ]
    }

    with pytest.raises(GuardrailViolation):
        check_configuration(configuration)


def test_check_configuration_clean_configuration_passes():
    configuration = {
        "pattern": "retry-on-timeout",
        "steps": [{"name": "retry", "max_attempts": 3}],
    }

    check_configuration(configuration)


def test_check_configuration_non_dict_input_raises():
    with pytest.raises(GuardrailViolation):
        check_configuration("not-a-dict")


@pytest.mark.parametrize(
    "target",
    [
        "authority",
        "policy",
        "policy-floor",
        "security-invariant",
        "graph-legality",
        "runtime-transition",
        "AUTHORITY",
        "  policy  ",
        " Policy-Floor ",
    ],
)
def test_check_target_forbidden_targets_raise(target):
    with pytest.raises(GuardrailViolation):
        check_target(target)


def test_check_target_allows_none_and_other_targets():
    check_target(None)
    check_target("some-benign-target")


def test_require_authority_review_no_authority_requirement_raises():
    policy = _policy(authority_requirement=None)

    with pytest.raises(GuardrailViolation):
        require_authority_review(policy)


def test_require_authority_review_only_preferred_and_prohibited_scopes_raises():
    policy = _policy(
        authority_requirement={
            "spec_version": _SPEC_VERSION,
            "scopes": [
                {"scope": "billing", "constraint": "preferred"},
                {"scope": "destructive", "constraint": "prohibited"},
            ],
        }
    )

    with pytest.raises(GuardrailViolation):
        require_authority_review(policy)


def test_require_authority_review_required_scope_passes():
    policy = _policy(
        authority_requirement={
            "spec_version": _SPEC_VERSION,
            "scopes": [
                {"scope": "billing", "constraint": "preferred"},
                {"scope": "learned-heuristic-promotion", "constraint": "required"},
            ],
        }
    )

    require_authority_review(policy)
