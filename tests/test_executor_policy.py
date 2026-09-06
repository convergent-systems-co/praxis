"""Tests for executor eligibility policies.

`test_as_eligibility_callable_wired_into_match_changes_selection` is the one
place this bundle proves the acceptance criterion "policy may restrict which
advertised executors are eligible without changing the graph" end-to-end: the
exact same `requirement`/`advertisements` dicts are matched twice, once with
no policy and once with a policy wired in via `as_eligibility_callable`, and
the selected candidate differs.
"""

from __future__ import annotations

from praxis_executors.matching import match
from praxis_executors.policy import (
    AllowListPolicy,
    DenyListPolicy,
    as_eligibility_callable,
)

_ADVERTISEMENT_A = {
    "spec_version": "1.0.0",
    "executor_id": "executor-a",
    "capabilities": [
        {
            "spec_version": "1.0.0",
            "id": "cap-a",
            "satisfies": [{"kind": "text-generation"}],
        }
    ],
}

_ADVERTISEMENT_B = {
    "spec_version": "1.0.0",
    "executor_id": "executor-b",
    "capabilities": [
        {
            "spec_version": "1.0.0",
            "id": "cap-b",
            "satisfies": [{"kind": "text-generation"}],
        }
    ],
}

_REQUIREMENT = {
    "spec_version": "1.0.0",
    "requirements": [
        {
            "promise": {"spec_version": "1.0.0", "kind": "text-generation"},
            "constraint": "required",
        }
    ],
}


def test_allow_list_policy_is_eligible_true_only_for_listed_executor_ids():
    policy = AllowListPolicy(allowed_executor_ids=frozenset({"executor-a"}))

    assert policy.is_eligible("executor-a", _ADVERTISEMENT_A) is True
    assert policy.is_eligible("executor-b", _ADVERTISEMENT_B) is False


def test_deny_list_policy_is_eligible_false_only_for_listed_executor_ids():
    policy = DenyListPolicy(denied_executor_ids=frozenset({"executor-a"}))

    assert policy.is_eligible("executor-a", _ADVERTISEMENT_A) is False
    assert policy.is_eligible("executor-b", _ADVERTISEMENT_B) is True


def test_as_eligibility_callable_wired_into_match_changes_selection():
    advertisements = [_ADVERTISEMENT_A, _ADVERTISEMENT_B]

    unrestricted_result = match(_REQUIREMENT, advertisements)

    policy = DenyListPolicy(denied_executor_ids=frozenset({"executor-a"}))
    restricted_result = match(
        _REQUIREMENT,
        advertisements,
        is_eligible=as_eligibility_callable(policy, advertisements),
    )

    assert unrestricted_result.selected.executor_id == "executor-a"
    assert restricted_result.selected.executor_id == "executor-b"


def test_as_eligibility_callable_returns_false_for_executor_id_absent_from_advertisements():
    policy = AllowListPolicy(allowed_executor_ids=frozenset({"executor-missing"}))
    is_eligible = as_eligibility_callable(policy, [_ADVERTISEMENT_A])

    assert is_eligible("executor-missing") is False
