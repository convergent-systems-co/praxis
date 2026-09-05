"""Malformed-contract tests: every distinct failure mode must raise
ContractValidationError with a non-empty, informative message (fail-closed,
acceptance criterion 5). Fixtures are declared inline here rather than
imported from test_valid_contracts.py, per the task brief.
"""

from __future__ import annotations

import copy
from pathlib import Path

import pytest

from praxis_contracts.validator import ContractValidationError, validate_document

SCHEMAS_DIR = Path(__file__).resolve().parent.parent / "schemas" / "v1"

VALID_REQUIREMENT = {
    "spec_version": "1.0.0",
    "requirements": [
        {
            "promise": {"spec_version": "1.0.0", "kind": "text-generation"},
            "constraint": "required",
        }
    ],
}

VALID_CAPABILITY_ADVERTISEMENT = {
    "spec_version": "1.0.0",
    "executor_id": "executor-7f3a",
    "capabilities": [
        {
            "spec_version": "1.0.0",
            "satisfies": [{"kind": "text-generation"}],
        }
    ],
}

VALID_EVIDENCE_REQUIREMENT = {
    "spec_version": "1.0.0",
    "evidence": [{"proof_type": "test-pass", "constraint": "required"}],
}

VALID_RESOURCE_CLAIM = {
    "spec_version": "1.0.0",
    "claims": [{"resource_type": "compute-slot", "quantity": 1}],
}


def _assert_fails_closed(instance: dict, schema_path: Path) -> None:
    with pytest.raises(ContractValidationError) as excinfo:
        validate_document(instance, schema_path)
    message = str(excinfo.value)
    assert message.strip()


class TestRequirementMalformed:
    """3 distinct failure modes against requirement.schema.json."""

    def test_missing_spec_version(self):
        instance = copy.deepcopy(VALID_REQUIREMENT)
        del instance["spec_version"]
        _assert_fails_closed(instance, SCHEMAS_DIR / "requirement.schema.json")

    def test_constraint_outside_enum(self):
        instance = copy.deepcopy(VALID_REQUIREMENT)
        instance["requirements"][0]["constraint"] = "optional"
        _assert_fails_closed(instance, SCHEMAS_DIR / "requirement.schema.json")

    def test_extra_top_level_property_rejected(self):
        instance = copy.deepcopy(VALID_REQUIREMENT)
        instance["unexpected_field"] = "not allowed"
        _assert_fails_closed(instance, SCHEMAS_DIR / "requirement.schema.json")


class TestCapabilityAdvertisementMalformed:
    def test_missing_required_executor_id(self):
        instance = copy.deepcopy(VALID_CAPABILITY_ADVERTISEMENT)
        del instance["executor_id"]
        _assert_fails_closed(
            instance, SCHEMAS_DIR / "capability-advertisement.schema.json"
        )


class TestEvidenceRequirementMalformed:
    def test_constraint_outside_enum(self):
        instance = copy.deepcopy(VALID_EVIDENCE_REQUIREMENT)
        instance["evidence"][0]["constraint"] = "mandatory"
        _assert_fails_closed(instance, SCHEMAS_DIR / "evidence-requirement.schema.json")


class TestResourceClaimMalformed:
    def test_non_positive_quantity_rejected(self):
        instance = copy.deepcopy(VALID_RESOURCE_CLAIM)
        instance["claims"][0]["quantity"] = 0
        _assert_fails_closed(instance, SCHEMAS_DIR / "resource-claim.schema.json")
