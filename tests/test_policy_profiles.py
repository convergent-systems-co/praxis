"""Tests for policy profile resolution and the fail-closed minimum-strictness rule.

A user may select a stricter profile than a node's declared minimum but may
never lower a node below it; `resolve_profile` enforces this by comparing
`strictness` ranks, and unknown profile names always fail closed (raise
`PolicyProfileError`) rather than silently falling back to a default.
"""

from __future__ import annotations

import copy

import pytest

from praxis_policy.profiles import (
    BUILTIN_PROFILES,
    _STRICTNESS_ORDER,
    PolicyProfileError,
    profile_from_document,
    resolve_profile,
)

VALID_POLICY_PROFILE_DOCUMENT = {
    "spec_version": "1.0.0",
    "name": "standard",
    "strictness": 1,
    "auto_approved_authority_scopes": [],
    "allow_alternate_executor_retry": True,
    "default_retry_budget": 3,
    "default_repair_budget": 1,
    "default_max_cost": None,
    "default_max_time_seconds": None,
}


def test_resolve_profile_stricter_than_minimum_succeeds():
    profile = resolve_profile("regulated", "fast")

    assert profile is BUILTIN_PROFILES["regulated"]


def test_resolve_profile_weaker_than_minimum_raises():
    with pytest.raises(PolicyProfileError):
        resolve_profile("fast", "regulated")


def test_resolve_profile_exactly_at_minimum_succeeds():
    profile = resolve_profile("fast", "fast")

    assert profile is BUILTIN_PROFILES["fast"]


def test_resolve_profile_unknown_selected_name_raises():
    with pytest.raises(PolicyProfileError):
        resolve_profile("nonexistent", "fast")


def test_resolve_profile_unknown_minimum_name_raises():
    with pytest.raises(PolicyProfileError):
        resolve_profile("fast", "nonexistent")


def test_builtin_profiles_budgets_are_monotonically_non_increasing_with_strictness():
    ordered = [BUILTIN_PROFILES[name] for name in _STRICTNESS_ORDER]

    retry_budgets = [profile.default_retry_budget for profile in ordered]
    repair_budgets = [profile.default_repair_budget for profile in ordered]

    assert retry_budgets == sorted(retry_budgets, reverse=True)
    assert repair_budgets == sorted(repair_budgets, reverse=True)


def test_profile_from_document_round_trips_valid_document():
    document = copy.deepcopy(VALID_POLICY_PROFILE_DOCUMENT)

    profile = profile_from_document(document)

    assert profile.name == document["name"]
    assert profile.strictness == document["strictness"]
    assert profile.auto_approved_authority_scopes == frozenset(
        document["auto_approved_authority_scopes"]
    )
    assert profile.allow_alternate_executor_retry == document["allow_alternate_executor_retry"]
    assert profile.default_retry_budget == document["default_retry_budget"]
    assert profile.default_repair_budget == document["default_repair_budget"]
    assert profile.default_max_cost == document["default_max_cost"]
    assert profile.default_max_time_seconds == document["default_max_time_seconds"]


def test_profile_from_document_missing_required_field_raises():
    document = copy.deepcopy(VALID_POLICY_PROFILE_DOCUMENT)
    del document["default_retry_budget"]

    with pytest.raises(PolicyProfileError):
        profile_from_document(document)
