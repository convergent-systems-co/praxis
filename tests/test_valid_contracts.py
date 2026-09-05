"""Positive-path behavior of validate_document.

A well-formed instance -- whether one of the checked-in examples/ documents
or an inline literal built to match a schema's required shape -- must
validate with no exception raised.
"""

from __future__ import annotations

import json
from pathlib import Path

from praxis_contracts.validator import validate_document

REPO_ROOT = Path(__file__).resolve().parent.parent
SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"
EXAMPLES_DIR = REPO_ROOT / "examples"


def _load_example(filename: str) -> dict:
    return json.loads((EXAMPLES_DIR / filename).read_text())


def test_graph_requests_capability_example_validates_against_requirement_schema():
    instance = _load_example("graph-requests-capability.json")

    validate_document(instance, SCHEMAS_DIR / "requirement.schema.json")


def test_executor_advertises_capability_example_validates_against_capability_advertisement_schema():
    instance = _load_example("executor-advertises-capability.json")

    validate_document(instance, SCHEMAS_DIR / "capability-advertisement.schema.json")


def test_valid_evidence_requirement_instance_validates():
    instance = {
        "spec_version": "1.0.0",
        "evidence": [
            {
                "proof_type": "test-pass",
                "constraint": "required",
            }
        ],
    }

    validate_document(instance, SCHEMAS_DIR / "evidence-requirement.schema.json")


def test_valid_resource_claim_instance_validates():
    instance = {
        "spec_version": "1.0.0",
        "claims": [
            {
                "resource_type": "compute-slot",
                "quantity": 4,
            }
        ],
    }

    validate_document(instance, SCHEMAS_DIR / "resource-claim.schema.json")
