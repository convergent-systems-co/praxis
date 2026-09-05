"""Schema-version migration strategy.

migrate_document(doc, kind) parses doc["spec_version"] and applies every
registered migration for `kind` up to the current minor version, returning
the document unchanged once it is already current. Since event.schema.json
and run-state.schema.json both start at 1.0.0, there is no real migration to
apply yet -- a 1.0.x document of either kind must round-trip unchanged. A
document whose major version doesn't match what this runtime supports must
raise the same ContractValidationError "version mismatch" shape used
elsewhere in the codebase (see praxis_contracts.validator.validate_document
and tests/test_version_mismatch.py), not a new error type.
"""

from __future__ import annotations

import copy

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_runtime.migrations import migrate_document

VALID_EVENT = {
    "spec_version": "1.0.0",
    "seq": 0,
    "run_id": "run-1",
    "node_id": "node-a",
    "event_type": "transition-attempted",
    "event_id": "evt-1",
    "payload": {"detail": "example"},
}

VALID_RUN_STATE = {
    "spec_version": "1.0.0",
    "run_id": "run-1",
    "cursors": {"start": {"node_id": "start", "status": "pending"}},
    "last_applied_seq": 0,
}


def test_current_event_document_round_trips_unchanged():
    doc = copy.deepcopy(VALID_EVENT)

    migrated = migrate_document(doc, "event")

    assert migrated == VALID_EVENT


def test_current_run_state_document_round_trips_unchanged():
    doc = copy.deepcopy(VALID_RUN_STATE)

    migrated = migrate_document(doc, "run-state")

    assert migrated == VALID_RUN_STATE


def test_event_major_version_mismatch_raises_contract_validation_error():
    doc = copy.deepcopy(VALID_EVENT)
    doc["spec_version"] = "2.0.0"

    with pytest.raises(ContractValidationError) as excinfo:
        migrate_document(doc, "event")

    assert "version mismatch" in str(excinfo.value).lower()


def test_run_state_major_version_mismatch_raises_contract_validation_error():
    doc = copy.deepcopy(VALID_RUN_STATE)
    doc["spec_version"] = "2.0.0"

    with pytest.raises(ContractValidationError) as excinfo:
        migrate_document(doc, "run-state")

    assert "version mismatch" in str(excinfo.value).lower()
