"""Behavior of evaluate_authority: whether an outcome's declared authority
scopes are auto-approved, require a human, or are denied outright.

A minimal stand-in profile (`_FakeProfile`) is used instead of importing
`praxis_policy.profiles.PolicyProfile`/`BUILTIN_PROFILES`, per the task
brief: this task takes `PolicyProfile` only as a type reference for
`auto_approved_authority_scopes` duck-typing, and must not depend on T1's
concrete implementation.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import pytest

from praxis_contracts.validator import ContractValidationError, validate_document
from praxis_policy.authority import AuthorityDecision, AuthorityOutcome, evaluate_authority

SCHEMAS_DIR = Path(__file__).resolve().parent.parent / "schemas" / "v1"


@dataclass(frozen=True)
class _FakeProfile:
    auto_approved_authority_scopes: frozenset[str]


def _requirement(*entries: tuple[str, str]) -> dict:
    return {
        "spec_version": "1.0.0",
        "scopes": [{"scope": scope, "constraint": constraint} for scope, constraint in entries],
    }


def test_no_authority_requirement_is_auto_approved():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    decision = evaluate_authority(None, profile)

    assert decision == AuthorityDecision(
        outcome=AuthorityOutcome.AUTO_APPROVED,
        unresolved_scopes=frozenset(),
        denied_scopes=frozenset(),
    )


def test_empty_scopes_list_is_auto_approved():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())
    requirement = {"spec_version": "1.0.0", "scopes": []}

    decision = evaluate_authority(requirement, profile)

    assert decision == AuthorityDecision(
        outcome=AuthorityOutcome.AUTO_APPROVED,
        unresolved_scopes=frozenset(),
        denied_scopes=frozenset(),
    )


def test_required_scope_covered_by_profile_auto_approval_is_auto_approved():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset({"billing"}))
    requirement = _requirement(("billing", "required"))

    decision = evaluate_authority(requirement, profile)

    assert decision.outcome is AuthorityOutcome.AUTO_APPROVED
    assert decision.unresolved_scopes == frozenset()
    assert decision.denied_scopes == frozenset()


def test_required_scope_missing_from_profile_and_grants_requires_human():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())
    requirement = _requirement(("production-deploy", "required"))

    decision = evaluate_authority(requirement, profile)

    assert decision.outcome is AuthorityOutcome.HUMAN_REQUIRED
    assert decision.unresolved_scopes == frozenset({"production-deploy"})
    assert decision.denied_scopes == frozenset()


def test_required_scope_covered_by_grant_is_auto_approved():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())
    requirement = _requirement(("production-deploy", "required"))

    decision = evaluate_authority(
        requirement, profile, granted_scopes=frozenset({"production-deploy"})
    )

    assert decision.outcome is AuthorityOutcome.AUTO_APPROVED
    assert decision.unresolved_scopes == frozenset()
    assert decision.denied_scopes == frozenset()


def test_prohibited_scope_granted_anyway_is_denied_even_if_required_scopes_are_satisfied():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset({"billing"}))
    requirement = _requirement(
        ("billing", "required"),
        ("destructive", "prohibited"),
    )

    decision = evaluate_authority(
        requirement, profile, granted_scopes=frozenset({"destructive"})
    )

    assert decision.outcome is AuthorityOutcome.DENIED
    assert decision.denied_scopes == frozenset({"destructive"})


def test_preferred_scope_neither_auto_approved_nor_granted_does_not_change_outcome():
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset({"billing"}))
    requirement_without_preferred = _requirement(("billing", "required"))
    requirement_with_preferred = _requirement(
        ("billing", "required"),
        ("credential-access", "preferred"),
    )

    baseline = evaluate_authority(requirement_without_preferred, profile)
    with_preferred = evaluate_authority(requirement_with_preferred, profile)

    assert with_preferred.outcome == baseline.outcome
    assert with_preferred.outcome is AuthorityOutcome.AUTO_APPROVED
    assert with_preferred.unresolved_scopes == frozenset()
    assert with_preferred.denied_scopes == frozenset()


def test_valid_authority_requirement_instance_validates_against_schema():
    instance = _requirement(("production-deploy", "required"))

    validate_document(instance, SCHEMAS_DIR / "authority-requirement.schema.json")


def test_malformed_authority_requirement_unknown_constraint_fails_closed():
    instance = {
        "spec_version": "1.0.0",
        "scopes": [{"scope": "billing", "constraint": "mandatory"}],
    }

    with pytest.raises(ContractValidationError):
        validate_document(instance, SCHEMAS_DIR / "authority-requirement.schema.json")
