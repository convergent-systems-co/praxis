"""Tests for the reusable production conversion between `ExecutionResult`'s
flat `evidence` claim dict and the `list[dict]` of proof-record documents
`TransitionEngine.apply(..., evidence=...)` requires.

Before this module existed, this exact conversion was only implemented as a
private helper duplicated verbatim in two test files (see
`test_executor_end_to_end.py` and `test_policy_gate_alternate_executor.py`),
leaving real callers with no reusable interoperability path between
`praxis_executors` and `praxis_evidence`/`praxis_runtime`.
"""

from __future__ import annotations

from pathlib import Path

from praxis_contracts.validator import validate_document
from praxis_executors.registry import evidence_to_proof_records

_SPEC_VERSION = "1.0.0"
REPO_ROOT = Path(__file__).resolve().parent.parent
PROOF_RECORD_SCHEMA = REPO_ROOT / "schemas" / "v1" / "proof-record.schema.json"


def test_truthy_claim_becomes_a_passing_proof_record():
    records = evidence_to_proof_records(
        {"process-exit-status": True},
        run_id="run-1",
        graph_version=_SPEC_VERSION,
        node_id="n1",
        executor_id="executor-1",
    )

    assert len(records) == 1
    validate_document(records[0], PROOF_RECORD_SCHEMA)
    assert records[0]["proof_type"] == "process-exit-status"
    assert records[0]["status"] == "pass"
    assert records[0]["node_id"] == "n1"
    assert records[0]["run_id"] == "run-1"
    assert records[0]["graph_version"] == _SPEC_VERSION
    assert records[0]["executor_id"] == "executor-1"
    assert records[0]["grader_kind"] == "deterministic"


def test_falsy_claim_becomes_a_failing_proof_record():
    records = evidence_to_proof_records(
        {"peer-attestation": False},
        run_id="run-1",
        graph_version=_SPEC_VERSION,
        node_id="n1",
        executor_id="executor-1",
    )

    assert records[0]["status"] == "fail"


def test_multiple_claims_produce_one_record_each():
    records = evidence_to_proof_records(
        {"a": True, "b": False},
        run_id="run-1",
        graph_version=_SPEC_VERSION,
        node_id="n1",
        executor_id="executor-1",
    )

    assert {record["proof_type"] for record in records} == {"a", "b"}


def test_empty_evidence_produces_no_records():
    assert evidence_to_proof_records(
        {},
        run_id="run-1",
        graph_version=_SPEC_VERSION,
        node_id="n1",
        executor_id="executor-1",
    ) == []
