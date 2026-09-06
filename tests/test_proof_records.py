"""Proof record construction and validation.

Covers the no-secrets-by-default guarantee: the proof-record schema is
closed (additionalProperties: false) and artifacts carry only references
(uri/digest/media_type), never inline content, so an artifact with an
unrecognized extra key must fail validation.
"""

from __future__ import annotations

import pytest

from praxis_contracts.validator import ContractValidationError
from praxis_evidence.proof import build_proof_record, validate_proof_record
from praxis_evidence.types import ProofRecord, proof_record_to_document


def test_build_proof_record_round_trips_through_validation():
    record = build_proof_record(
        run_id="run-1",
        graph_version="1.0.0",
        node_id="n1",
        proof_type="test-pass",
        executor_id="executor-1",
        grader_kind="deterministic",
        status="pass",
    )

    assert isinstance(record, ProofRecord)
    assert record.spec_version == "1.0.0"
    assert record.proof_id
    assert record.produced_at

    # Should not raise: build_proof_record already validated, but re-validating
    # the emitted document proves the shape really is schema-conformant.
    validate_proof_record(proof_record_to_document(record))


def test_build_proof_record_accepts_arbitrary_open_proof_type_and_executor_id():
    record = build_proof_record(
        run_id="run-1",
        graph_version="1.0.0",
        node_id="n1",
        proof_type="totally-made-up-proof-type",
        executor_id="totally-made-up-executor-id",
        grader_kind="human",
        status="inconclusive",
    )

    assert record.proof_type == "totally-made-up-proof-type"
    assert record.executor_id == "totally-made-up-executor-id"


def test_validate_proof_record_raises_on_missing_required_field():
    document = {
        "spec_version": "1.0.0",
        "proof_id": "proof-1",
        "run_id": "run-1",
        "graph_version": "1.0.0",
        "node_id": "n1",
        "proof_type": "test-pass",
        "executor_id": "executor-1",
        "grader_kind": "deterministic",
        # missing "status"
    }

    with pytest.raises(ContractValidationError):
        validate_proof_record(document)


def test_validate_proof_record_raises_on_out_of_range_confidence():
    document = {
        "spec_version": "1.0.0",
        "proof_id": "proof-1",
        "run_id": "run-1",
        "graph_version": "1.0.0",
        "node_id": "n1",
        "proof_type": "test-pass",
        "executor_id": "executor-1",
        "grader_kind": "deterministic",
        "status": "pass",
        "confidence": 1.5,
    }

    with pytest.raises(ContractValidationError):
        validate_proof_record(document)


def test_build_proof_record_raises_on_out_of_range_confidence():
    # Proves build_proof_record's own fail-closed validate_proof_record call
    # (not just validate_proof_record in isolation) rejects invalid input.
    with pytest.raises(ContractValidationError):
        build_proof_record(
            run_id="run-1",
            graph_version="1.0.0",
            node_id="n1",
            proof_type="test-pass",
            executor_id="executor-1",
            grader_kind="deterministic",
            status="pass",
            confidence=1.5,
        )


def test_validate_proof_record_raises_on_artifact_with_unrecognized_key():
    document = {
        "spec_version": "1.0.0",
        "proof_id": "proof-1",
        "run_id": "run-1",
        "graph_version": "1.0.0",
        "node_id": "n1",
        "proof_type": "test-pass",
        "executor_id": "executor-1",
        "grader_kind": "deterministic",
        "status": "pass",
        "artifacts": [{"uri": "file:///tmp/out.log", "content": "leaked secret"}],
    }

    with pytest.raises(ContractValidationError):
        validate_proof_record(document)
