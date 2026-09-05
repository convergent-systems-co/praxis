"""Version-mismatch behavior of validate_document.

expected_major_version compares only the major component of spec_version;
a mismatch must raise ContractValidationError with a message naming the
version mismatch, before any generic schema validation runs.
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
            "promise": {
                "spec_version": "1.0.0",
                "kind": "text-generation",
            },
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
            "id": "cap-primary",
            "satisfies": [
                {
                    "kind": "text-generation",
                    "parameters": {"max_context_tokens": 32768},
                }
            ],
        }
    ],
}


def test_requirement_major_version_mismatch_raises_with_version_message():
    instance = copy.deepcopy(VALID_REQUIREMENT)
    instance["spec_version"] = "2.0.0"

    with pytest.raises(ContractValidationError) as excinfo:
        validate_document(
            instance,
            SCHEMAS_DIR / "requirement.schema.json",
            expected_major_version=1,
        )

    assert "version mismatch" in str(excinfo.value).lower()


def test_capability_advertisement_major_version_mismatch_raises_with_version_message():
    instance = copy.deepcopy(VALID_CAPABILITY_ADVERTISEMENT)
    instance["spec_version"] = "0.9.0"

    with pytest.raises(ContractValidationError) as excinfo:
        validate_document(
            instance,
            SCHEMAS_DIR / "capability-advertisement.schema.json",
            expected_major_version=1,
        )

    assert "version mismatch" in str(excinfo.value).lower()


def test_matching_major_version_does_not_raise_for_version_mismatch():
    instance = copy.deepcopy(VALID_REQUIREMENT)
    instance["spec_version"] = "1.2.3"

    try:
        validate_document(
            instance,
            SCHEMAS_DIR / "requirement.schema.json",
            expected_major_version=1,
        )
    except ContractValidationError as exc:
        assert "version mismatch" not in str(exc).lower()
