"""Behavior of effective_budget and BudgetLedger: how a node-declared
budget_requirement composes with a policy profile's defaults, and how the
ledger tracks per-node retry/repair consumption against that budget.

A minimal stand-in profile (`_FakeProfile`) is used instead of importing
`praxis_policy.profiles.PolicyProfile`/`BUILTIN_PROFILES`, per the task
brief: this task takes `PolicyProfile` only as a type reference for
`default_retry_budget`/`default_repair_budget`/`default_max_cost`/
`default_max_time_seconds` duck-typing, and must not depend on T1's concrete
implementation (same convention `test_authority_boundaries.py` uses for T2).
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import pytest

from praxis_contracts.validator import ContractValidationError, validate_document
from praxis_policy.budgets import BudgetLedger, EffectiveBudget, effective_budget

SCHEMAS_DIR = Path(__file__).resolve().parent.parent / "schemas" / "v1"


@dataclass(frozen=True)
class _FakeProfile:
    default_retry_budget: int
    default_repair_budget: int
    default_max_cost: float | None
    default_max_time_seconds: float | None


def test_no_budget_requirement_uses_profile_defaults_unmodified():
    profile = _FakeProfile(
        default_retry_budget=3,
        default_repair_budget=1,
        default_max_cost=10.0,
        default_max_time_seconds=60.0,
    )

    budget = effective_budget(profile, None)

    assert budget == EffectiveBudget(
        max_retries=3, max_repairs=1, max_cost=10.0, max_time_seconds=60.0
    )


def test_smaller_declared_max_retries_tightens_effective_budget():
    profile = _FakeProfile(
        default_retry_budget=3,
        default_repair_budget=1,
        default_max_cost=None,
        default_max_time_seconds=None,
    )
    requirement = {"spec_version": "1.0.0", "max_retries": 1}

    budget = effective_budget(profile, requirement)

    assert budget.max_retries == 1


def test_larger_declared_max_retries_does_not_loosen_effective_budget():
    profile = _FakeProfile(
        default_retry_budget=3,
        default_repair_budget=1,
        default_max_cost=None,
        default_max_time_seconds=None,
    )
    requirement = {"spec_version": "1.0.0", "max_retries": 10}

    budget = effective_budget(profile, requirement)

    assert budget.max_retries == 3


def test_retry_exhaustion_false_below_cap_true_at_cap_no_cross_node_leakage():
    ledger = BudgetLedger()
    budget = EffectiveBudget(max_retries=2, max_repairs=5, max_cost=None, max_time_seconds=None)

    assert ledger.is_retry_exhausted("node-a", budget) is False
    assert ledger.record_retry("node-a") == 1
    assert ledger.is_retry_exhausted("node-a", budget) is False
    assert ledger.record_retry("node-a") == 2
    assert ledger.is_retry_exhausted("node-a", budget) is True

    assert ledger.is_retry_exhausted("node-b", budget) is False


def test_repair_exhaustion_false_below_cap_true_at_cap_no_cross_node_leakage():
    ledger = BudgetLedger()
    budget = EffectiveBudget(max_retries=5, max_repairs=1, max_cost=None, max_time_seconds=None)

    assert ledger.is_repair_exhausted("node-a", budget) is False
    assert ledger.record_repair("node-a") == 1
    assert ledger.is_repair_exhausted("node-a", budget) is True

    assert ledger.is_repair_exhausted("node-b", budget) is False


def test_valid_budget_requirement_instance_validates_against_schema():
    instance = {"spec_version": "1.0.0", "max_retries": 2, "max_repairs": 1}

    validate_document(instance, SCHEMAS_DIR / "budget-requirement.schema.json")


def test_malformed_budget_requirement_negative_max_retries_fails_closed():
    instance = {"spec_version": "1.0.0", "max_retries": -1}

    with pytest.raises(ContractValidationError):
        validate_document(instance, SCHEMAS_DIR / "budget-requirement.schema.json")
