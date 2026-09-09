"""Execution-property fields on capability.schema.json (issue #39).

Covers the five new optional, additive top-level properties -- auth_transport,
interactive, context_window, platform, availability -- per
docs/develop/plans/b1-issue38.md task T1.
"""

from __future__ import annotations

import copy

import pytest

from praxis_contracts.schema_paths import schema_path
from praxis_contracts.validator import ContractValidationError, validate_document

CAPABILITY_SCHEMA = schema_path("capability.schema.json")

BASE_CAPABILITY = {
    "spec_version": "1.0.0",
    "satisfies": [{"kind": "text-generation"}],
}


def test_capability_with_all_execution_properties_validates():
    instance = copy.deepcopy(BASE_CAPABILITY)
    instance.update(
        {
            "auth_transport": "oauth_cli",
            "interactive": True,
            "context_window": 200000,
            "platform": "macos",
            "availability": "generally-available",
        }
    )

    validate_document(instance, CAPABILITY_SCHEMA)


def test_capability_without_execution_properties_still_validates():
    instance = copy.deepcopy(BASE_CAPABILITY)

    validate_document(instance, CAPABILITY_SCHEMA)


def test_capability_with_unrecognized_auth_transport_rejected():
    instance = copy.deepcopy(BASE_CAPABILITY)
    instance["auth_transport"] = "totally-unrecognized"

    with pytest.raises(ContractValidationError) as excinfo:
        validate_document(instance, CAPABILITY_SCHEMA)
    assert str(excinfo.value).strip()


def test_capability_with_negative_context_window_rejected():
    instance = copy.deepcopy(BASE_CAPABILITY)
    instance["context_window"] = -1

    with pytest.raises(ContractValidationError) as excinfo:
        validate_document(instance, CAPABILITY_SCHEMA)
    assert str(excinfo.value).strip()
